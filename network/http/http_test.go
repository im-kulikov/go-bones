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
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/internal"
	"github.com/im-kulikov/go-bones/logger"
)

type customHTTPSettings struct {
	config.Network
	Address string
}

func (c customHTTPSettings) Addr() string { return c.Address }

type pipeListenerOpener struct{ listener net.Listener }

func (o pipeListenerOpener) Listen(context.Context, string, string) (net.Listener, error) {
	return o.listener, nil
}

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

func TestServer_GracefulShutdown_WithInMemoryTransport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		listener := newPipeListener()
		requestStarted := make(chan struct{})
		releaseRequest := make(chan struct{})

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		cfg := customHTTPSettings{
			Network: config.Network{ShutdownTimeout: time.Second},
			Address: "pipe-listener",
		}
		svc, err := NewServer(
			cfg,
			logger.ForTests(logger.TestLoggerWriteToTB(t)),
			func(options *serverOptions) { options.open = pipeListenerOpener{listener} },
			WithHandler(HandlerFunc(func(w ResponseWriter, _ *Request) {
				close(requestStarted)
				<-releaseRequest
				w.WriteHeader(StatusNoContent)
			})),
		)
		require.NoError(t, err)

		runDone := make(chan error, 1)
		go func() { runDone <- svc.Start(ctx) }()

		client := &Client{Transport: &Transport{DialContext: listener.DialContext}}
		requestDone := make(chan error, 1)
		go func() {
			response, requestErr := client.Get("http://pipe-listener/")
			if response != nil {
				_ = response.Body.Close()
			}
			requestDone <- requestErr
		}()

		<-requestStarted
		cancel()
		synctest.Wait()

		select {
		case errRun := <-runDone:
			require.Failf(
				t,
				"server stopped too early",
				"before the active request was released: %v",
				errRun,
			)
		default:
		}

		close(releaseRequest)
		synctest.Wait()

		require.NoError(t, <-requestDone)
		require.NoError(t, <-runDone)
	})
}

// TestServer_ShutdownTimeout_WithInMemoryTransport pins the configured (and
// fallback) ShutdownTimeout exactly: with virtual time the server must keep
// waiting for an active request until just before the deadline and must stop
// once it expires, closing the still active connection, without adding any
// wall-clock delay to the suite.
func TestServer_ShutdownTimeout_WithInMemoryTransport(t *testing.T) {
	cases := []struct {
		name    string
		timeout time.Duration
		expect  time.Duration
	}{
		{
			name:    "configured timeout",
			timeout: 250 * time.Millisecond,
			expect:  250 * time.Millisecond,
		},
		{
			name:    "zero timeout falls back to default",
			timeout: 0,
			expect:  internal.FallbackTimeout(0),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				listener := newPipeListener()
				requestStarted := make(chan struct{})
				releaseRequest := make(chan struct{})

				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(releaseRequest) }) }
				defer release()

				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()

				cfg := customHTTPSettings{
					Network: config.Network{ShutdownTimeout: tc.timeout},
					Address: "pipe-listener",
				}
				svc, err := NewServer(
					cfg,
					logger.ForTests(logger.TestLoggerWriteToTB(t)),
					func(options *serverOptions) { options.open = pipeListenerOpener{listener} },
					ServerOptions(func(server *Server) {
						server.Handler = HandlerFunc(func(w ResponseWriter, _ *Request) {
							close(requestStarted)
							<-releaseRequest
							w.WriteHeader(StatusNoContent)
						})
					}),
				)
				require.NoError(t, err)

				runDone := make(chan error, 1)
				go func() { runDone <- svc.Start(ctx) }()

				client := &Client{Transport: &Transport{
					DialContext:       listener.DialContext,
					DisableKeepAlives: true,
				}}
				requestDone := make(chan error, 1)
				go func() {
					response, requestErr := client.Get("http://pipe-listener/")
					if response != nil {
						_ = response.Body.Close()
					}
					requestDone <- requestErr
				}()

				<-requestStarted
				cancel()

				time.Sleep(tc.expect - time.Nanosecond)
				synctest.Wait()
				require.Empty(
					t,
					runDone,
					"server must keep waiting for the active request until the shutdown timeout",
				)

				time.Sleep(time.Nanosecond)
				synctest.Wait()
				require.Len(t, runDone, 1, "server must stop once the shutdown timeout expires")
				require.NoError(t, <-runDone)
				require.Error(t, <-requestDone, "the active request must be force-closed")
			})
		})
	}
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
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		DisableKeepAlives: true,
	}

	return &Client{Transport: transport}
}

func startHTTPRequest(done chan<- error, address string) {
	httpAddress := (&url.URL{
		Scheme: "http",
		Host:   address,
	}).String()

	go func() {
		client := &Client{Transport: &Transport{DisableKeepAlives: true}}
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

		res, errRes := client.Do(req)
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
	releaseRequest func(),
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

	releaseRequest()

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

type pipeListener struct {
	connections chan net.Conn
	done        chan struct{}
	closeOnce   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		connections: make(chan net.Conn),
		done:        make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	case conn := <-l.connections:
		return conn, nil
	}
}

func (l *pipeListener) Close() error {
	l.closeOnce.Do(func() { close(l.done) })

	return nil
}

func (*pipeListener) Addr() net.Addr { return pipeListenerAddr{} }

func (l *pipeListener) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	server, client := net.Pipe()

	select {
	case <-ctx.Done():
		_ = server.Close()
		_ = client.Close()

		return nil, ctx.Err()
	case <-l.done:
		_ = server.Close()
		_ = client.Close()

		return nil, net.ErrClosed
	case l.connections <- server:
		return client, nil
	}
}

type pipeListenerAddr struct{}

func (pipeListenerAddr) Network() string { return "pipe" }

func (pipeListenerAddr) String() string { return "pipe-listener" }

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
