# logger

Read when: touching `logger/`, log formats, secret masking, trace ids in logs,
the OTel log bridge. Also read `.github/instructions/logger.instructions.md` —
the pipeline order and the review findings that are known false positives.
User view: [Observability › Logging](../observability.md#logging).

## Files

| File | What it holds |
|---|---|
| `logger/doc.go` | package godoc |
| `logger/attributes.go` | type aliases to `slog` (`Logger`, `Attr`, `Handler`, …) and attr helpers (`String`, `Err`, `NamedError`, …) |
| `logger/default.go` | process default logger, `Init(cfg)`, `Default()`, package-level `Info`/`Error`/…, `formatFor` |
| `logger/logger.go` | `New`, `prepareTransformers` (the pipeline), `applyHandler` (`app` group) |
| `logger/handlers.go` | `wrappedHandler` (runs transformers, `Named` prefixes, `redactAttrs` for `With`), `Named` |
| `logger/options.go` | `Option`s for `Init`: level, output, handler, transformers, source |
| `logger/secrets.go` | `secretTransformer`: `REDACTED` by key, recursive into groups, resolves `LogValuer` |
| `logger/context.go` | `AddContextAttrs` and the transformer that merges them |
| `logger/tracing.go` | `openTracingTransform`: `trace` group, span event, error status |
| `logger/otel.go` | OTel log bridge (`SetOpenTelemetryBridge`), slog → OTel value mapping |
| `logger/console.go` | `NewConsoleHandler` (`LOGGER_FORMAT=console`, honors `NO_COLOR`) |
| `logger/testing.go` | `ForTests`, `TestLoggerWriteToTB`, `TestLoggerWriter`, `TestLoggerSecrets`, `NewSyncBuffer` |

## Pipeline

`wrappedHandler.Handle` runs, in order: context attrs → secrets → OTel bridge
(export) → `openTracingTransform` (only with `OpenTracingEnabled`) → caller
transformers → secrets again (only when both are set) → final handler, with
`[name:sub]` prefixes from `Named`. Exporters see the record as it is at their
step. Full contract: `.github/instructions/logger.instructions.md`.

## Invariants

- Nothing may export or print an attribute before a `secrets` pass. Attributes
  bound with `Logger.With` bypass transformers, so `WithAttrs` redacts them via
  `redactAttrs`. `LogValuer`s are resolved before redaction.
- `Logger` is `*slog.Logger`; the package adds behavior through the handler,
  not a wrapper type. Keep it that way so users can pass `*slog.Logger`.
- The default logger and the OTel bridge switch are process-wide atomics (see
  [architecture](architecture.md#process-wide-state)). Library code takes an
  explicit `*logger.Logger`; package-level helpers are for apps.
- `Init` never fails: a bad level or format logs a warning and falls back
  (level `info`, format `json`). Formats: `json` (default), `text`, `console`.
- The OTel bridge is toggled only by `tracer` (on with `send_logs`, off on
  shutdown). When it is on, `openTracingTransform` skips span events so a line
  is not shipped twice.
- Framework loggers are named `go-bones:<component>` via `Named`.
- `NewConsoleHandler` keeps the `slog` contract of `ReplaceAttr`: time (unless
  zero), level and msg go through it once, in `builtin`, before the prefix is
  printed; the inner `TextHandler` drops them so they are not printed twice.

## Tests

`go test -race ./logger/...`. Assert on output with `ForTests` +
`NewSyncBuffer`; `ForTests` masks `time`, so do not assert timestamps. OTel
assertions use `testutil.InstallOTelRecorder` ([testing](testing.md)).
Benchmarks: `logger/benchmark_test.go`.
