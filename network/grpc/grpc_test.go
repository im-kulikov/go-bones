package grpc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
)

type customGRPCSettings struct {
	config.BaseGRPC
	Address string
}

type testContextKey string

func (c customGRPCSettings) Addr() string { return c.Address }

type fakeOpener struct {
	onListen error
	lis      net.Listener
}

func (f *fakeOpener) Listen(context.Context, string, string) (net.Listener, error) {
	if f.onListen != nil {
		return nil, f.onListen
	}

	return f.lis, nil
}

type fakeListener struct {
	addr net.Addr
	err  error
}

func (f *fakeListener) Accept() (net.Conn, error) { return nil, f.err }

func (f *fakeListener) Close() error { return nil }

func (f *fakeListener) Addr() net.Addr {
	if f.addr != nil {
		return f.addr
	}

	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

type sleepServer struct {
	delay   atomic.Int64
	started chan struct{}
}

func (s *sleepServer) Sleep(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	if s.started != nil {
		select {
		case <-s.started:
		default:
			close(s.started)
		}
	}

	timer := time.NewTimer(time.Duration(s.delay.Load()))
	defer timer.Stop()

	select {
	case <-timer.C:
		return &emptypb.Empty{}, nil
	case <-ctx.Done():
		return nil, status.Error(codes.Canceled, ctx.Err().Error())
	}
}

type sleepService interface {
	Sleep(context.Context, *emptypb.Empty) (*emptypb.Empty, error)
}

type fakeServerStream struct {
	ServerStream
	ctx context.Context
}

func (f fakeServerStream) Context() context.Context { return f.ctx }

var sleepServiceDesc = ServiceDesc{ //nolint:gochecknoglobals
	ServiceName: "test.SleepService",
	HandlerType: (*sleepService)(nil),
	Methods: []MethodDesc{
		{
			MethodName: "Sleep",
			Handler: func(
				srv any,
				ctx context.Context,
				dec func(any) error,
				interceptor UnaryServerInterceptor,
			) (any, error) {
				in := new(emptypb.Empty)
				if err := dec(in); err != nil {
					return nil, err
				}

				handle := func(ctx context.Context, req any) (any, error) {
					return srv.(sleepService).Sleep(ctx, req.(*emptypb.Empty))
				}

				if interceptor == nil {
					return handle(ctx, in)
				}

				info := &UnaryServerInfo{
					Server:     srv,
					FullMethod: "/test.SleepService/Sleep",
				}

				return interceptor(ctx, in, info, handle)
			},
		},
	},
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
		NotAfter:  time.Now().Add(24 * time.Hour),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
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

func newGRPCConn(t *testing.T, addr string, opts ...DialOption) *ClientConn {
	t.Helper()

	conn, err := NewClient("passthrough:///"+addr, opts...)
	require.NoError(t, err)

	conn.Connect()
	require.Eventually(t, func() bool {
		state := conn.GetState()
		if state == connectivity.Ready {
			return true
		}

		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		defer cancel()
		conn.WaitForStateChange(ctx, state)

		return conn.GetState() == connectivity.Ready
	}, time.Second, 20*time.Millisecond)

	return conn
}

func requireHealthServing(t *testing.T, conn *ClientConn) {
	t.Helper()

	client := healthpb.NewHealthClient(conn)
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()

		resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
		return err == nil && resp.GetStatus() == healthpb.HealthCheckResponse_SERVING
	}, time.Second, 20*time.Millisecond)
}

func Test_NewGRPCServer(t *testing.T) {
	cfg := customGRPCSettings{BaseGRPC: config.BaseGRPC{TLSConfig: &config.TLS{Enabled: true}}}
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	require.ErrorIs(
		t,
		bones.ExtractError(NewServer(cfg, log)),
		config.ErrTLSEmptyKeyPair,
	)
}

func Test_openTelemetryStreamServerInterceptor(t *testing.T) {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	parentSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5},
		SpanID:     trace.SpanID{4, 4, 4, 4, 4, 4, 4, 4},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})

	md := metadata.New(nil)
	otel.GetTextMapPropagator().Inject(
		trace.ContextWithSpanContext(context.Background(), parentSpanCtx),
		metadataCarrier(md),
	)

	interceptor := openTelemetryStreamServerInterceptor()
	var got trace.SpanContext

	err := interceptor(
		struct{}{},
		fakeServerStream{ctx: metadata.NewIncomingContext(context.Background(), md)},
		&StreamServerInfo{FullMethod: "/test.SleepService/StreamSleep"},
		func(_ any, ss ServerStream) error {
			got = trace.SpanContextFromContext(ss.Context())
			return nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, parentSpanCtx.TraceID(), got.TraceID())
	require.True(t, got.IsValid())
}

func Test_extractIncomingContext(t *testing.T) {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	t.Run("returns original context when metadata is absent", func(t *testing.T) {
		const key testContextKey = "ctx"

		ctx := context.WithValue(context.Background(), key, "value")
		out := extractIncomingContext(ctx)
		require.Equal(t, "value", out.Value(key))
	})

	t.Run("extracts remote trace context from metadata", func(t *testing.T) {
		parentSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    trace.TraceID{8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8},
			SpanID:     trace.SpanID{9, 9, 9, 9, 9, 9, 9, 9},
			TraceFlags: trace.FlagsSampled,
			Remote:     true,
		})

		md := metadata.New(nil)
		otel.GetTextMapPropagator().Inject(
			trace.ContextWithSpanContext(context.Background(), parentSpanCtx),
			metadataCarrier(md),
		)

		out := extractIncomingContext(metadata.NewIncomingContext(context.Background(), md))
		got := trace.SpanContextFromContext(out)
		require.Equal(t, parentSpanCtx.TraceID(), got.TraceID())
	})
}

func Test_serverStreamWithContext_Context(t *testing.T) {
	const key testContextKey = "ctx"

	expected := context.WithValue(context.Background(), key, "value")
	stream := &serverStreamWithContext{ctx: expected}

	require.Equal(t, "value", stream.Context().Value(key))
}

func Test_metadataCarrier_Keys(t *testing.T) {
	md := metadata.MD{
		"traceparent": []string{"value"},
		"baggage":     []string{"user_id=42"},
	}

	keys := metadataCarrier(md).Keys()
	require.ElementsMatch(t, []string{"traceparent", "baggage"}, keys)
}

func Test_GRPCServer_Options(t *testing.T) {
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	cfg := customGRPCSettings{Address: "127.0.0.1:0"}

	srv, err := prepareServer(
		cfg,
		log,
		Options(
			ServiceName("custom-grpc"),
			ServerOptions(MaxRecvMsgSize(1024)),
			RegisterServices(func(server *Server) {
				healthServer := health.NewServer()
				healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
				healthpb.RegisterHealthServer(server, healthServer)
			}),
		),
	)
	require.NoError(t, err)
	require.Equal(t, "custom-grpc", srv.name)
	require.Len(t, srv.opts, 1)
	require.NotNil(t, srv.grpc)
	_, ok := srv.grpc.GetServiceInfo()["grpc.health.v1.Health"]
	require.True(t, ok, "register callbacks from Options should be applied")
}

func Test_GRPCServer_FailsOnListener(t *testing.T) {
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	cfg := customGRPCSettings{Address: "127.0.0.1:0"}

	const errOnListen bones.Error = "error on opening listener"

	srv, err := NewServer(
		cfg,
		log,
		func(o *serverOptions) {
			o.open = &fakeOpener{onListen: errOnListen}
		},
	)
	require.NoError(t, err)
	require.ErrorIs(t, srv.Start(t.Context()), errOnListen)
}

func Test_GRPCServer_ReturnsServeError(t *testing.T) {
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	cfg := customGRPCSettings{Address: "127.0.0.1:0"}

	const errServe bones.Error = "listener accept failed"

	srv, err := NewServer(
		cfg,
		log,
		func(o *serverOptions) {
			o.open = &fakeOpener{
				lis: &fakeListener{
					addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
					err:  errServe,
				},
			}
		},
	)
	require.NoError(t, err)
	require.ErrorIs(t, srv.Start(t.Context()), errServe)
}

func Test_GRPCServer_ForceStop_SkipsAfterGracefulCompletion(t *testing.T) {
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	srv, err := prepareServer(customGRPCSettings{Address: "127.0.0.1:0"}, log)
	require.NoError(t, err)

	stopDone := make(chan struct{})
	close(stopDone)

	require.NotPanics(t, func() { srv.forceStop(stopDone) })
}

func Test_GRPCServer_ForceStop_StopsWhenGracefulShutdownStillRunning(t *testing.T) {
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	srv, err := prepareServer(customGRPCSettings{Address: "127.0.0.1:0"}, log)
	require.NoError(t, err)

	stopDone := make(chan struct{})
	require.NotPanics(t, func() { srv.forceStop(stopDone) })
}
