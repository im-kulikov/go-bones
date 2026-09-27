package grpc

import (
	"sync"

	grpchealth "google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/im-kulikov/go-bones/health"
)

const (
	// HealthServiceReadiness is the gRPC health service name mapped to Snapshot.Ready,
	// for Kubernetes readinessProbe.grpc.service.
	HealthServiceReadiness = "readiness"
	// HealthServiceLiveness is the gRPC health service name mapped to Snapshot.Live,
	// for Kubernetes livenessProbe.grpc.service.
	HealthServiceLiveness = "liveness"
)

// WithHealth registers the standard grpc.health.v1 service and keeps it in sync
// with the reader (usually a *health.Monitor):
//   - "" and "readiness" follow Snapshot.Ready, "liveness" follows Snapshot.Live;
//   - every registered gRPC service follows Snapshot.Ready unless bound to
//     specific checks with WithHealthService;
//   - once the reader is draining, the health server is shut down: every
//     service becomes NOT_SERVING permanently.
//
// Statuses are synchronized when the server starts and on every health event,
// so Watch clients see changes immediately.
func WithHealth(r health.Reader) Option {
	return func(s *serverOptions) {
		if r == nil {
			return
		}

		if s.health == nil {
			s.health = &healthSync{bindings: make(map[string][]string)}
		}

		s.health.reader = r
	}
}

// WithHealthService binds a gRPC service name to specific health checks: it is
// SERVING while the service is live, not draining and all the checks are up.
// It requires WithHealth.
func WithHealthService(service string, checks ...string) Option {
	return func(s *serverOptions) {
		if s.health == nil {
			s.health = &healthSync{bindings: make(map[string][]string)}
		}

		s.health.bindings[service] = append([]string(nil), checks...)
	}
}

// healthSync mirrors a health.Reader into a grpc health server.
type healthSync struct {
	reader   health.Reader
	bindings map[string][]string
	server   *grpchealth.Server
	grpc     *Server

	mu       sync.Mutex
	shutdown bool
}

// register attaches the health service to srv. It is a no-op without a reader.
func (h *healthSync) register(srv *Server) {
	if h == nil || h.reader == nil {
		return
	}

	h.grpc = srv
	h.server = grpchealth.NewServer()
	healthpb.RegisterHealthServer(srv, h.server)
	h.sync()
}

// start performs the first sync and follows events until the returned stop is called.
func (h *healthSync) start() (stop func()) {
	if h == nil || h.server == nil {
		return func() {}
	}

	unsubscribe := h.reader.Subscribe(func(health.Event) { h.sync() })
	h.sync()

	return unsubscribe
}

func servingStatus(ok bool) healthpb.HealthCheckResponse_ServingStatus {
	if ok {
		return healthpb.HealthCheckResponse_SERVING
	}

	return healthpb.HealthCheckResponse_NOT_SERVING
}

// sync re-reads the snapshot and updates every service status.
func (h *healthSync) sync() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.shutdown {
		return
	}

	snap := h.reader.Snapshot()
	if snap.Draining {
		h.shutdown = true
		h.server.Shutdown()

		return
	}

	h.server.SetServingStatus("", servingStatus(snap.Ready))
	h.server.SetServingStatus(HealthServiceReadiness, servingStatus(snap.Ready))
	h.server.SetServingStatus(HealthServiceLiveness, servingStatus(snap.Live))

	for name := range h.grpc.GetServiceInfo() {
		if name == healthpb.Health_ServiceDesc.ServiceName {
			continue
		}

		if _, bound := h.bindings[name]; !bound {
			h.server.SetServingStatus(name, servingStatus(snap.Ready))
		}
	}

	for name, checks := range h.bindings {
		h.server.SetServingStatus(name, servingStatus(boundOK(snap, checks)))
	}
}

// boundOK reports whether all the given checks are up while the service is live.
func boundOK(snap health.Snapshot, checks []string) bool {
	if !snap.Live {
		return false
	}

	for _, name := range checks {
		res, ok := snap.Checks[name]
		if !ok || !res.Up() {
			return false
		}
	}

	return true
}
