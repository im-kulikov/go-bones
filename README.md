<p align="center">
  <img src=".github/logo@2.png" alt="go-bones" width="360">
</p>

<p align="center">
  <b>The production skeleton for Go services.</b><br>
  Health probes, graceful drain, OpenTelemetry and ops endpoints — wired correctly in a few lines.<br>
  Stdlib-first. No DI container, no reflection, no codegen.
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/im-kulikov/go-bones"><img src="https://pkg.go.dev/badge/github.com/im-kulikov/go-bones.svg" alt="Go Reference"></a>
  <a href="https://github.com/im-kulikov/go-bones/actions/workflows/go.yml"><img src="https://github.com/im-kulikov/go-bones/actions/workflows/go.yml/badge.svg" alt="CI"></a>
  <a href="https://codecov.io/gh/im-kulikov/go-bones"><img src="https://img.shields.io/codecov/c/github/im-kulikov/go-bones?style=flat" alt="Coverage"></a>
  <a href="https://goreportcard.com/report/github.com/im-kulikov/go-bones"><img src="https://goreportcard.com/badge/github.com/im-kulikov/go-bones" alt="Go Report Card"></a>
  <a href="https://github.com/im-kulikov/go-bones/releases"><img src="https://img.shields.io/github/v/release/im-kulikov/go-bones?sort=semver" alt="Release"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/im-kulikov/go-bones" alt="Go version">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/im-kulikov/go-bones" alt="License"></a>
</p>

<p align="center">
  <a href="docs/getting-started.md">Getting started</a> ·
  <a href="docs/README.md">Docs</a> ·
  <a href="https://pkg.go.dev/github.com/im-kulikov/go-bones">API reference</a> ·
  <a href="https://github.com/im-kulikov/go-bones/releases">Releases</a>
</p>

---

## Why go-bones?

Every Go service ends up with the same couple of hundred lines in `main.go`: load config, set up logging, start servers, handle SIGTERM, expose `/metrics` and health endpoints, plug in tracing. Most of them get the hard parts subtly wrong:

- **502s on every deploy** — the pod stops accepting connections before the load balancer notices.
- **A `/health` that takes the cluster down** — it pings the database, the database hiccups, every replica gets restarted at once.
- **Probes that hang** — a stuck dependency makes `/ready` time out instead of answering "not ready".
- **Traces without logs, logs without traces** — `trace_id` never makes it into the log line.

go-bones is that `main.go`, done once and done right, split into small packages you can use together or one by one.

## Quick start

```bash
go get github.com/im-kulikov/go-bones@latest
```

```go
package main

import (
	"github.com/im-kulikov/go-bones/app"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/network/http"
	"github.com/im-kulikov/go-bones/service"
)

type settings struct {
	config.Base // logger, ops, tracer, health

	API config.HTTP `env:"API" yaml:"api"`
}

// newAPI is a component constructor: its config section and the application environment.
func newAPI(cfg config.HTTP, env service.Env) (service.Service, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello, world"))
	})

	return http.NewServer(cfg, env.Logger, http.ServiceName("api"), http.WithHandler(mux))
}

func main() {
	cfg := app.Init[settings](config.WithVersion("1.0.0"))
	app.Add(cfg.API, newAPI)
	app.Run() // blocks until SIGTERM, then drains and stops everything in order
}
```

```console
$ LOGGER_FORMAT=console go run .
$ curl localhost:8080/hello          # your API
$ curl 'localhost:8090/readyz?verbose' # probes
readyz check passed
$ curl localhost:8090/metrics        # Prometheus
$ go run . --print-config            # the loaded config as YAML, secrets hidden
```

A real service adds one line per component, in dependency order. Components find what was built before them by type — no globals, no reflection:

```go
app.Add(cfg.DB, postgres.New)   // returns *pgxpool.Pool
app.Add(cfg.Orders, orders.New) // service.Get[*pgxpool.Pool](env)
app.Add(cfg.API, api.New)       // service.Get[orders.Service](env)
```

That's it. You now have JSON logs, `/livez` `/readyz` `/healthz`, Prometheus metrics, pprof, OpenTelemetry (off until you set `OTEL_ENABLED=true`), `--config`, `--help` and `--print-config`, and a Kubernetes-friendly shutdown.

Want to wire everything yourself? Every piece is a regular package — see [Manual wiring](docs/getting-started.md#manual-wiring).

## What you get

| | Feature | Details |
|---|---|---|
| 🩺 | **Health probes that can't hang** | `/livez`, `/readyz`, `/healthz` answer from an in-memory snapshot, even when a dependency is stuck. Kubernetes API server semantics: `?verbose`, `?exclude=`, per-check paths. [→ Health](docs/health.md) |
| 🚦 | **Zero-downtime deploys** | On SIGTERM: readiness goes 503 → wait `drain_delay` → stop servers → stop ops last. [→ Lifecycle](docs/lifecycle.md#shutdown-and-drain) |
| 🔌 | **Poll, push and heartbeat checks** | `Ping`-style polling, callback-driven statuses (Kafka, NATS), heartbeats for worker loops, anti-flapping thresholds. |
| ⏱️ | **Periodic tasks** | `service.NewTicker` runs a job now and every interval, never overlapping, with timeout and jitter. [→ Lifecycle](docs/lifecycle.md#periodic-tasks) |
| 📡 | **gRPC health for free** | Standard `grpc.health.v1`, driven by the same monitor. [→ Health](docs/health.md#grpc-health) |
| 🔭 | **OpenTelemetry by the book** | Standard `OTEL_*` env vars, traces + metrics + logs over OTLP, `trace_id`/`span_id` in log lines. [→ Observability](docs/observability.md) |
| 📊 | **Ops server** | `/metrics` with full Go runtime metrics, `pprof`, `expvar` and opt-in `/version` (`OPS_VERSION_ENABLED=true`) on a separate private port. [→ Ops](docs/ops.md) |
| 🔐 | **Safe logging** | `log/slog` under the hood: JSON by default, a colored `console` format for local runs, `journal` for systemd, your own via `logger.RegisterFormat`, secret masking by key, request-scoped attributes. [→ Observability](docs/observability.md#logging) |
| ⚙️ | **One config, many sources** | Defaults → YAML/JSON/TOML → env → flags. `--help` lists every flag and variable, `--print-config` prints the loaded config with secrets hidden. [→ Configuration](docs/configuration.md) |
| 🧩 | **Assembly without a framework** | `app.Add(cfg.Section, pkg.New)` per component, dependencies by type with `service.Get`, a clear error with `file:line` when something is missing. [→ Getting started](docs/getting-started.md) |
| 🧪 | **Testable** | Test logger, `service.TestEnv(t, fakes...)` to build one component with fake dependencies, `config.Defaults[T]()` for configs. [→ Testing](docs/testing.md) |

## Packages

Use the `app` facade, or pick only what you need:

| Package | What it does |
|---|---|
| [`app`](https://pkg.go.dev/github.com/im-kulikov/go-bones/app) | Optional facade: `Init`, `Add`, `Run` — wires everything below with production defaults |
| [`config`](https://pkg.go.dev/github.com/im-kulikov/go-bones/config) | Config loading on top of [gonfig](https://github.com/im-kulikov/gonfig), `Defaults`, `HTTP` / `GRPC` sections |
| [`logger`](https://pkg.go.dev/github.com/im-kulikov/go-bones/logger) | `log/slog` with secret masking, trace correlation and the OTel log bridge |
| [`service`](https://pkg.go.dev/github.com/im-kulikov/go-bones/service) | Lifecycle runner: start, signals, phased shutdown, tickers; component contract `Env`, `Constructor`, `Get` |
| [`health`](https://pkg.go.dev/github.com/im-kulikov/go-bones/health) | Non-blocking health monitor |
| [`network/http`](https://pkg.go.dev/github.com/im-kulikov/go-bones/network/http) | HTTP server, ops server, health endpoints |
| [`network/grpc`](https://pkg.go.dev/github.com/im-kulikov/go-bones/network/grpc) | gRPC server, health service |
| [`tracer`](https://pkg.go.dev/github.com/im-kulikov/go-bones/tracer) | OpenTelemetry bootstrap |

## Documentation

| Page | Read it when |
|---|---|
| [Getting started](docs/getting-started.md) | First service, components, and wiring without the `app` facade |
| [Configuration](docs/configuration.md) | Config structs, load order, file/env/flags, full env reference |
| [Lifecycle](docs/lifecycle.md) | Services, workers, periodic tasks, graceful shutdown and drain |
| [HTTP & gRPC](docs/transports.md) | Servers, TLS, instrumenting handlers and clients |
| [Health checks](docs/health.md) | Readiness vs liveness, writing checks, endpoints, metrics |
| [Ops server](docs/ops.md) | `/metrics`, pprof, expvar, `/version`, securing the port |
| [Observability](docs/observability.md) | Logging, OpenTelemetry, correlating logs and traces |
| [Kubernetes](docs/kubernetes.md) | Probes, grace periods, a complete Deployment |
| [Testing](docs/testing.md) | Test logger, testing components, health checks in tests |
| [FAQ & troubleshooting](docs/faq.md) | Comparisons, common mistakes, "why is my pod not ready?" |

## Is it for me?

**Good fit** — HTTP/gRPC microservices and workers running in containers, teams that want the same operational behavior in every service without adopting a framework.

**Not a fit** — you need a web framework (routing DSL, ORM, validation), a dependency injection container, or code generation from proto. go-bones stays out of your business code; pair it with any router or gRPC stack you like. See [how it compares](docs/faq.md#how-does-it-compare-to-other-libraries).

## Status

go-bones is **pre-1.0**: the API may change between minor versions; breaking changes are listed in the [release notes](https://github.com/im-kulikov/go-bones/releases).

## Contributing

Issues and PRs are welcome. Local checks:

| Command | Description |
|---|---|
| `make help` | Show available targets |
| `make deps` | Ensure dependencies are available |
| `make lint` | Run `golangci-lint` |
| `make vet` | Run `go vet ./...` |
| `make test` | Run tests |
| `make install-tools` | Install development tools |

## License

[MIT](LICENSE)
