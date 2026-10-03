# network: HTTP, gRPC, OPS

Read when: touching `network/`, `network/http/` or `network/grpc/`.
User view: [HTTP & gRPC](../transports.md), [Ops server](../ops.md),
[Health › HTTP endpoints](../health.md#http-endpoints),
[Health › gRPC health](../health.md#grpc-health).

## Files

| File | What it holds |
|---|---|
| `network/listener.go` | `ListenOpener` — listener seam for tests |
| `network/http/doc.go` | package godoc |
| `network/http/http.go` | `NewServer` (launcher around `http.Server`), `Option`s (`ServiceName`, `WithHandler`, `ServerOptions`, `Options`, `WithOpenTelemetry`), graceful shutdown |
| `network/http/ops.go` | `NewOPSServer`, `RegisterMetrics`, process-wide Prometheus registry, pprof, expvar, `/version`, `ErrOPSRouteConflict` |
| `network/http/health.go` | `WithHealth` (OPS option), `/livez` `/readyz` `/healthz` handlers, JSON report |
| `network/http/aliases.go` | re-exports of `net/http` types, constants and funcs |
| `network/grpc/doc.go` | package godoc |
| `network/grpc/grpc.go` | `NewServer` (launcher around `grpc.Server`), `Option`s (`ServiceName`, `ServerOptions`, `RegisterServices`, `Options`, `WithOpenTelemetry`), OTel interceptors |
| `network/grpc/health.go` | `WithHealth`, `WithHealthService`, `healthSync` (monitor → `grpc.health.v1`) |
| `network/grpc/aliases.go` | re-exports of `google.golang.org/grpc` types and funcs |

## Invariants

- Both `NewServer`s return `service.NewLauncher(name, listen, …)`. `listen`
  opens the socket through `ListenOpener`, records the real address (`:0`
  works), serves, and shuts down gracefully within `Network.ShutdownTimeout`
  when the ctx is canceled. `ErrServerClosed`/`ErrServerStopped` are clean.
- HTTP: `Shutdown(ctx)`, and `Close()` when it hits the deadline, so active
  connections are force-closed like gRPC's hard `Stop`. Handlers still running
  see write errors; their goroutines are not killed.
- gRPC: `GracefulStop` with a hard `Stop` after the timeout (force-close). The defer order
  in `listen` is load-bearing (see the comment there) — do not reorder.
- `config.ErrTLSDisabled` from `PrepareTLSConfig` means plain TCP; any other
  TLS error fails construction.
- `NewOPSServer` returns `(nil, nil)` when disabled; `WithService` skips nil.
  Conflicting paths are an `ErrOPSRouteConflict` error, not a `ServeMux` panic.
- `RegisterMetrics` uses one process-wide registry; the Go runtime collector is
  registered once. A health `Reader` that is a `prometheus.Collector` goes to a
  per-server registry.
- Health HTTP: GET/HEAD only, `Cache-Control: no-store`, kube-apiserver
  semantics (`?verbose`, `?exclude=`, `?format=json`, `/livez/<check>`),
  `/healthz` returns 200 when degraded. Without `WithHealth` probes answer 200
  (`staticReader`). Paths are normalized (`/livez/` → `/livez`).
- gRPC health: `""` and `readiness` follow `Ready`, `liveness` follows `Live`,
  every registered service follows `Ready` unless bound by
  `WithHealthService`; on drain the health server shuts down (permanent
  NOT_SERVING). Registering `grpc.health.v1` yourself together with
  `WithHealth` → `ErrGRPCHealthRegistered`.
- `network/http/aliases.go` and `network/grpc/aliases.go` let users import one package instead of `net/http` or
  `google.golang.org/grpc`; add a re-export there rather than asking users for
  a second import.
- `WithOpenTelemetry` is a minimal in-repo instrumentation (extract
  propagation, one server span). Do not pull in otelhttp/otelgrpc without a
  real need.

## Tests

`go test -race ./network/...`. Socket tests live in `*_integration_test.go`
and start with `testutil.RequireNetworkIntegration(t)`; handler tests build the
OPS mux with `newOPSHandler` without a socket. Tests that touch the OPS
registry must not run in parallel. See [testing](testing.md).
