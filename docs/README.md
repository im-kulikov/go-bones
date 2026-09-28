# go-bones documentation

Pick a path:

**I want to try it** → [Getting started](getting-started.md) — a running service in 5 minutes.

**I'm putting it in production** → [Health](health.md) → [Lifecycle](lifecycle.md) → [Kubernetes](kubernetes.md).

**I'm looking something up** → [Configuration reference](configuration.md).

## Guides

| Page | Read it when |
|---|---|
| [Getting started](getting-started.md) | First service, components, testing a component, and wiring without the `app` facade |
| [Configuration](configuration.md) | Config structs, load order, file/env/flags, full env reference |
| [Lifecycle](lifecycle.md) | Services, workers, signals, graceful shutdown and drain |
| [HTTP & gRPC](transports.md) | Servers, TLS, instrumenting handlers and clients |
| [Health checks](health.md) | Readiness vs liveness, writing checks, endpoints, metrics |
| [Ops server](ops.md) | `/metrics`, pprof, expvar, `/version`, securing the port |
| [Observability](observability.md) | Logging, OpenTelemetry, correlating logs and traces |
| [Kubernetes](kubernetes.md) | Probes, grace periods, a complete Deployment |
| [Testing](testing.md) | Test logger, testing components with `service.TestEnv`, health checks in tests |
| [FAQ & troubleshooting](faq.md) | Comparisons, common mistakes, "why is my pod not ready?" |

## Conventions in these docs

- Snippets are trimmed to the point: imports and error handling are often omitted. Runnable versions are the godoc `Example`s in each package.
- `http` and `grpc` in snippets mean `github.com/im-kulikov/go-bones/network/http` and `.../network/grpc` — they re-export the stdlib/gRPC types you need, so one import is enough.
- Config keys are shown as YAML; the env name is usually the upper-cased path joined with `_` (`ops.metrics_enabled` → `OPS_METRICS_ENABLED`). Exceptions, such as `tracer.*` → `OTEL_*`, are listed in the [configuration reference](configuration.md#reference).
