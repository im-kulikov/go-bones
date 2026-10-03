# tracer

Read when: touching `tracer/` or OTel export. Also read
`.github/instructions/tracer.instructions.md` (known non-issues about
endpoint masking). User view:
[Observability › OpenTelemetry](../observability.md#opentelemetry),
[Configuration › tracer](../configuration.md#tracer).

## Files

| File | What it holds |
|---|---|
| `tracer/doc.go` | package godoc |
| `tracer/tracer.go` | `Init` (a launcher service or nil), provider/exporter construction, resource, propagators, `OTEL_*` env lookups, insecure-endpoint warning |

## Invariants

- Env-first: standard `OTEL_*` variables win; `config.TracerConfig`
  (`OTEL_ENABLED`, `OTEL_SEND_LOGS`, `OTEL_SEND_METRICS`, `OTEL_ENDPOINT`,
  `OTEL_INSECURE`, `OTEL_USE_HTTP`) is only a fallback.
- Enabled when `Enabled`, `SendMetrics` or `SendLogs` is set, or a standard
  endpoint/service-name/resource variable is present; `OTEL_SDK_DISABLED=true`
  always disables. Disabled → `Init` returns nil (skipped by `WithService`).
- The service sets OTel globals (tracer, meter, logger providers, propagator)
  and switches `logger.SetOpenTelemetryBridge`; shutdown hooks flush providers
  with a 10s budget and switch the bridge off.
- Traces are always on when enabled; metrics and logs are opt-in. OTLP metrics
  are independent from the OPS Prometheus endpoint.
- `endpointHost` is a whitelist (IP or DNS name, otherwise `[redacted]`), so
  the insecure warning never logs credentials.
- Constructors are reached through `newXxxFunc` package seams for tests.

## Tests

`go test -race ./tracer/...`. Tests set env with `t.Setenv` and replace the
seams; they must not run in parallel. Exporter round-trips are in
`tracer/tracer_integration_test.go`.
