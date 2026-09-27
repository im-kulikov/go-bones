package grpc

import (
	"context"
	"net"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/logger"
)

const (
	serving    = healthpb.HealthCheckResponse_SERVING
	notServing = healthpb.HealthCheckResponse_NOT_SERVING
)

type healthFixture struct {
	monitor *health.Monitor
	db      *health.StatusHandle
	kafka   *health.StatusHandle
	client  healthpb.HealthClient
	stop    func()
}

// newHealthFixture runs a monitor and a gRPC server over an in-memory transport.
// Must be called inside a synctest bubble.
func newHealthFixture(t *testing.T) *healthFixture {
	t.Helper()

	hc := health.New(config.Health{}, logger.ForTests())
	db, err := hc.Status("db")
	require.NoError(t, err)
	kafka, err := hc.Status("kafka", health.WithImpact(health.Informational))
	require.NoError(t, err)

	listener := bufconn.Listen(1024 * 1024)
	srv, err := NewServer(customGRPCSettings{Address: "bufconn"}, logger.ForTests(),
		func(options *serverOptions) { options.open = &fakeOpener{lis: listener} },
		WithHealth(hc),
		WithHealth(nil),
		WithHealthService("pkg.Orders", "db", "kafka"),
		RegisterServices(func(server *Server) {
			server.RegisterService(&sleepServiceDesc, new(sleepServer))
		}),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	monitorDone := make(chan error, 1)
	serverDone := make(chan error, 1)

	go func() { monitorDone <- hc.Start(ctx) }()
	go func() { serverDone <- srv.Start(ctx) }()

	conn, err := NewClient("passthrough:///bufconn",
		WithTransportCredentials(insecure.NewCredentials()),
		gogrpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}))
	require.NoError(t, err)

	synctest.Wait()

	return &healthFixture{
		monitor: hc, db: db, kafka: kafka,
		client: healthpb.NewHealthClient(conn),
		stop: func() {
			require.NoError(t, conn.Close())
			cancel()
			require.NoError(t, <-serverDone)
			require.NoError(t, <-monitorDone)
		},
	}
}

func (f *healthFixture) status(t *testing.T, service string) healthpb.HealthCheckResponse_ServingStatus {
	t.Helper()

	synctest.Wait()

	resp, err := f.client.Check(t.Context(), &healthpb.HealthCheckRequest{Service: service})
	require.NoError(t, err, service)

	return resp.GetStatus()
}

func TestGRPCHealth_FollowsMonitor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newHealthFixture(t)
		defer f.stop()

		require.Equal(t, notServing, f.status(t, ""), "db is unknown yet")
		require.Equal(t, notServing, f.status(t, HealthServiceReadiness))
		require.Equal(t, serving, f.status(t, HealthServiceLiveness))
		require.Equal(t, notServing, f.status(t, "test.SleepService"))
		require.Equal(t, notServing, f.status(t, "pkg.Orders"))

		f.db.Set(nil)
		require.Equal(t, serving, f.status(t, ""))
		require.Equal(t, serving, f.status(t, HealthServiceReadiness))
		require.Equal(t, serving, f.status(t, "test.SleepService"))
		require.Equal(t, notServing, f.status(t, "pkg.Orders"), "kafka is still unknown")

		f.kafka.Set(nil)
		require.Equal(t, serving, f.status(t, "pkg.Orders"))

		f.kafka.Set(context.Canceled)
		require.Equal(t, notServing, f.status(t, "pkg.Orders"))
		require.Equal(t, serving, f.status(t, ""), "informational check does not affect readiness")
	})
}

func TestGRPCHealth_WatchAndDrain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newHealthFixture(t)
		defer f.stop()

		stream, err := f.client.Watch(t.Context(), &healthpb.HealthCheckRequest{Service: HealthServiceReadiness})
		require.NoError(t, err)

		next := func() healthpb.HealthCheckResponse_ServingStatus {
			resp, errRecv := stream.Recv()
			require.NoError(t, errRecv)

			return resp.GetStatus()
		}

		require.Equal(t, notServing, next())

		f.db.Set(nil)
		require.Equal(t, serving, next(), "watchers see changes without polling")

		f.monitor.Drain()
		require.Equal(t, notServing, next())

		f.db.Set(nil)
		require.Equal(t, notServing, f.status(t, HealthServiceLiveness), "shutdown is permanent")
		require.Equal(t, notServing, f.status(t, "test.SleepService"))
	})
}

func TestGRPCHealth_Optional(t *testing.T) {
	srv, err := prepareServer(customGRPCSettings{Address: "127.0.0.1:0"}, logger.ForTests(),
		WithHealthService("pkg.Orders", "db"))
	require.NoError(t, err)

	_, ok := srv.grpc.GetServiceInfo()[healthpb.Health_ServiceDesc.ServiceName]
	require.False(t, ok, "WithHealthService alone does not register the health service")
	require.NotPanics(t, func() { srv.health.start()() })

	var none *healthSync
	require.NotPanics(t, func() { none.start()() })
}

func TestBoundOK(t *testing.T) {
	snap := health.Snapshot{
		Live: true,
		Checks: map[string]health.Result{
			"db": {Status: health.StatusPassing},
		},
	}

	require.True(t, boundOK(snap, []string{"db"}))
	require.False(t, boundOK(snap, []string{"db", "missing"}))

	snap.Live = false
	require.False(t, boundOK(snap, []string{"db"}))
}
