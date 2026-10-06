# health

Read when: touching `health/`, probe semantics, health metrics or events.
User view: [Health checks](../health.md) (impact, check kinds, endpoints,
metrics, "Monitor behavior in detail").

## Files

| File | What it holds |
|---|---|
| `health/doc.go` | package godoc: impact semantics, leaf-package rule |
| `health/types.go` | `Checker`, `CheckerFunc`, `Configurer`, `Reader`, `Impact`, `Status`, `Overall`, `Result`, `Snapshot` (+ `Without`, `aggregate`), `Event`, errors, `PublicError`/`PublicMessage` |
| `health/options.go` | registration `Option`s (`WithImpact`, `WithInterval`, `WithTimeout`, `WithThresholds`, `WithTTL`, …), `registration`, validation, `staleAfter` |
| `health/monitor.go` | `Monitor`: `New`, `Register`, `Status`, `Heartbeat`, `Trigger`, `Start`/`Stop`/`Drain`, immutable `state` publishing, `Snapshot` |
| `health/check.go` | scheduler (`schedule`, `run`, `classify`), push TTL watcher (`watch`, `sweep`), thresholds (`record`), transitions and logs (`applyLocked`) |
| `health/push.go` | `StatusHandle.Set`, `Heartbeat.Beat` (atomic fast path) |
| `health/subscribe.go` | `Subscribe`: goroutine per subscriber, 64-event drop-oldest queue |
| `health/metrics.go` | `go_bones_health_*` metrics; `Monitor` is a `prometheus.Collector` |

## Model

Registrations (polled `Register`, push `Status`, liveness `Heartbeat`) are
added before `Start`. The monitor owns all I/O in its goroutines and publishes
an immutable `state` through an `atomic.Pointer` after every change.
`Snapshot()` copies it and evaluates staleness at read time. Aggregation:
`Live` = running and no liveness check down; `Ready` = live, not draining and
every readiness check up; `Overall` adds `degraded` for informational failures.

## Invariants

- Readers never do I/O or wait for a checker: `Snapshot`, HTTP/gRPC adapters
  and `Collect` read the published state only.
- Registration after `Start` → `ErrMonitorStarted`; names are unique.
- `Start` after `Stop` returns `ErrMonitorStopped`, or `ctx.Err()` when its
  context is already done: on shutdown the runner may stop the monitor before
  its `Start` goroutine ran, and that must not fail the application.
- At most one `Check` call per registration is in flight; a timeout is
  recorded at the deadline without waiting for the checker; a panic becomes
  `ErrPanic`; cancellations during shutdown are not recorded.
- The first outcome sets the status at once; later flips need
  `FailureThreshold`/`SuccessThreshold` consecutive outcomes. Exception: a
  failure within `StartPeriod` after `Start` keeps an unknown check unknown
  (`health/check.go` `starting`).
- Unknown is neither up nor down: an unknown readiness check keeps `Ready`
  false, an unknown liveness check keeps `Live` true.
- `Drain` is permanent and idempotent; `Stop` drains first.
- Events are emitted after the state is stored and are hints only:
  subscribers re-read `Snapshot`.
- Raw `Result.Err` stays in logs; over the network only `PublicMessage`
  (classified text or a `PublicError` message) is exposed.
- Liveness impact is for internal state (heartbeats), never for external
  dependencies.
- A global `StaleAfter` never undercuts a check's own interval + jitter +
  timeout (`staleAfter`).
- `sweep` clears `up` before reading `lastBeat`; this ordering fixes a race
  with `Beat`'s fast path — do not reorder.
- Health metrics are registered in a registry owned by each OPS server, not
  the shared one (`network/http/ops.go` `metricsHandler`).

## Tests

`go test -race ./health/...`. Use `testing/synctest` and set
`m.jitter = func(d time.Duration) time.Duration { return d }` (see
`newTestMonitor` in `health/monitor_test.go`). Examples:
`health/example_test.go`. Endpoint behavior is tested in `network/http` and
`network/grpc`.
