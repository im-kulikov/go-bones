# Observability

- [Logging](#logging)
- [OpenTelemetry](#opentelemetry)
- [Correlating logs and traces](#correlating-logs-and-traces)
- [Metrics: Prometheus or OTLP?](#metrics-prometheus-or-otlp)

## Logging

`logger` is `log/slog` with production defaults. `logger.Logger` is an alias of `slog.Logger`, so every slog method and any slog-compatible library works.

```go
log := logger.Init(cfg.Logger) // also becomes the process-wide default

log.Info("order created", logger.String("order_id", id), logger.Int("items", n))
log.Error("charge failed", logger.Err(err))

logger.Info("package-level helpers use the default logger")
```

**Request-scoped attributes** — add once, see them in every log line down the call chain:

```go
ctx = logger.AddContextAttrs(ctx,
	logger.String("request_id", reqID),
	logger.String("tenant", tenant),
)
log.InfoContext(ctx, "handled") // … request_id=… tenant=…
```

**Secret masking** — values of keys listed in `logger.secrets` are replaced with `REDACTED` everywhere a record can carry them: record attributes, `slog.Group`s (a group whose name is a secret is masked as a whole), attributes bound with `log.With`, `AddContextAttrs`, and groups returned by a `slog.LogValuer`. Masking runs before anything is exported to OTel:

```yaml
logger:
  secrets: [authorization, password, api_key]
```

**Output**: `LOGGER_FORMAT` is `json` by default — what log collectors expect in production. Use `console` for a colored, human-readable line locally (`NO_COLOR` turns colors off) or `text` for plain `key=value`.

Under systemd, set `LOGGER_FORMAT=journal` (`Environment=LOGGER_FORMAT=journal` in the unit): each line starts with the syslog priority (`<6>` info, `<4>` warning, …) instead of time and level, journald stores it as the entry priority, and `journalctl -p warning` filters by it. The format is never chosen automatically: `JOURNAL_STREAM` is set for any service under systemd, including those whose JSON is shipped on by a collector.

## OpenTelemetry

```go
telemetry := tracer.Init(log, cfg.Tracer) // a service: run it, it flushes on shutdown
```

Returns `nil` when tracing is disabled (`app.Init` handles that for you). When enabled, it builds the resource from `OTEL_RESOURCE_ATTRIBUTES`/`OTEL_SERVICE_NAME` plus SDK info — falling back to the app name and version from config — and installs the propagators (`OTEL_PROPAGATORS`, default `tracecontext,baggage`), the `TracerProvider`, and optionally the `MeterProvider` and `LoggerProvider`.

Tracing turns on with `OTEL_ENABLED=true`, `OTEL_SEND_METRICS`/`OTEL_SEND_LOGS`, or any of `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`, `OTEL_EXPORTER_OTLP_[SIGNAL_]ENDPOINT`. `OTEL_SDK_DISABLED=true` turns it off regardless.

Configuration follows the **standard OpenTelemetry environment variables** — the same ones every OTel SDK and the Collector docs use:

```bash
OTEL_ENABLED=true
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318
OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=prod
OTEL_SEND_METRICS=true
OTEL_SEND_LOGS=true
```

The `tracer.*` config keys are only a fallback for when the standard variables are absent ([reference](configuration.md#tracer)). With `insecure: true` and a non-local endpoint, go-bones logs a warning per exported signal; only the endpoint host is logged, never userinfo or query parameters.

Transport instrumentation:

| Where | How |
|---|---|
| Incoming HTTP | `http.WithOpenTelemetry()` |
| Incoming gRPC | `grpc.WithOpenTelemetry()` |
| Outgoing HTTP / gRPC | `otelhttp` / `otelgrpc` from opentelemetry-go-contrib — see [transports](transports.md#instrumenting-your-own-code) |
| Your code | `otel.Tracer("orders").Start(ctx, "charge")` — plain OTel API |

## Correlating logs and traces

Always log with the `*Context` methods inside a request — that is how the logger finds the active span. Two independent switches:

- `logger.open_tracing: true` (`LOGGER_OPEN_TRACING_ENABLED`) — every record written inside a recording span gets a `trace` group with `trace_id` and `span_id`, an `ERROR` record marks the span as failed, and, while the log bridge below is off, the record is also attached to the span as a `log` event:

  ```text
  level=INFO msg="order created" order_id=42 trace.trace_id=4bf92f3577b34da6a3ce929d0e0e4736 trace.span_id=00f067aa0ba902b7
  ```

- `tracer.send_logs: true` (`OTEL_SEND_LOGS`) — every record is also exported as an OTLP log record through the OTel `LoggerProvider`, linked to the span from its context. This works whether `open_tracing` is on or not; span events are then skipped so the same line isn't shipped twice.

Either way you can jump from a trace to its logs and back in Grafana or Jaeger.

## Metrics: Prometheus or OTLP?

| | Prometheus (`/metrics` on ops) | OTLP (`OTEL_SEND_METRICS`) |
|---|---|---|
| Model | pull | push |
| Includes | Go runtime and process collectors, health checks, your collectors | OTel instruments you create |
| Pick when | you already scrape pods | you route everything through a Collector |

They are independent; enabling both is fine.

