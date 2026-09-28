# Ops server

A separate HTTP server for everything that is not your API: probes, metrics, profiling. Default address `:8090`.

| Path | What | Enabled by default |
|---|---|---|
| `/livez`, `/readyz`, `/healthz` | [Health](health.md#http-endpoints) | ✅ |
| `/metrics` | Prometheus, incl. full Go runtime metrics | ✅ |
| `/version` | Go build info: module, version, dependencies, build settings incl. VCS revision; `?format=json` | — |
| `/debug/pprof/*` | pprof | ✅ |
| `/debug/vars` | expvar | ✅ |

With the `app` facade the ops server is created by `app.Init` with the health monitor attached and stopped last. By hand: `http.NewOPSServer(cfg.OpsServer, log, http.WithHealth(hc))`.

## Security

The ops port must **never** be exposed publicly: pprof can stall the process and reveals internals, `/healthz` lists your dependencies.

- Don't add the ops port to a public Service / Ingress; use a separate `ClusterIP` Service or scrape pod IPs directly.
- pprof and expvar are **on by default** and have no authentication. Turn them off where you don't need them, and reach them through `kubectl port-forward` when you do:

```bash
OPS_PROFILE_ENABLED=false OPS_EXP_VARS_ENABLED=false ./orders
kubectl port-forward pod/orders-xyz 8090 && go tool pprof -http=: localhost:8090/debug/pprof/heap
```

## Metrics

Runtime metrics use the Prometheus Go collector with `collectors.MetricsAll`: scheduler, GC, memory classes, goroutines, mutex wait and more.

Register your own collectors before the server starts:

```go
ordersCreated := prometheus.NewCounter(prometheus.CounterOpts{
	Name: "orders_created_total",
	Help: "Orders created.",
})
if err := http.RegisterMetrics(ordersCreated); err != nil {
	return err // a duplicate registration is an error: register once, at startup
}
```

The registry is go-bones' own, process-wide, shared by every OPS server in the process. It is **not** `prometheus.DefaultRegisterer`: collectors registered with `promauto` or `prometheus.MustRegister` do not show up on `/metrics` — pass them to `http.RegisterMetrics`. Health metrics are the exception: each OPS server serves its own monitor from a private registry, see [Health › Metrics](health.md#metrics-and-alerts).

Prometheus metrics (ops) and OpenTelemetry metrics (OTLP push) are independent paths — use either or both. See [Observability](observability.md#metrics-prometheus-or-otlp).

## Profiling cheatsheet

```bash
go tool pprof -http=: localhost:8090/debug/pprof/profile?seconds=30   # CPU
go tool pprof -http=: localhost:8090/debug/pprof/heap                 # memory
curl -s localhost:8090/debug/pprof/goroutine?debug=2 | less           # goroutine dump
```

With `GOEXPERIMENT=goroutineleakprofile` (Go 1.26) there is also `/debug/pprof/goroutineleak`.

## Configuration

All keys: [Configuration › ops](configuration.md#ops). Disable the whole server with `OPS_ENABLED=false`. In code and tests start from `config.Defaults[config.Ops]()`, not from a literal.

`http.NewOPSServer` returns:

- `nil, nil` when the server or all its endpoints are disabled — `service.Run` skips a nil service, so you can pass it along unconditionally;
- `http.ErrOPSRouteConflict` when two enabled endpoints end up on the same path (for example `live_path` and `ready_path` both `/`, or a probe on `metrics_path`), instead of the `ServeMux` panic.
