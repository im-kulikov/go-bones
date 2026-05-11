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
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
)

type customHTTPSettings struct {
	config.Network
	Address string
}

func (c customHTTPSettings) Addr() string { return c.Address }

func Test_NewHTTPServer(t *testing.T) {
	cfg := customHTTPSettings{Network: config.Network{TLSConfig: &config.TLS{Enabled: true}}}
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	require.ErrorIs(
		t,
		bones.ExtractError(NewServer(cfg, log)),
		config.ErrTLSEmptyKeyPair,
	)
}

func Test_serve_TLSConfigPresentButDisabled_UsesHTTPBranch(t *testing.T) {
	buf := logger.NewSyncBuffer()
	log := logger.ForTests(logger.TestLoggerWriter(buf), logger.TestLoggerWriteToTB(t))

	h := &serverOptions{
		Logger: log,
		Server: &Server{},
		base: config.Network{
			TLSConfig: &config.TLS{Enabled: false},
		},
		name: defaultHTTPServiceName,
	}

	err := h.serve(&fakeServeListener{err: net.ErrClosed})
	require.ErrorIs(t, err, net.ErrClosed)
	require.Contains(t, buf.String(), httpServerStarting)
	require.NotContains(t, buf.String(), httpsServerStarting)
}

func Test_newOpenTelemetryHandler(t *testing.T) {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	t.Run("falls back to default serve mux and service name", func(t *testing.T) {
		handler := newOpenTelemetryHandler("fallback-service", nil)
		address := (&url.URL{Scheme: "http", Host: "example.com"}).String()

		req := httptest.NewRequest(MethodGet, address, NoBody)
		req.Method = ""
		req.URL.Path = ""
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)
		require.Equal(t, StatusTemporaryRedirect, rr.Code)
	})

	t.Run("continues extracted trace context", func(t *testing.T) {
		var got trace.SpanContext
		handler := newOpenTelemetryHandler(
			"svc",
			HandlerFunc(func(w ResponseWriter, r *Request) {
				got = trace.SpanContextFromContext(r.Context())
				w.WriteHeader(StatusNoContent)
			}),
		)

		parentSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    trace.TraceID{7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7},
			SpanID:     trace.SpanID{6, 6, 6, 6, 6, 6, 6, 6},
			TraceFlags: trace.FlagsSampled,
			Remote:     true,
		})

		address := (&url.URL{Scheme: "http", Host: "example.com", Path: "/otel"}).String()
		req := httptest.NewRequest(MethodGet, address, NoBody)
		otel.GetTextMapPropagator().Inject(
			trace.ContextWithSpanContext(context.Background(), parentSpanCtx),
			propagationHeaderCarrier(req.Header),
		)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)
		require.Equal(t, StatusNoContent, rr.Code)
		require.Equal(t, parentSpanCtx.TraceID(), got.TraceID())
		require.True(t, got.IsValid())
	})
}

func Test_propagationHeaderCarrier_Keys(t *testing.T) {
	header := Header{
		"Traceparent": []string{"value"},
		"Baggage":     []string{"user_id=42"},
	}

	keys := propagationHeaderCarrier(header).Keys()
	assert.ElementsMatch(t, []string{"Traceparent", "Baggage"}, keys)
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

func newInsecureTLSClient() *Client {
	transport := &Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}

	return &Client{Transport: transport}
}

func startHTTPRequest(done chan<- error, address string) {
	httpAddress := (&url.URL{
		Scheme: "http",
		Host:   address,
	}).String()

	go func() {
		req, errReq := NewRequestWithContext(
			context.Background(),
			MethodGet,
			httpAddress,
			nil,
		)
		if errReq != nil {
			done <- errReq
			return
		}

		res, errRes := DefaultClient.Do(req)
		if errRes == nil {
			_ = res.Body.Close()
		}

		done <- errRes
	}()
}

func requireHTTPServerReady(
	t *testing.T,
	address string,
) {
	t.Helper()

	require.Eventually(t, func() bool {
		conn, err := net.DialTimeout("tcp", address, 20*time.Millisecond)
		if err != nil {
			return false
		}

		_ = conn.Close()

		return true
	}, time.Second, 10*time.Millisecond)
}

func requireHTTPShutdownWithinBudget(
	t *testing.T,
	runDone <-chan error,
	requestDone <-chan error,
	requestFinished <-chan struct{},
	cancelledAt time.Time,
	budget time.Duration,
) {
	t.Helper()

	select {
	case errDone := <-runDone:
		require.Failf(
			t,
			"server stopped too early",
			"after %s: %v",
			time.Since(cancelledAt),
			errDone,
		)
	case <-time.After(20 * time.Millisecond):
		// server should still be waiting for the active request because ShutdownTimeout allows it
	}

	select {
	case <-requestFinished:
	case <-time.After(time.Second):
		require.FailNow(t, "request handler did not finish")
	}

	select {
	case errDone := <-requestDone:
		assert.NoError(t, errDone, "request should complete within configured shutdown timeout")
	case <-time.After(time.Second):
		require.FailNow(t, "request did not finish")
	}

	select {
	case errDone := <-runDone:
		assert.NoError(t, errDone)
		assert.LessOrEqual(t, time.Since(cancelledAt), budget,
			"server shutdown should complete within configured timeout budget")
	case <-time.After(budget):
		require.Fail(t, "server did not stop within configured shutdown timeout budget")
	}
}

type fakeOpener struct {
	onListen error
	lis      net.Listener
}

type fakeServeListener struct {
	addr net.Addr
	err  error
}

func (f *fakeServeListener) Accept() (net.Conn, error) { return nil, f.err }

func (f *fakeServeListener) Close() error { return nil }

func (f *fakeServeListener) Addr() net.Addr {
	if f.addr != nil {
		return f.addr
	}

	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

const (
	errOnListen bones.Error = "error on opening listener"
)

func (f *fakeOpener) Accept() (net.Conn, error) {
	return nil, net.ErrClosed
}

func (f *fakeOpener) Addr() net.Addr {
	if f.lis != nil {
		return f.lis.Addr()
	}

	return &net.IPNet{}
}

func (f *fakeOpener) Listen(context.Context, string, string) (net.Listener, error) {
	if f.onListen != nil {
		return nil, f.onListen
	}

	if f.lis != nil {
		return f.lis, nil
	}

	return f, nil
}

func (f *fakeOpener) Close() error { return nil }

func withFakeListener(errs ...error) Option {
	return func(o *serverOptions) {
		var onListen error
		if len(errs) > 0 {
			onListen = errs[0]
		}

		o.open = &fakeOpener{onListen: onListen}
	}
}

func Test_shouldFailOnListener(t *testing.T) {
	var cfg customHTTPSettings
	require.NoError(t, gonfig.SetDefaults(&cfg))

	cfg.Address = "127.0.0.1:0"
	cfg.ShutdownTimeout = 10 * time.Millisecond

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	t.Run("should fail on listen", func(t *testing.T) { // should fail to listen
		svc, err := NewServer(
			cfg,
			log,
			withFakeListener(errOnListen),
		)
		require.NoError(t, err)

		require.ErrorIs(t, svc.Start(t.Context()), errOnListen)
	})

	t.Run("should fail on serve", func(t *testing.T) {
		const errOnServe bones.Error = "error on serving listener"

		svc, err := NewServer(
			cfg,
			log,
			func(o *serverOptions) {
				o.open = &fakeOpener{
					lis: &fakeServeListener{
						addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
						err:  errOnServe,
					},
				}
			},
		)
		require.NoError(t, err)

		require.ErrorIs(t, svc.Start(t.Context()), errOnServe)
	})
}
