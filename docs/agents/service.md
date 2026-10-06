# service

Read when: touching `service/` or `app/` — the runner, shutdown phases,
launcher, ticker, component wiring. Also read
`.github/instructions/service.instructions.md` (known non-issues).
User view: [Lifecycle](../lifecycle.md), [Getting started](../getting-started.md).

## Files

| File | What it holds |
|---|---|
| `service/doc.go` | package godoc |
| `service/routine.go` | `Service`, `Enabler`, `Run`/`RunContext`, classic `run`, error joining (`containsError`, `result`), default signals and ignored errors |
| `service/health.go` | `WithHealth`, `WithDrainDelay`, `WithShutdownLast`, `WithLauncherHealthCheck`, `phasedRun`, `drain`, `stopServices`, auto-registration in `prepareHealth` |
| `service/options.go` | `WithService` (unwraps `Compose`, skips nil/disabled), `WithShutdownTimeout`, `WithIgnoreError`, `WithLoggerPingPong` |
| `service/launcher.go` | `NewLauncher`: one-shot `Service` around a func, panic guard, shutdown hooks |
| `service/ticker.go` | `NewTicker`: periodic task on top of a launcher |
| `service/group.go` | `Compose`: a container unwrapped by `WithService` |
| `service/signal.go` | `SignalContext`, `ErrOsSignal`, `ErrCancelCalled` |
| `service/env.go` | `Env`, `Constructor`, `Get[T]`, `Build` |
| `service/testing.go` | `TestEnv(t, fakes...)` |
| `service/ping_pong.go` | debug heartbeat service for `WithLoggerPingPong` |

## Run

- Classic path (no health options): every service gets the signal context;
  on cancel all are stopped concurrently within `WithShutdownTimeout`
  (default 15s via `internal.FallbackTimeout`).
- Phased path (`WithHealth`, `WithDrainDelay` or `WithShutdownLast`):
  `trigger` (signals/parent) is separate from `serve` and `last` contexts,
  which are detached from the parent and canceled up front if it is already
  done. Order: `Monitor.Drain()` → drain delay (only for an OS signal; a second
  signal skips it) → cancel+stop regular services → cancel+stop "last".
- Health registration errors are returned before any service starts.
- Result: `errors.Join` of every non-ignored `Start` error plus the parent's
  cause unless ignored or already reported. Ignored by default:
  `ErrOsSignal`, `ErrCancelCalled`, `context.Canceled`,
  `context.DeadlineExceeded`. `containsError` also looks inside joined errors.

## Invariants

- A service's `Start` returning (even `nil`) stops the app. `Stop` is
  best-effort, bounded by its ctx, and reports its own failures in logs.
- `WithShutdownTimeout` is one deadline shared by both stop groups; "last"
  gets what is left, possibly nothing. `stopServices` waits for every `Stop`
  without a select of its own, so a `Stop` must honor its ctx. `Run` returns
  only after every `Start` has returned and never interrupts a callback.
- Launcher is one-shot: `Start` runs the callback once and stores its error for
  every waiter; hooks run in `Start`'s goroutine strictly after the callback
  returns; `Stop` before `Start` is a no-op; `Stop` waits for the cancel func
  to be published even with an expired ctx (gRPC's fire-and-forget stop relies
  on it); a panic becomes `ErrLauncherPanicked`.
- Hooks get `context.WithoutCancel` — by design, not a bug (see the
  instructions file).
- `WithHealth` + `WithService(monitor)` is harmless: the duplicate is removed
  with `sameService`, which never compares non-comparable types.
- `Get` panics with `dependencyError`; `Build` recovers only that and returns
  it with the constructor name; other panics propagate, so a constructor
  called directly (not via `Build`) panics on a missing dependency. Exactly one
  value in `Env` must be assignable to `T`. A nil interface result means a
  disabled component and is not stored; a typed nil (`(*T)(nil)`) is an error
  wrapping `ErrNilComponent` (`typedNil`). A nil constructor fails with
  `ErrNilConstructor` before anything runs, so `app.Add` reports its file:line.
  `Build` appends to `Env` without a lock on purpose: components are built in
  order, concurrent builds would make `Get` results depend on timing.
- `NewTicker` runs immediately, never overlaps runs, logs task errors and keeps
  going, panics on a non-positive interval. `WithTickerJitter` clamps to
  [0, 1] and turns NaN into 0: a NaN jitter makes every interval negative and
  spins the timer. For the same reason `next` falls back to the plain interval
  when the jittered one is not positive (it wraps past MaxInt64). The timer branch checks `ctx.Err()` first: when a tick and
  the cancellation are ready together `select` picks at random, so without it
  a task could run once after shutdown.
- `WithLauncherHealthCheck` returns `checkedLauncher`, which implements
  `HealthChecker` and `health.Configurer`. Its options are cloned when stored
  and when handed out, like the shutdown hooks.

## Tests

`go test -race ./service/...`. Timing tests use `testing/synctest`; signal
handling is injected through `settings.newSignal` and `settings.notify`. See
[testing](testing.md). Examples: `service/example_test.go`.
