package http

import (
	"context"
	"net/url"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/internal/testutil"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

func Test_NewHTTPServer_With_TLS(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	addr := testutil.FreeTCPAddr(t)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	var cfg customHTTPSettings
	cfg.Address = addr
	cfg.TLSConfig = new(config.TLS)
	cfg.ShutdownTimeout = time.Nanosecond

	require.NoError(t, gonfig.SetDefaults(cfg.TLSConfig))
	cfg.TLSConfig.Enabled = true
	cfg.TLSConfig.KeyFile, cfg.TLSConfig.CertFile = generateTLSKeyPair(t)

	var i atomic.Int64
	srv, err := NewServer(cfg, log, Options(
		ServerOptions(func(server *Server) {
			server.WriteTimeout = 10 * time.Second
			server.Handler = HandlerFunc(func(w ResponseWriter, r *Request) {
				time.Sleep(time.Second * time.Duration(i.Load()))

				Error(w, "test", StatusNotFound)
			})
		})))
	require.NoError(t, err)

	done := make(chan struct{})
	wait := make(chan struct{})
	go func() {
		close(done)
		assert.NoError(t, srv.Start(ctx))
		close(wait)
	}()

	<-done
	defer func() { <-wait }()

	requireHTTPServerReady(t, cfg.Address)
	uri, err := url.Parse("https://" + cfg.Address)
	require.NoError(t, err)

	req, err := NewRequestWithContext(ctx, MethodGet, uri.String(), nil)
	require.NoError(t, err)

	cli := newInsecureTLSClient()
	res, err := cli.Do(req)
	require.NoError(t, err)
	require.Equal(t, StatusNotFound, res.StatusCode)
	require.NoError(t, res.Body.Close())

	out := make(chan struct{})
	go func() {
		i.Store(10)
		reqGet, errGet := NewRequestWithContext(ctx, MethodGet, uri.String(), nil)
		assert.NoError(t, errGet)

		_, errDo := cli.Do(reqGet) // nolint:bodyclose
		assert.ErrorIs(t, errDo, service.ErrCancelCalled)

		close(out)
	}()

	cancel()
	<-out
}

// testShutdownDeadlineCancelsActiveRequest checks that the run context deadline
// cancels a request that is still active when the server shuts down.
func testShutdownDeadlineCancelsActiveRequest(t *testing.T) {
	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	addr := testutil.FreeTCPAddr(t)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	cfg := customHTTPSettings{
		Network: config.Network{ShutdownTimeout: 10 * time.Millisecond},
		Address: addr,
	}
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	defer close(releaseRequest)

	svc, err := NewServer(
		cfg,
		log,
		ServerOptions(func(srv *Server) {
			srv.Handler = HandlerFunc(func(ResponseWriter, *Request) {
				close(requestStarted)
				<-releaseRequest
			})
		}),
	)
	require.NoError(t, err)

	runCtx, runCancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer runCancel()

	runDone := make(chan error, 1)
	go func() { runDone <- svc.Start(runCtx) }()

	requireHTTPServerReady(t, cfg.Address)

	uri := url.URL{Scheme: "http", Host: cfg.Address}
	req, err := NewRequestWithContext(runCtx, MethodGet, uri.String(), nil)
	require.NoError(t, err)

	requestDone := make(chan error, 1)
	go func() {
		resp, requestErr := new(Client).Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}

		requestDone <- requestErr
	}()

	select {
	case <-requestStarted:
	case errReq := <-requestDone:
		require.FailNowf(t, "request failed before reaching the handler", "%v", errReq)
	case <-time.After(time.Second):
		require.FailNow(t, "request did not reach the handler")
	}

	require.ErrorIs(t, <-requestDone, context.DeadlineExceeded)
	assert.NoError(t, <-runDone)
}

func Test_HTTPServer_ShutdownBehavior_Integration(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	t.Run("deadline cancels active request", testShutdownDeadlineCancelsActiveRequest)

	t.Run("configured timeout keeps request alive", func(t *testing.T) {
		ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
		defer cancel()

		addr := testutil.FreeTCPAddr(t)

		log := logger.ForTests(logger.TestLoggerWriteToTB(t))
		expect := 250 * time.Millisecond

		cfg := customHTTPSettings{
			Network: config.Network{ShutdownTimeout: expect},
			Address: addr,
		}

		requestStarted := make(chan struct{})
		requestFinished := make(chan struct{})
		requestDone := make(chan error, 1)
		releaseRequest := make(chan struct{})
		var releaseOnce sync.Once
		defer func() { releaseOnce.Do(func() { close(releaseRequest) }) }()

		svc, err := NewServer(cfg, log, ServerOptions(func(server *Server) {
			server.Handler = HandlerFunc(func(w ResponseWriter, r *Request) {
				close(requestStarted)
				<-releaseRequest
				w.WriteHeader(StatusNoContent)
				close(requestFinished)
			})
		}))
		require.NoError(t, err)

		runDone := make(chan error, 1)
		go func() { runDone <- svc.Start(ctx) }()

		requireHTTPServerReady(t, cfg.Address)
		startHTTPRequest(requestDone, cfg.Address)

		<-requestStarted
		cancelledAt := time.Now()
		cancel()

		requireHTTPShutdownWithinBudget(
			t,
			runDone,
			requestDone,
			requestFinished,
			cancelledAt,
			func() { releaseOnce.Do(func() { close(releaseRequest) }) },
			expect+150*time.Millisecond,
		)
	})

	t.Run("default timeout fallback keeps request alive", func(t *testing.T) {
		ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
		defer cancel()

		addr := testutil.FreeTCPAddr(t)

		log := logger.ForTests(logger.TestLoggerWriteToTB(t))
		cfg := customHTTPSettings{
			Network: config.Network{ShutdownTimeout: 0},
			Address: addr,
		}

		requestStarted := make(chan struct{})
		requestDone := make(chan error, 1)
		releaseRequest := make(chan struct{})
		var releaseOnce sync.Once
		defer func() { releaseOnce.Do(func() { close(releaseRequest) }) }()

		svc, err := NewServer(cfg, log, ServerOptions(func(server *Server) {
			server.Handler = HandlerFunc(func(w ResponseWriter, r *Request) {
				close(requestStarted)
				<-releaseRequest
				w.WriteHeader(StatusNoContent)
			})
		}))
		require.NoError(t, err)

		runDone := make(chan error, 1)
		go func() { runDone <- svc.Start(ctx) }()

		requireHTTPServerReady(t, cfg.Address)
		startHTTPRequest(requestDone, cfg.Address)

		<-requestStarted
		cancel()

		select {
		case errDone := <-runDone:
			require.Failf(
				t,
				"server stopped too early",
				"before active request was released: %v",
				errDone,
			)
		case <-time.After(20 * time.Millisecond):
		}

		releaseOnce.Do(func() { close(releaseRequest) })

		select {
		case errDone := <-requestDone:
			assert.NoError(
				t,
				errDone,
				"zero shutdown timeout should fall back to default graceful timeout",
			)
		case <-time.After(time.Second):
			require.FailNow(t, "request did not finish")
		}

		select {
		case errDone := <-runDone:
			assert.NoError(t, errDone)
		case <-time.After(time.Second):
			require.FailNow(t, "server did not stop")
		}
	})
}

func Test_HTTPServer_OpenTelemetryPropagation_Integration(t *testing.T) {
	testutil.RequireNetworkIntegration(t)
	client := &Client{Transport: &Transport{DisableKeepAlives: true}}

	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	recorder := testutil.InstallOTelRecorder(t)
	logger.SetOpenTelemetryBridge(true)
	t.Cleanup(func() { logger.SetOpenTelemetryBridge(false) })

	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	addr := testutil.FreeTCPAddr(t)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	cfg := customHTTPSettings{Address: addr}

	svc, err := NewServer(
		cfg,
		log,
		WithOpenTelemetry(),
		ServerOptions(func(server *Server) {
			server.Handler = HandlerFunc(func(w ResponseWriter, r *Request) {
				log.InfoContext(r.Context(), "http transport handled")
				w.WriteHeader(StatusNoContent)
			})
		}),
	)
	require.NoError(t, err)

	runDone := make(chan error, 1)
	go func() { runDone <- svc.Start(ctx) }()

	requireHTTPServerReady(t, cfg.Address)

	uri := (&url.URL{Scheme: "http", Host: cfg.Address, Path: "/otel"}).String()
	req, err := NewRequestWithContext(ctx, MethodGet, uri, NoBody)
	require.NoError(t, err)

	parentSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		SpanID:     trace.SpanID{2, 2, 2, 2, 2, 2, 2, 2},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	parentCtx := trace.ContextWithSpanContext(context.Background(), parentSpanCtx)
	otel.GetTextMapPropagator().Inject(parentCtx, propagationHeaderCarrier(req.Header))

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, StatusNoContent, resp.StatusCode)

	require.Eventually(t, func() bool {
		span, ok := recorder.FindSpan("GET /otel")
		if !ok {
			return false
		}

		record, ok := recorder.FindLog("http transport handled")
		if !ok {
			return false
		}

		return span.TraceID == parentSpanCtx.TraceID() &&
			span.ParentSpan == parentSpanCtx.SpanID() &&
			record.TraceID == span.TraceID &&
			record.SpanID == span.SpanID
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.NoError(t, <-runDone)
}
