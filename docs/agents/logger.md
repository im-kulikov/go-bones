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
| `logger/default.go` | process default logger, `Init(cfg)`, `Default()`, package-level `Info`/`Error`/…, `formatFor`, `RegisterFormat` |
| `logger/logger.go` | `New`, `prepareTransformers` (the pipeline), `applyHandler` (`app` group) |
| `logger/handlers.go` | `wrappedHandler` (runs transformers, `Named` prefixes, `redactAttrs` for `With`), `Named` |
| `logger/options.go` | `Option`s for `Init`: level, output, handler, transformers, source |
| `logger/secrets.go` | `secretTransformer`: `REDACTED` by key, recursive into groups, resolves `LogValuer` |
| `logger/context.go` | `AddContextAttrs` and the transformer that merges them |
| `logger/tracing.go` | `openTracingTransform`: `trace` group, span event, error status |
| `logger/otel.go` | OTel log bridge (`SetOpenTelemetryBridge`), slog → OTel value mapping |
| `logger/console.go` | `NewConsoleHandler` (`LOGGER_FORMAT=console`, honors `NO_COLOR`), `newJournalHandler` (`LOGGER_FORMAT=journal`) |
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
  (level `info`, format `json`). Formats: `json` (default), `text`, `console`,
  `journal`.
- The OTel bridge is toggled only by `tracer` (on with `send_logs`, off on
  shutdown). When it is on, `openTracingTransform` skips span events so a line
  is not shipped twice.
- Framework loggers are named `go-bones:<component>` via `Named`.
- `NewConsoleHandler` keeps the `slog` contract of `ReplaceAttr`: time (unless
  zero), level and msg go through it once, in `builtin`, before the prefix is
  printed; the inner `TextHandler` drops them so they are not printed twice.
  It drops by position, not key: slog passes time, level, source, msg before
  the record attributes, so the `builtins` flag (set in `Handle`, cleared at
  msg) keeps user attributes keyed `msg`/`time`/`level`. `WithAttrs` takes
  `mu` because the inner handler runs `ReplaceAttr` there too.
  Line breaks in the message are escaped (`oneLine`): one record, one line;
  attribute values are already quoted by the `TextHandler`.
- Formats go-bones does not ship are registered by the service with
  `RegisterFormat` before `Init` (a `sync.Map`; built-in names, duplicates and
  a nil constructor panic). The registered handler is the final one behind
  the pipeline, like the built-ins. Do not add more built-in formats for one
  user: point them to `RegisterFormat` (journald fields: systemd/slog-journal,
  example in [Observability](../observability.md#logging)).
- `journal` is the console handler with `journal` set: no colors, no time or
  level, the `<N>` priority glued to the line with no space (journald would
  keep the space in MESSAGE). Never picked from `JOURNAL_STREAM`: that would
  change the output of services already under systemd.

## Tests

`go test -race ./logger/...`. Assert on output with `ForTests` +
`NewSyncBuffer`; `ForTests` masks `time`, so do not assert timestamps. OTel
assertions use `testutil.InstallOTelRecorder` ([testing](testing.md)).
Benchmarks: `logger/benchmark_test.go`.
