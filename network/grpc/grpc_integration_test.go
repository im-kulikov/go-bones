package grpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/internal/testutil"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

type otelSleepServer struct{ log *logger.Logger }

func (s otelSleepServer) Sleep(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	s.log.InfoContext(ctx, "grpc transport handled")
	return new(emptypb.Empty), nil
}

func Test_GRPCServer_ServesRequests(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	lis, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	sleep := new(sleepServer)
	sleep.delay.Store(int64(10 * time.Millisecond))

	cfg := customGRPCSettings{Address: lis.Addr().String()}
	srv, err := NewServer(cfg, log,
		RegisterServices(
			func(server *Server) {
				healthServer := health.NewServer()
				healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
				healthpb.RegisterHealthServer(server, healthServer)
				server.RegisterService(&sleepServiceDesc, sleep)
			},
		))
	require.NoError(t, err)

	runDone := make(chan error, 1)
	go func() {
		defer close(runDone)
		if errStart := srv.Start(ctx); errStart != nil {
			runDone <- errStart
		}
	}()

	conn := newGRPCConn(
		t,
		cfg.Address,
		WithTransportCredentials(insecure.NewCredentials()),
	)
	defer func() { require.NoError(t, conn.Close()) }()

	requireHealthServing(t, conn)

	out := new(emptypb.Empty)
	require.NoError(t, conn.Invoke(t.Context(), "/test.SleepService/Sleep", &emptypb.Empty{}, out))
	cancel()
	require.NoError(t, <-runDone)
}

func Test_GRPCServer_With_TLS(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	lis, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	var cfg customGRPCSettings
	cfg.Address = lis.Addr().String()
	cfg.TLSConfig = new(config.TLS)
	cfg.ShutdownTimeout = time.Second

	require.NoError(t, gonfig.SetDefaults(cfg.TLSConfig))
	cfg.TLSConfig.Enabled = true
	cfg.TLSConfig.KeyFile, cfg.TLSConfig.CertFile = generateTLSKeyPair(t)

	srv, err := NewServer(cfg, log, RegisterServices(
		func(server *Server) {
			healthServer := health.NewServer()
			healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
			healthpb.RegisterHealthServer(server, healthServer)
		},
	))
	require.NoError(t, err)

	runDone := make(chan error, 1)
	go func() {
		defer close(runDone)
		if errStart := srv.Start(ctx); errStart != nil {
			runDone <- errStart
		}
	}()

	dialOption := WithTransportCredentials(credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	}))

	conn := newGRPCConn(t, cfg.Address, dialOption)
	defer func() { require.NoError(t, conn.Close()) }()

	requireHealthServing(t, conn)

	cancel()
	require.NoError(t, <-runDone)
}

func Test_GRPCServer_LogsShutdownCallback_Integration(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	top, topCancel := context.WithCancel(t.Context())
	defer topCancel()

	lis, err := new(net.ListenConfig).Listen(top, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	buf := logger.NewSyncBuffer()
	log := logger.ForTests(
		logger.TestLoggerWriteToTB(t),
		logger.TestLoggerWriter(logger.NewSyncWriter(buf)),
	)

	cfg := customGRPCSettings{
		Address:  lis.Addr().String(),
		BaseGRPC: config.BaseGRPC{ShutdownTimeout: time.Millisecond},
	}
	srv, err := NewServer(cfg, log, ServiceName("custom-grpc"))
	require.NoError(t, err)

	runDone := make(chan error, 1)
	go func() {
		defer close(runDone)
		if errStart := srv.Start(top); errStart != nil {
			runDone <- errStart
		}
	}()

	require.Eventually(t, func() bool {
		return bytes.Contains(buf.Bytes(), []byte(grpcServerStarting))
	}, time.Second, 10*time.Millisecond)

	stopCtx, stopCancel := context.WithCancel(context.Background())
	stopCancel()
	srv.Stop(stopCtx)
	require.NoError(t, <-runDone)
	assert.Contains(t, buf.String(), "shutdown gracefully done")
	assert.Contains(t, buf.String(), "custom-grpc")
	assert.Contains(t, buf.String(), cfg.Address)
	testutil.WriteArtifact(t, "grpc-shutdown.log", buf.Bytes())
}

func Test_GRPCServer_UsesConfiguredShutdownTimeout_Integration(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	lis, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	sleep := &sleepServer{started: make(chan struct{})}
	sleep.delay.Store(int64(100 * time.Millisecond))

	cfg := customGRPCSettings{
		Address: lis.Addr().String(),
		BaseGRPC: config.BaseGRPC{
			ShutdownTimeout: 250 * time.Millisecond,
		},
	}

	srv, err := NewServer(cfg, log, RegisterServices(
		func(server *Server) { server.RegisterService(&sleepServiceDesc, sleep) },
	))
	require.NoError(t, err)

	runDone := make(chan error, 1)
	go func() {
		defer close(runDone)
		if errStart := srv.Start(ctx); errStart != nil {
			runDone <- errStart
		}
	}()

	conn := newGRPCConn(
		t,
		cfg.Address,
		WithTransportCredentials(insecure.NewCredentials()),
	)
	defer func() { require.NoError(t, conn.Close()) }()

	callDone := make(chan error, 1)
	go func() {
		callDone <- conn.Invoke(context.Background(), "/test.SleepService/Sleep", &emptypb.Empty{}, new(emptypb.Empty))
	}()

	<-sleep.started
	cancelledAt := time.Now()
	cancel()

	select {
	case errDone := <-callDone:
		assert.NoError(t, errDone, "request should complete within configured shutdown timeout")
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}

	select {
	case errDone := <-runDone:
		assert.NoError(t, errDone)
		assert.LessOrEqual(t, time.Since(cancelledAt), cfg.ShutdownTimeout+150*time.Millisecond)
	case <-time.After(cfg.ShutdownTimeout + 150*time.Millisecond):
		require.Fail(t, "server did not stop within configured shutdown timeout budget")
	}
}

func Test_GRPCServer_UsesDefaultShutdownTimeoutFallback_Integration(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	lis, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	sleep := &sleepServer{started: make(chan struct{})}
	sleep.delay.Store(int64(100 * time.Millisecond))

	cfg := customGRPCSettings{Address: lis.Addr().String()}
	srv, err := NewServer(cfg, log, RegisterServices(
		func(server *Server) { server.RegisterService(&sleepServiceDesc, sleep) },
	))
	require.NoError(t, err)

	runDone := make(chan error, 1)
	go func() {
		defer close(runDone)
		if errStart := srv.Start(ctx); errStart != nil {
			runDone <- errStart
		}
	}()

	conn := newGRPCConn(
		t,
		cfg.Address,
		WithTransportCredentials(insecure.NewCredentials()),
	)
	defer func() { require.NoError(t, conn.Close()) }()

	callDone := make(chan error, 1)
	go func() {
		callDone <- conn.Invoke(context.Background(), "/test.SleepService/Sleep", &emptypb.Empty{}, new(emptypb.Empty))
	}()

	<-sleep.started
	cancel()

	select {
	case errDone := <-callDone:
		assert.NoError(
			t,
			errDone,
			"zero shutdown timeout should fall back to default graceful timeout",
		)
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}

	select {
	case errDone := <-runDone:
		assert.NoError(t, errDone)
	case <-time.After(time.Second):
		t.Fatal("server did not stop")
	}
}

func Test_GRPCServer_OpenTelemetryPropagation_Integration(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	recorder := testutil.InstallOTelRecorder(t)
	logger.SetOpenTelemetryBridge(true)
	t.Cleanup(func() { logger.SetOpenTelemetryBridge(false) })

	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	lis, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	cfg := customGRPCSettings{Address: lis.Addr().String()}
	srv, err := NewServer(cfg, log,
		WithOpenTelemetry(),
		RegisterServices(func(server *Server) {
			server.RegisterService(&sleepServiceDesc, otelSleepServer{log: log})
		}),
	)
	require.NoError(t, err)

	runDone := make(chan error, 1)
	go func() { runDone <- srv.Start(ctx) }()

	conn := newGRPCConn(
		t,
		cfg.Address,
		WithTransportCredentials(insecure.NewCredentials()),
	)
	defer func() { require.NoError(t, conn.Close()) }()

	parentSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
		SpanID:     trace.SpanID{4, 4, 4, 4, 4, 4, 4, 4},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	parentCtx := trace.ContextWithSpanContext(context.Background(), parentSpanCtx)
	md := metadata.New(nil)
	otel.GetTextMapPropagator().Inject(parentCtx, metadataCarrier(md))

	err = conn.Invoke(
		metadata.NewOutgoingContext(ctx, md),
		"/test.SleepService/Sleep",
		&emptypb.Empty{},
		new(emptypb.Empty),
	)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		span, ok := recorder.FindSpan("/test.SleepService/Sleep")
		if !ok {
			return false
		}

		record, ok := recorder.FindLog("grpc transport handled")
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

func Test_GRPCServer_ForcesStopWhenShutdownTimeoutExceeded_Integration(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	ctx, cancel := service.SignalContext(t.Context(), syscall.SIGTERM)
	defer cancel()

	lis, err := new(net.ListenConfig).Listen(ctx, "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	sleep := &sleepServer{started: make(chan struct{})}
	sleep.delay.Store(int64(time.Second))

	cfg := customGRPCSettings{
		Address: lis.Addr().String(),
		BaseGRPC: config.BaseGRPC{
			ShutdownTimeout: 20 * time.Millisecond,
		},
	}

	srv, err := NewServer(cfg, log, RegisterServices(
		func(server *Server) { server.RegisterService(&sleepServiceDesc, sleep) },
	))
	require.NoError(t, err)

	runDone := make(chan error, 1)
	go func() {
		defer close(runDone)
		if errStart := srv.Start(ctx); errStart != nil {
			runDone <- errStart
		}
	}()

	conn := newGRPCConn(
		t,
		cfg.Address,
		WithTransportCredentials(insecure.NewCredentials()),
	)
	defer func() { require.NoError(t, conn.Close()) }()

	callDone := make(chan error, 1)
	go func() {
		callDone <- conn.Invoke(context.Background(), "/test.SleepService/Sleep", &emptypb.Empty{}, new(emptypb.Empty))
	}()

	<-sleep.started
	cancelledAt := time.Now()
	cancel()

	select {
	case errDone := <-callDone:
		require.Error(t, errDone)
		assert.Contains(
			t,
			[]codes.Code{codes.Canceled, codes.Unavailable, codes.DeadlineExceeded},
			status.Code(errDone),
		)
	case <-time.After(time.Second):
		t.Fatal("request did not finish")
	}

	select {
	case errDone := <-runDone:
		assert.NoError(t, errDone)
		assert.LessOrEqual(t, time.Since(cancelledAt), cfg.ShutdownTimeout+150*time.Millisecond)
	case <-time.After(cfg.ShutdownTimeout + 150*time.Millisecond):
		require.Fail(t, "server did not force stop within shutdown timeout budget")
	}
}
