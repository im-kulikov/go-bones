// Package health provides a non-blocking health monitor for go-bones services.
//
// A Monitor runs registered checks in its own goroutines (polled checks with
// timeouts, thresholds and staleness), accepts push statuses and heartbeats,
// and keeps the last result in an immutable Snapshot. HTTP OPS endpoints
// (/livez, /readyz, /healthz), gRPC health, Prometheus metrics and Subscribe
// callbacks only read that snapshot, so they answer in microseconds even when
// a checker hangs.
//
// Impact decides what a failing check affects:
//   - Readiness (default): /readyz returns 503 and the instance leaves load balancing;
//   - Informational: /healthz reports "degraded", traffic is kept;
//   - Liveness: /livez returns 503 and the orchestrator restarts the process.
//     Use it only for internal state (see Heartbeat), never for external dependencies.
//
// The package is a leaf: it does not import service. Monitor satisfies
// service.Service structurally and is wired by service.WithHealth.
package health
