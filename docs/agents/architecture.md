# Architecture map

Read when: first contact with the repo, or a change that spans packages.
Per-package internals live in the sibling docs; user-visible behavior lives in
[`docs/`](../README.md).

## Layers

Arrows are imports inside the module. Imports only go down.

```text
app                         composition root, optional, nothing imports it
 ├─ network/http ─┐
 ├─ network/grpc ─┼─► service ─► health ─► logger ─► config ─► bones (root)
 └─ tracer ───────┘      │          │                  └─► gonfig
                         └──────────┴─► internal, network (leaf helpers)
```

Exact edges: `service` → `health`, `logger`, `internal`, `bones`, `config`
(only in `service/testing.go`); `health` → `config`, `logger`, `bones`;
`logger` → `config`; `config` → `bones`; transports and `tracer` → `service`,
`config`, `logger`, `internal` (+ `health`, `network` for transports).

Rules:

- `health` must not import `service`: `service` imports `health`, and
  `*health.Monitor` satisfies `service.Service` structurally.
- Library packages must work without `app` (manual wiring, see
  [Getting started › Manual wiring](../getting-started.md#manual-wiring)).
- `internal/` is not public API; `internal/testutil` is imported from
  `_test.go` files only.

## Packages

| Package | Purpose | Agent doc |
|---|---|---|
| `bones` (root) | `Error` (constant errors), `ExtractError` | this page |
| `app` | `Init`/`Add`/`Run` facade with production defaults | this page, [service](service.md) |
| `config` | config structs and `Load` on top of gonfig | [config](config.md) |
| `logger` | `log/slog` + secret masking, trace ids, OTel log bridge | [logger](logger.md) |
| `service` | `Service` contract, `Run` with phased shutdown, launcher, ticker, `Env`/`Get`/`Build` | [service](service.md) |
| `health` | non-blocking health monitor | [health](health.md) |
| `network`, `network/http`, `network/grpc` | servers, OPS server, health endpoints | [transports](transports.md) |
| `tracer` | OpenTelemetry bootstrap | [tracer](tracer.md) |
| `internal`, `internal/testutil` | shutdown helpers; test helpers | this page, [testing](testing.md) |

Small files documented here:

| File | What it holds |
|---|---|
| `doc.go` | godoc of the root package `bones` |
| `error.go` | `bones.Error` string type for `const ErrX bones.Error = "..."`, `ExtractError` |
| `internal/graceful.go` | `GracefulShutdown`, `LazyGracefulShutdown`: run a shutdown func with a detached, bounded ctx |
| `internal/timeout.go` | `FallbackTimeout`: non-positive duration → 15s |
| `app/app.go` | the whole facade, see below |

## Cross-package contracts

| Contract | Defined in | Implemented / consumed by |
|---|---|---|
| `Service{Name; Start(ctx) error; Stop(ctx)}` | `service/routine.go` | launcher, ticker, `health.Monitor`, HTTP/gRPC/OPS servers, tracer |
| `Enabler{Enabled() bool}` | `service/routine.go` | disabled services are skipped with a warning |
| `health.Checker` = `service.HealthChecker` | `health/types.go` | services implementing it are auto-registered by `service.WithHealth` |
| `health.Configurer{HealthOptions()}` | `health/types.go` | tunes an auto-registration (impact, intervals) |
| `health.Reader{Snapshot; Subscribe}` | `health/types.go` | `http.WithHealth`, `grpc.WithHealth` |
| `config.INetwork{Addr; Base}` | `config/base.go` | `config.HTTP`, `config.GRPC`, `config.Ops` → `NewServer` |
| `Env`, `Constructor[C,T]`, `Get[T]`, `Build` | `service/env.go` | `app.Add`, component constructors, `service.TestEnv` |

`Start` blocks until the service is done. Any return, `nil` included, stops
the whole application; non-ignored errors are joined into `Run`'s result.

## Application lifecycle

`app/app.go`, one application per process (`std`, like `flag.CommandLine`):

1. `Init[C]` — `config.Load` (defaults → file → env → flags, name/version from
   build info) → `logger.Init` (replaces the process default logger) →
   `health.New` → `http.NewOPSServer(…, http.WithHealth(monitor))` (nil when
   disabled) → `service.SignalContext(SIGINT, SIGTERM)` becomes `Env.Context` →
   `tracer.Init` is the first service in the list (nil when disabled).
2. `Add(cfg, ctor)` — `service.Build`; on error logs `file:line` of the call and
   exits 1; a result implementing `service.Service` is appended.
3. `Run()` — `service.RunContext(Env.Context, …)` with `WithHealth(monitor)`,
   `WithDrainDelay(health.drain_delay)`, `WithShutdownLast(ops)`, the services,
   then OPS. Failure exits 1. Running under the signal context of `Init` is
   what stops it on a signal received while components were built: `Init`'s
   subscription consumed that signal, `Run`'s own one never sees it.

Exits go through the `exit` variable so tests can replace `os.Exit`. `Add`/`Run`
before `Init` panic.

## Process-wide state

Globals are deliberate and marked `//nolint:gochecknoglobals` with a reason.
Tests that touch them restore the old value in `t.Cleanup` and do not run in
parallel. Prefer a struct field seam over a new global.

| State | Where | Set by |
|---|---|---|
| application `std`, `exit` seam | `app/app.go` | `app.Init`, tests |
| default logger (`atomic.Pointer`) | `logger/default.go` | `logger.Init` |
| log format registry (`sync.Map`) | `logger/default.go` | `logger.RegisterFormat`, from `main` |
| OTel log bridge switch | `logger/otel.go` | `tracer` via `logger.SetOpenTelemetryBridge` |
| Prometheus registry, runtime collector flag | `network/http/ops.go` | `RegisterMetrics`, `NewOPSServer` |
| OTel tracer/meter/logger providers, propagator | OTel globals | `tracer.Init` |
| exporter/provider constructor seams | `tracer/tracer.go` | tests |
| default signals and ignored errors | `service/routine.go` | read-only |

## Feature → code

| Feature (user doc) | Code |
|---|---|
| `/livez` `/readyz` `/healthz` ([health](../health.md#http-endpoints)) | `network/http/health.go` over `health.Snapshot` |
| gRPC `grpc.health.v1` ([health](../health.md#grpc-health)) | `network/grpc/health.go` |
| drain on SIGTERM, OPS stops last ([lifecycle](../lifecycle.md#shutdown-and-drain)) | `service/health.go` `phasedRun`, `drain` |
| poll / push / heartbeat checks, thresholds, staleness | `health/check.go`, `health/push.go` |
| health metrics `go_bones_health_*` | `health/metrics.go`, served per OPS server by `network/http/ops.go` `metricsHandler` |
| periodic tasks | `service/ticker.go` |
| components by type | `service/env.go` |
| `--config`, `--help`, `--print-config` | gonfig flags embedded in `config.Base` (`config/alias.go`) |
| version from VCS without ldflags | `config/config.go` `buildVersion` |
| secret masking in logs | `logger/secrets.go`, `logger/handlers.go` `redactAttrs` |
| log formats `json`/`text`/`console`/`journal`, your own via `RegisterFormat` | `logger/default.go` `formatFor`, `logger/console.go` |
| `trace_id`/`span_id` in log lines | `logger/tracing.go` (only with `logger.open_tracing`) |
| OTLP logs/metrics/traces | `tracer/tracer.go`, `logger/otel.go` |
| `/metrics`, pprof, expvar, `/version` ([ops](../ops.md)) | `network/http/ops.go` |
| TLS / mTLS | `config/tls.go` |
