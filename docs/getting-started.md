# Getting started

In 5 minutes you will have a service with an HTTP API, health probes, metrics and graceful shutdown.

**You need:** Go 1.26+, `curl`. Docker is optional (for the observability demo).

- [1. Create the project](#1-create-the-project)
- [2. Write main.go](#2-write-maingo)
- [3. Run and poke it](#3-run-and-poke-it)
- [4. Split it into components](#4-split-it-into-components)
- [5. Add a dependency with a health check](#5-add-a-dependency-with-a-health-check)
- [6. Add a background worker](#6-add-a-background-worker)
- [7. Test a component](#7-test-a-component)
- [8. Configure it](#8-configure-it)
- [Manual wiring](#manual-wiring) — the same service without the facade
- [Next steps](#next-steps)

## 1. Create the project

```bash
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/im-kulikov/go-bones@latest
```

## 2. Write main.go

```go
package main

import (
	"github.com/im-kulikov/go-bones/app"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/network/http"
	"github.com/im-kulikov/go-bones/service"
)

type settings struct {
	config.Base
	API config.HTTP `env:"API" yaml:"api"`
}

func newAPI(cfg config.HTTP, env service.Env) (service.Service, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello/{name}", func(w http.ResponseWriter, r *http.Request) {
		env.Logger.InfoContext(r.Context(), "greeting", "name", r.PathValue("name"))
		_, _ = w.Write([]byte("hello, " + r.PathValue("name")))
	})

	return http.NewServer(cfg, env.Logger, http.ServiceName("api"), http.WithHandler(mux))
}

func main() {
	cfg := app.Init[settings](config.WithVersion("dev"))
	app.Add(cfg.API, newAPI)
	app.Run()
}
```

What `app.Init` did for you:

1. Loaded `settings` from defaults, env and flags (and a file, see [Configuration](configuration.md#config-files)). `settings` must embed `config.Base` — otherwise it does not compile.
2. Initialized the logger (`logger.Init`) and OpenTelemetry (`tracer.Init`, disabled by default).
3. Created the health monitor and the ops server on `:8090` with `/livez`, `/readyz`, `/healthz`, `/metrics`, `/debug/pprof`.

What `app.Add(cfg.API, newAPI)` does: calls `newAPI(cfg.API, env)`, remembers the result for later constructors, and — because it is a `service.Service` — starts it in `Run`. If the constructor fails, the process exits with code `1` and a message like `main.go:31: could not build component … main.newAPI: …`.

What `app.Run()` does:

1. Starts every service, the health monitor and the ops server.
2. Waits for SIGINT/SIGTERM or for any service to fail.
3. Drains: `/readyz` → 503, waits `health.drain_delay`, stops the API, stops the ops server last.
4. Returns on a clean shutdown, exits with code `1` on failure.

## 3. Run and poke it

```bash
LOGGER_FORMAT=console go run .   # colored logs for local development; json is the default
```

```bash
curl localhost:8080/hello/gopher      # hello, gopher
curl 'localhost:8090/readyz?verbose'  # readyz check passed
curl localhost:8090/healthz           # JSON report
curl -s localhost:8090/metrics | grep go_bones_health
```

Press `Ctrl+C` and watch the phased shutdown in the log.

## 4. Split it into components

A component is a package with a config type and a constructor of one fixed shape:

```go
// service.Constructor[C, T]
func New(cfg Config, env service.Env) (T, error)
```

`env` carries what every component needs — `env.Context` (canceled on SIGINT/SIGTERM, for work done while building), `env.Logger`, `env.Health` — and gives access to the components built before it:

```go
// internal/api
func New(cfg config.HTTP, env service.Env) (service.Service, error) {
	orders := service.Get[orders.Service](env) // dependencies first

	return http.NewServer(cfg, env.Logger, http.ServiceName("api"), http.WithHandler(routes(orders)))
}
```

`main` lists the components in dependency order:

```go
app.Add(cfg.DB, postgres.New)
app.Add(cfg.Orders, orders.New)
app.Add(cfg.API, api.New)
```

`service.Get[T]` returns the only component built so far that is assignable to `T`, usually an interface. Nothing assignable, or more than one, stops the start with `needs orders.Service, nothing built before provides it` or `…, ambiguous: *a.Repo, *b.Repo` — before any server opens a port.

Rules of thumb:

- the config parameter is a concrete type, not an interface (Go cannot infer type parameters otherwise);
- call `Get` at the top of the constructor, so a component's dependencies are visible at a glance;
- prefer an explicit dependency in the signature? `app.Add` returns what it built, so `ord := app.Add(cfg.Orders, orders.New)` followed by `app.Add(cfg.API, api.New(ord))` works too, with `api.New` returning a `service.Constructor`.

## 5. Add a dependency with a health check

A component that is not a service is just returned and made available to the next ones. It can register its own readiness check:

```go
// internal/postgres
func New(cfg Config, env service.Env) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(env.Context, cfg.DSN)
	if err != nil {
		return nil, err
	}

	return pool, env.Health.Register("postgres", health.CheckerFunc(pool.Ping))
}
```

Stop the database and `/readyz` turns 503 within `health.interval`; start it again and the instance comes back. Read [Health checks](health.md) before you add many — especially [what must never be a liveness check](health.md#impact).

## 6. Add a background worker

A worker is a component that returns a launcher:

```go
// internal/outbox
func New(cfg Config, env service.Env) (service.Service, error) {
	db := service.Get[*pgxpool.Pool](env)
	hb, err := env.Health.Heartbeat("outbox", 3*cfg.Every)
	if err != nil {
		return nil, err
	}

	return service.NewTicker("outbox", cfg.Every, func(ctx context.Context) error {
		hb.Beat()
		return flush(ctx, db)
	}), nil
}
```

`app.Add(cfg.Outbox, outbox.New)` and it runs every `cfg.Every`, beats the heartbeat and stops with the rest. A worker built with `service.NewLauncher` that returns a non-nil error stops the whole application — that is intentional: let the orchestrator restart a broken process. See [Lifecycle](lifecycle.md) for periodic tasks and heartbeats.

## 7. Test a component

Components never see `app`, so a unit test builds one with fakes:

```go
func TestAPI(t *testing.T) {
	svc, err := api.New(config.Defaults[config.HTTP](), service.TestEnv(t, fakeOrders{}))
	require.NoError(t, err)
	// ...
}
```

`service.TestEnv` gives the test context, a test logger, a fresh health monitor, and the fakes as the dependencies `service.Get` can find. More in [Testing](testing.md).

## 8. Configure it

Add your own settings next to the built-in ones:

```go
type settings struct {
	config.Base

	API    config.HTTP     `env:"API"    yaml:"api"`
	DB     postgres.Config `env:"DB"     yaml:"db"`
	Outbox outbox.Config   `env:"OUTBOX" yaml:"outbox"`
}

// internal/postgres
type Config struct {
	DSN string `env:"DSN" yaml:"dsn"`
}
```

Then set values any way you like:

```bash
DB_DSN=postgres://... go run .
go run . --config ./config.yaml   # --config comes with config.Base
go run . --print-config           # the loaded config, as YAML
```

Full reference: [Configuration](configuration.md).

## Manual wiring

`app` is a thin facade (one small file). The same service assembled from the underlying packages — use this form when you need full control or want to adopt go-bones gradually. Component constructors don't change: call them with `service.Build` and an `Env` from `service.NewEnv`, or directly.

```go
func main() {
	var cfg settings
	if err := config.Load(&cfg, config.WithName("hello"), config.WithVersion("dev")); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	log := logger.Init(cfg.Logger)
	telemetry := tracer.Init(log, cfg.Tracer)
	hc := health.New(cfg.Health, log)

	api, err := http.NewServer(cfg.API, log,
		http.ServiceName("api"),
		http.WithOpenTelemetry(),
		http.WithHandler(mux),
	)
	if err != nil {
		log.Error("build api", logger.Err(err))
		os.Exit(1)
	}

	ops, err := http.NewOPSServer(cfg.OpsServer, log, http.WithHealth(hc))
	if err != nil {
		log.Error("build ops", logger.Err(err))
		os.Exit(1)
	}

	err = service.Run(log,
		service.WithHealth(hc),
		service.WithDrainDelay(cfg.Health.DrainDelay),
		service.WithShutdownLast(ops),
		service.WithService(telemetry, api, ops),
	)
	if err != nil { // a signal is a clean shutdown: nil
		log.Error("stopped with error", logger.Err(err))
		os.Exit(1)
	}
}
```

Rules the facade follows, and you should too:

- pass the **same** `hc` to the ops server, gRPC server and `service.Run`;
- put the ops server into `WithShutdownLast` so probes answer 503 until the very end;
- start `telemetry` as a service — it flushes exporters on shutdown;
- exit with a non-zero code on failure — `service.Run` returns the error, it does not exit for you.

## Next steps

- [Kubernetes](kubernetes.md) — probes and a Deployment you can copy.
- [Observability](observability.md) — turn on tracing and see a request end to end.
- [Testing](testing.md) — build a component with fakes, assemble a stack in a test.
