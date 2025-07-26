package http

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

type customHTTPSettings struct {
	config.BaseHTTP
	Address string
}

func (c customHTTPSettings) Addr() string { return c.Address }

func Test_NewHTTPServer(t *testing.T) {
	cfg := customHTTPSettings{BaseHTTP: config.BaseHTTP{TLSConfig: &config.TLS{Enabled: true}}}
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	require.ErrorIs(
		t,
		bones.ExtractError(NewServer(cfg, log)),
		config.ErrTLSEmptyKeyPair,
	)
}

func generateTLSKeyPair(t *testing.T) (string, string) {
	t.Helper()

	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	serialNumber, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	require.NoError(t, err)

	tpl := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour * 24),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &tpl, &tpl, &private.PublicKey, private)
	require.NoError(t, err)

	data := x509.MarshalPKCS1PrivateKey(private)
	keyFile, err := os.CreateTemp(t.TempDir(), "*.key")
	require.NoError(t, err)
	require.NoError(t, pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: data}))
	require.NoError(t, keyFile.Close())

	crtFile, err := os.CreateTemp(t.TempDir(), "*.crt")
	require.NoError(t, err)
	require.NoError(t, pem.Encode(crtFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}))
	require.NoError(t, crtFile.Close())

	return keyFile.Name(), crtFile.Name()
}

func Test_NewHTTPServer_With_TLS(t *testing.T) {
	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	lis, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	var cfg customHTTPSettings
	cfg.Address = lis.Addr().String()
	cfg.TLSConfig = new(config.TLS)
	cfg.ShutdownTimeout = time.Nanosecond

	require.NoError(t, gonfig.SetDefaults(cfg.TLSConfig))
	cfg.TLSConfig.Enabled = true
	cfg.TLSConfig.KeyFile, cfg.TLSConfig.CertFile = generateTLSKeyPair(t)

	var i atomic.Int64
	srv, err := NewServer(cfg, log, Options(
		ServerOptions(func(server *http.Server) {
			server.WriteTimeout = 10 * time.Second
			server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(time.Second * time.Duration(i.Load()))

				http.Error(w, "test", http.StatusNotFound)
			})
		})))
	require.NoError(t, err)

	done := make(chan struct{})
	wait := make(chan struct{})
	go func() {
		close(done)
		assert.ErrorIs(t, srv.Start(ctx), service.ErrCancelCalled)
		close(wait)
	}()

	<-done
	defer func() { <-wait }()

	time.Sleep(100 * time.Millisecond) // wait for start server
	uri, err := url.Parse("https://" + cfg.Address)
	require.NoError(t, err)

	{
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil)
		require.NoError(t, err)

		cli := new(http.Client)
		cli.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}

		res, err := cli.Do(req)
		require.NoError(t, err)

		require.Equal(t, http.StatusNotFound, res.StatusCode)
		require.NoError(t, res.Body.Close())
	}

	out := make(chan struct{})
	go func() {
		i.Store(10)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil)
		assert.NoError(t, err)

		cli := new(http.Client)
		cli.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}

		_, errDo := cli.Do(req) // nolint:bodyclose
		assert.ErrorIs(t, errDo, service.ErrCancelCalled)

		close(out)
	}()

	time.Sleep(100 * time.Millisecond)

	cancel()
	<-out
}

type fakeOpener struct {
	onListen error
	onClose  error
}

const (
	errOnListen bones.Error = "error on opening listener"
	errOnClose  bones.Error = "error on closing listener"
)

func (f *fakeOpener) Accept() (net.Conn, error) {
	panic("implement me")
}

func (f *fakeOpener) Addr() net.Addr { return &net.IPNet{} }

func (f *fakeOpener) Listen(context.Context, string, string) (net.Listener, error) {
	return f, f.onListen
}

func (f *fakeOpener) Close() error { return f.onClose }

func withFakeListener(errs ...error) Option {
	return func(o *serverOptions) {
		var onListen error
		if len(errs) > 0 {
			onListen = errs[0]
		}

		var onClose error
		if len(errs) > 1 {
			onClose = errs[1]
		}

		o.open = &fakeOpener{onListen: onListen, onClose: onClose}
	}
}

func Test_shouldFailOnListener(t *testing.T) {
	top, stop := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer stop()

	var address string
	{
		lis, err := new(net.ListenConfig).Listen(top, "tcp", "127.0.0.1:0")
		require.NoError(t, err)
		require.NoError(t, lis.Close())

		address = lis.Addr().String()
	}

	var cfg customHTTPSettings
	require.NoError(t, gonfig.SetDefaults(&cfg))

	cfg.Address = address
	cfg.ShutdownTimeout = 10 * time.Millisecond

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	t.Run("should fail on listen", func(t *testing.T) { // should fail on listen
		svc, err := NewServer(
			cfg,
			log,
			withFakeListener(errOnListen),
		)
		require.NoError(t, err)

		require.ErrorIs(t, svc.Start(t.Context()), errOnListen)
	})

	t.Run("should fail on close listener", func(t *testing.T) { // should fail on close listener
		svc, err := NewServer(
			cfg,
			log,
			withFakeListener(nil, errOnClose),
		)
		require.NoError(t, err)

		require.ErrorIs(t, svc.Start(t.Context()), errOnClose)
	})

	t.Run("should fail on shutdown", func(t *testing.T) { // should fail on shutdown
		ctx, cancel := context.WithTimeout(top, time.Millisecond*100)
		defer cancel()

		svc, err := NewServer(
			cfg,
			log,
			ServerOptions(func(srv *http.Server) {
				srv.Handler = http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					time.Sleep(time.Second)
				})
			}))
		require.NoError(t, err)

		done := make(chan struct{})
		wait := make(chan struct{})
		context.AfterFunc(ctx, func() {
			close(done)
			assert.ErrorIs(t, svc.Start(ctx), context.DeadlineExceeded)
			close(wait)
		})

		<-done
		defer func() { <-wait }()

		time.Sleep(100 * time.Millisecond) // wait for server up

		uri := url.URL{Scheme: "http", Host: address}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil)
		require.NoError(t, err)

		cli := new(http.Client)
		require.ErrorIs(
			t,
			bones.ExtractError(cli.Do(req)), // nolint:bodyclose
			context.DeadlineExceeded,
		)
	})
}
