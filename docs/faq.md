# FAQ & troubleshooting

- [How does it compare to other libraries?](#how-does-it-compare-to-other-libraries)
- [Do I have to use the `app` facade?](#do-i-have-to-use-the-app-facade)
- [Isn't `service.Get` a DI container?](#isnt-serviceget-a-di-container)
- [Can I use my own router / logger / metrics?](#can-i-use-my-own-router--logger--metrics)
- [My pod never becomes ready](#my-pod-never-becomes-ready)
- [Pods get restarted when the database is down](#pods-get-restarted-when-the-database-is-down)
- [I still see 502s during deploys](#i-still-see-502s-during-deploys)
- [Traces stop at my service](#traces-stop-at-my-service)
- [No `trace_id` in logs](#no-trace_id-in-logs)
- [`duplicate metrics collector registration`](#duplicate-metrics-collector-registration)
- [`/metrics` returns 500](#metrics-returns-500)

## How does it compare to other libraries?

| | go-bones | uber-go/fx | go-kratos | go-kit | oklog/run |
|---|---|---|---|---|---|
| Kind | Operational skeleton | DI + lifecycle | Full framework | Toolkit of patterns | Lifecycle only |
| Dependency injection | constructors + `service.Get` (no container) | ✓ | ✓ (wire) | ✗ | ✗ |
| Code generation | ✗ | ✗ | ✓ | ✗ | ✗ |
| Health probes + drain | ✓ built in | DIY | partial | DIY | DIY |
| OpenTelemetry bootstrap | ✓ | DIY | ✓ | DIY | ✗ |
| Touches your business code | no | via DI graph | yes (layout, proto) | yes (endpoints) | no |

go-bones combines well with most of them — e.g. use `service.Run` alongside fx, or plug a kratos/ConnectRPC handler into `http.WithHandler`.

## Do I have to use the `app` facade?

No. It is a small file that wires `config`, `logger`, `tracer`, `health`, the ops server and `service`. Every package works on its own — see [Manual wiring](getting-started.md#manual-wiring). Many teams start with just `service` + `health` + the ops server. Component constructors (`New(cfg, env)`) work with and without the facade.

## Isn't `service.Get` a DI container?

No graph, no reflection, no lifetimes. `app.Add` runs constructors in the order you write them; `service.Get[T]` looks through what was already built with a type assertion and fails with a clear message if nothing or more than one value matches. Ordering stays your code, errors appear at startup before any port opens. If you need a real container, use uber-go/fx and plug go-bones services into its lifecycle.

## Can I use my own router / logger / metrics?

- **Router**: yes, anything that is an `http.Handler`.
- **Logger**: go-bones uses `log/slog`. Wrap your handler with `logger.New(cfg, yourHandler)`.
- **Metrics**: any `prometheus.Collector` — register it with `http.RegisterMetrics` (the ops registry is not `prometheus.DefaultRegisterer`). For OTel instruments, enable `OTEL_SEND_METRICS`.

## My pod never becomes ready

```bash
kubectl port-forward pod/orders-xyz 8090
curl 'localhost:8090/readyz?verbose'
```

- `[-]postgres failed: timeout` — the check exceeds `health.timeout`; check the client's own timeout and network policy.
- `[-]postgres failed: stale` — the check is stuck in flight; see `go_bones_health_check_skipped_total{reason="in_flight"}`.
- `unknown` — the check has not completed once yet; `startupProbe.failureThreshold` may be too low.
- Everything `[+]` but still 503 — the process is draining (it received SIGTERM).

## Pods get restarted when the database is down

A check depending on something outside the process has `Liveness` impact. Change it to `Readiness` or `Informational`. See [Impact](health.md#impact).

## I still see 502s during deploys

1. Is `HEALTH_DRAIN_DELAY` set (≈5s)?
2. Is `terminationGracePeriodSeconds` > `drain_delay + shutdown_timeout`?
3. Does your ingress/LB use the pod's readiness (it must not route to pods by IP ignoring endpoints)?
4. Is there also a `preStop: sleep`? Remove one of the two.

## Traces stop at my service

Incoming requests need `http.WithOpenTelemetry()` / `grpc.WithOpenTelemetry()` on the server, and outgoing calls need an instrumented client — `otelhttp.NewTransport` or `otelgrpc.NewClientHandler` from opentelemetry-go-contrib. See [Instrumenting your own code](transports.md#instrumenting-your-own-code). Check `OTEL_PROPAGATORS` too: `none` turns propagation off.

## No `trace_id` in logs

- `logger.open_tracing: true` (`LOGGER_OPEN_TRACING_ENABLED=true`)? The IDs are in the `trace` group: `trace.trace_id`, `trace.span_id`.
- Are you logging with `InfoContext(ctx, ...)` and passing the request's `ctx`?
- Is `OTEL_ENABLED=true`? Without a tracer provider there are no spans.

## `duplicate metrics collector registration`

`http.RegisterMetrics` returned this error: the ops registry is process-wide and the same collector (or one with the same name and labels) was registered twice — often from a constructor that runs more than once, e.g. in tests. Register at startup, once (a package-level `sync.Once` works), or keep test-only collectors in a `prometheus.NewRegistry()` of your own. Health metrics never cause this: each OPS server keeps its monitor in its own registry.

## `/metrics` returns 500

One of your collectors exports a metric with the same name as a `go_bones_health_*` metric. The response body names it; rename your metric. go-bones reports the clash on scrape instead of silently dropping one of the two.
