![go-bones](.github/logo@2.png)

![Codecov](https://img.shields.io/codecov/c/github/im-kulikov/go-bones.svg?style=flat-square)
[![GitHub Workflow Status](https://github.com/im-kulikov/go-bones/actions/workflows/go.yml/badge.svg)](https://github.com/im-kulikov/go-bones/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/im-kulikov/go-bones)](https://goreportcard.com/report/github.com/im-kulikov/go-bones)
![Go version](https://img.shields.io/github/go-mod/go-version/im-kulikov/go-bones?style=flat&label=Go%20%3E%3D)
[![PkgGoDev](https://pkg.go.dev/badge/mod/github.com/im-kulikov/go-bones)](https://pkg.go.dev/mod/github.com/im-kulikov/go-bones)
[![GitHub release](https://img.shields.io/github/release/im-kulikov/go-bones.svg)](https://github.com/im-kulikov/go-bones)
![GitHub](https://img.shields.io/github/license/im-kulikov/go-bones.svg?style=popout)
[![Dependabot Status](https://img.shields.io/badge/dependabot-active-brightgreen?logo=dependabot)](https://dependabot.com)

`go-bones` is a small application foundation for Go services. It provides:

- configuration loading via `config`
- structured logging via `logger`
- lifecycle orchestration via `service`
- HTTP, gRPC, and OPS transports via `network/http` and `network/grpc`
- OpenTelemetry bootstrap via `tracer`

The library is intentionally opinionated:

- use one config struct with embedded `config.Base`
- initialize logging once at startup
- run long-lived components through `service.Run`
- expose operational endpoints through the OPS server
- prefer standard `OTEL_*` environment variables for telemetry configuration

## Contents

- [Installation](#installation)
- [Quick Start](#quick-start)
- [Configuration Model](#configuration-model)
- [Logger](#logger)
- [Lifecycle Runner](#lifecycle-runner)
- [HTTP Service](#http-service)
- [gRPC Service](#grpc-service)
- [OPS Service](#ops-service)
- [OpenTelemetry](#opentelemetry)
- [Make Targets](#make-targets)

## Installation

```bash
go get github.com/im-kulikov/go-bones
```

## Quick Start

This is the intended shape of an application built on top of `go-bones`.

```go
package main

import (
	"context"
	"errors"
	"os"
	"time"

	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/network/grpc"
	"github.com/im-kulikov/go-bones/network/http"
	"github.com/im-kulikov/go-bones/service"
	"github.com/im-kulikov/go-bones/tracer"
)

type appConfig struct {
	config.Base `env:",squash" yaml:",inline" json:",inline" toml:",inline"`

	HTTP httpConfig `env:"HTTP" yaml:"http" json:"http" toml:"http"`
	GRPC grpcConfig `env:"GRPC" yaml:"grpc" json:"grpc" toml:"grpc"`
	App  runtimeConfig `env:"APP" yaml:"app" json:"app" toml:"app"`
}

type runtimeConfig struct {
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" yaml:"shutdown_timeout" default:"20s"`
}

type httpConfig struct {
	config.BaseHTTP `env:",squash" yaml:",inline" json:",inline" toml:",inline"`
	Address         string `env:"ADDRESS" yaml:"address" json:"address" toml:"address" default:":8080"`
}

func (c httpConfig) Addr() string          { return c.Address }
func (c httpConfig) Base() config.BaseHTTP { return c.BaseHTTP }

type grpcConfig struct {
	config.BaseGRPC `env:",squash" yaml:",inline" json:",inline" toml:",inline"`
	Address         string `env:"ADDRESS" yaml:"address" json:"address" toml:"address" default:":9090"`
}

func (c grpcConfig) Addr() string          { return c.Address }
func (c grpcConfig) Base() config.BaseGRPC { return c.BaseGRPC }

func main() {
	var cfg appConfig
	if err := config.Load(&cfg,
		config.WithName("my-service"),
		config.WithVersion("dev"),
	); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	log := logger.Init(cfg.Logger)
	telemetry := tracer.Init(log, cfg.Tracer)

	httpSvc, err := newHTTPService(cfg, log)
	if err != nil {
		log.Error("build http service", logger.Err(err))
		os.Exit(1)
	}

	grpcSvc, err := newGRPCService(cfg, log)
	if err != nil {
		log.Error("build grpc service", logger.Err(err))
		os.Exit(1)
	}

	opsSvc, err := http.NewOPSServer(cfg.OpsServer, log)
	if err != nil {
		log.Error("build ops service", logger.Err(err))
		os.Exit(1)
	}

	group := service.Compose(telemetry, opsSvc, httpSvc, grpcSvc)
	if err = service.Run(log,
		service.WithShutdownTimeout(cfg.App.ShutdownTimeout),
		service.WithService(group),
	); err != nil && !errors.Is(err, context.Canceled) {
		log.Error("application stopped with error", logger.Err(err))
		os.Exit(1)
	}
}

func newHTTPService(cfg appConfig, log *logger.Logger) (service.Service, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	return http.NewServer(
		cfg.HTTP,
		log,
		http.ServiceName("api"),
		http.WithOpenTelemetry(),
		http.ServerOptions(func(srv *http.Server) {
			srv.Handler = mux
		}),
	)
}

func newGRPCService(cfg appConfig, log *logger.Logger) (service.Service, error) {
	return grpc.NewServer(
		cfg.GRPC,
		log,
		grpc.ServiceName("rpc"),
		grpc.WithOpenTelemetry(),
		grpc.RegisterServices(func(server *grpc.Server) {
			healthServer := health.NewServer()
			healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
			healthpb.RegisterHealthServer(server, healthServer)
		}),
	)
}
```

## Configuration Model

`config.Load` is the entrypoint for application configuration.

Under the hood, `go-bones/config` uses [`gonfig`](https://github.com/im-kulikov/gonfig) as the loader engine.  
`go-bones` adds:

- the shared `config.Base` model
- app name/version propagation into supported config sections
- repository-friendly defaults such as YAML are enabled by default
- a stable place for built-in logger, ops, and tracer config

```go
var cfg appConfig
err := config.Load(&cfg,
	config.WithName("my-service"),
	config.WithVersion("1.2.3"),
)
```

Key points:

- `config.Base` already includes `Logger`, `OpsServer`, and `Tracer`
- YAML loading is enabled by default
- app name and version are propagated into config sections that support them
- `gonfig` still does the heavy lifting for env/YAML/JSON/TOML parsing
- transport packages expect config types implementing:
  - `config.HTTPConfig`
  - `config.GRPCConfig`

Typical custom config shape:

```go
type appConfig struct {
	config.Base `env:",squash" yaml:",inline" json:",inline" toml:",inline"`

	HTTP struct {
		config.BaseHTTP `env:",squash" yaml:",inline" json:",inline" toml:",inline"`
		Address string `env:"ADDRESS" yaml:"address" default:":8080"`
	} `env:"HTTP" yaml:"http" json:"http" toml:"http"`

	App struct {
		WorkerInterval time.Duration `env:"WORKER_INTERVAL" yaml:"worker_interval" default:"15s"`
	} `env:"APP" yaml:"app" json:"app" toml:"app"`
}
```

Representative YAML:

```yaml
logger:
  level: info
  format: text
  add_source: true
  add_app_info: true
  open_tracing: true
  secrets:
    - authorization
    - api_key

ops:
  address: ":8090"
  version_enabled: true

tracer:
  enabled: true
  send_logs: true
  send_metrics: true
  use_http: true
  endpoint: "otel-collector:4318"
  insecure: true

http:
  address: ":8080"
  shutdown_timeout: 10s

grpc:
  address: ":9090"
  shutdown_timeout: 10s
```

### Loading configuration from a file

`config.Load` can load configuration from a file through `gonfig` file loaders.

To enable this, you need two things:

1. Enable the parser you want:
   - `config.WithYAML()`
   - `config.WithJSON()`
   - `config.WithTOML()`
2. Add a root config field marked as a config-path flag:

```go
type appConfig struct {
	config.Base `env:",squash" yaml:",inline"`

	Config string `flag:"config,config:true"`

	HTTP httpConfig `env:"HTTP" yaml:"http"`
}
```

If you prefer the default config-path flags, you can embed `config.DefaultConfigFlag`
instead of declaring the `Config` field manually. It enables the standard `--config`
and `-c` flags for your config struct.

```go
type appConfig struct {
	config.Base `env:",squash" yaml:",inline"`
	config.DefaultConfigFlag `env:",squash" yaml:",inline" json:",inline" toml:",inline"`

	HTTP httpConfig `env:"HTTP" yaml:"http"`
}
```

Then you can start the service with:

```bash
./my-service --config ./config.yaml
```

Example:

```go
var cfg appConfig
err := config.Load(&cfg,
	config.WithName("my-service"),
	config.WithVersion("dev"),
	config.WithYAML(),
)
```

Notes:

- if you do not specify a parser option, `config.Load` enables YAML by default
- the `--config` flag is provided by `gonfig` through the `flag:"config,config:true"` tag
- file loading is optional; the same config can still be fully driven by environment variables and flags

### Configuration load order

The effective load order follows `gonfig` semantics:

1. defaults from struct tags
2. config-path pre-scan from flags, for example `--config ./config.yaml`
3. file loader (`YAML`, `JSON`, or `TOML`)
4. environment variables
5. command-line flags

In practice this means:

- file values override defaults
- env variables override file values
- flags have the highest priority

This is usually the desired production behavior:

- keep a baseline in a file
- override sensitive or environment-specific values through env
- use flags only for explicit local overrides or bootstrapping

### Environment variables by module

Built-in config blocks from `config.Base` are exposed through these prefixes:

- `LOGGER_*`
- `OPS_*`
- `OTEL_*`

Your own application blocks keep the same pattern. For example, if your config has:

```go
type appConfig struct {
	config.Base `env:",squash" yaml:",inline"`

	HTTP httpConfig `env:"HTTP" yaml:"http"`
	GRPC grpcConfig `env:"GRPC" yaml:"grpc"`
	App  runtimeConfig `env:"APP" yaml:"app"`
}
```

then the derived env names look like:

- `HTTP_ADDRESS`
- `HTTP_SHUTDOWN_TIMEOUT`
- `HTTP_TLS_ENABLED`
- `GRPC_ADDRESS`
- `GRPC_SHUTDOWN_TIMEOUT`
- `GRPC_TLS_ENABLED`
- `APP_SHUTDOWN_TIMEOUT`

#### `LOGGER_*`

| Env                           | Default | Meaning                                                                              |
|-------------------------------|---------|--------------------------------------------------------------------------------------|
| `LOGGER_OPEN_TRACING_ENABLED` | `false` | Enables bridging logger records into the process-wide OpenTelemetry logger provider. |
| `LOGGER_SECRETS`              | empty   | Comma-separated field names to redact in logs.                                       |
| `LOGGER_LEVEL`                | `info`  | Log level for the default logger.                                                    |
| `LOGGER_FORMAT`               | `text`  | Output format: `text` or `json`.                                                     |
| `LOGGER_ADD_SOURCE`           | `false` | Includes source file and line information.                                           |
| `LOGGER_ADD_APP_INFO`         | `false` | Includes app metadata such as name and version in log output.                        |

#### `OPS_*`

| Env                       | Default          | Meaning                                      |
|---------------------------|------------------|----------------------------------------------|
| `OPS_ADDRESS`             | `:8090`          | Listen address for the OPS server.           |
| `OPS_METRICS_PATH`        | `/metrics`       | Prometheus metrics endpoint path.            |
| `OPS_PROFILE_PATH`        | `/debug/pprof`   | Base path for `pprof` handlers.              |
| `OPS_EXP_VARS_PATH`       | `/debug/vars`    | `expvar` endpoint path.                      |
| `OPS_VERSION_PATH`        | `/version`       | Version endpoint path.                       |
| `OPS_VERSION_ENABLED`     | `false`          | Enables the version endpoint.                |
| `OPS_READ_TIMEOUT`        | `0`              | HTTP read timeout for the OPS server.        |
| `OPS_WRITE_TIMEOUT`       | `0`              | HTTP write timeout for the OPS server.       |
| `OPS_READ_HEADER_TIMEOUT` | `0`              | HTTP read-header timeout for the OPS server. |
| `OPS_IDLE_TIMEOUT`        | `0`              | HTTP idle timeout for the OPS server.        |
| `OPS_SHUTDOWN_TIMEOUT`    | `30s`            | Graceful shutdown timeout.                   |
| `OPS_MAX_HEADER_BYTES`    | `0`              | Max request header size.                     |
| `OPS_TLS_ENABLED`         | `false`          | Enables TLS for the OPS server.              |
| `OPS_TLS_CERT_FILE`       | empty            | TLS certificate path.                        |
| `OPS_TLS_KEY_FILE`        | empty            | TLS private key path.                        |
| `OPS_TLS_CLIENT_AUTH`     | `no-client-cert` | TLS client auth mode.                        |
| `OPS_TLS_CA_CERT_FILE`    | empty            | CA certificate path for client verification. |
| `OPS_TLS_MIN_VERSION`     | `TLS13`          | Minimum TLS version.                         |
| `OPS_TLS_CIPHER_SUITES`   | empty            | Optional cipher suite list.                  |

#### `OTEL_*` from `config.TracerConfig`

These are repository-level fallback variables. Standard OpenTelemetry exporter variables still have priority.

| Env                 | Default          | Meaning                                                            |
|---------------------|------------------|--------------------------------------------------------------------|
| `OTEL_ENABLED`      | `false`          | Enables repository OpenTelemetry bootstrap.                        |
| `OTEL_SEND_LOGS`    | `false`          | Enables OTel log export path.                                      |
| `OTEL_SEND_METRICS` | `false`          | Enables OTel metrics export path.                                  |
| `OTEL_ENDPOINT`     | `localhost:4317` | Fallback OTLP endpoint when exporter-specific env is absent.       |
| `OTEL_INSECURE`     | `true`           | Fallback insecure OTLP transport setting.                          |
| `OTEL_USE_HTTP`     | `false`          | Fallback switch to OTLP HTTP when exporter protocol env is absent. |

#### Standard `OTEL_*` variables

These are the preferred runtime contracts for real deployments:

| Env                                   | Meaning                                                                                          |
|---------------------------------------|--------------------------------------------------------------------------------------------------|
| `OTEL_SERVICE_NAME`                   | Explicit service name in OTel resource attributes.                                               |
| `OTEL_RESOURCE_ATTRIBUTES`            | Additional resource attributes, for example `deployment.environment=prod,service.version=1.2.3`. |
| `OTEL_PROPAGATORS`                    | Propagator list, for example `tracecontext,baggage`.                                             |
| `OTEL_EXPORTER_OTLP_ENDPOINT`         | Common OTLP endpoint for all enabled signals.                                                    |
| `OTEL_EXPORTER_OTLP_INSECURE`         | Common insecure flag for OTLP exporters.                                                         |
| `OTEL_EXPORTER_OTLP_PROTOCOL`         | Common OTLP protocol, for example `grpc` or `http/protobuf`.                                     |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`  | Trace-specific OTLP endpoint.                                                                    |
| `OTEL_EXPORTER_OTLP_TRACES_INSECURE`  | Trace-specific insecure flag.                                                                    |
| `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL`  | Trace-specific protocol.                                                                         |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | Metrics-specific OTLP endpoint.                                                                  |
| `OTEL_EXPORTER_OTLP_METRICS_INSECURE` | Metrics-specific insecure flag.                                                                  |
| `OTEL_EXPORTER_OTLP_METRICS_PROTOCOL` | Metrics-specific protocol.                                                                       |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`    | Logs-specific OTLP endpoint.                                                                     |
| `OTEL_EXPORTER_OTLP_LOGS_INSECURE`    | Logs-specific insecure flag.                                                                     |
| `OTEL_EXPORTER_OTLP_LOGS_PROTOCOL`    | Logs-specific protocol.                                                                          |

## Logger

The logger package wraps `log/slog` and exposes both explicit logger instances and a process-wide default logger.

Typical startup:

```go
log := logger.Init(cfg.Logger)
```

What `logger.Init` gives you:

- log level and format from `config.Logger`
- optional source locations
- optional app metadata fields
- secret masking
- optional OpenTelemetry log bridge when `logger.open_tracing` is enabled and `tracer.Init(...)` installs a logger provider

Package-level helpers are available:

```go
logger.Info("service started")
logger.Error("request failed", logger.Err(err))
```

For request-scoped fields, attach attributes to context:

```go
ctx = logger.AddContextAttrs(ctx,
	logger.String("request_id", requestID),
	logger.String("tenant", tenantID),
)

log.InfoContext(ctx, "handled request")
```

## Lifecycle Runner

`service.Run` is the main orchestration primitive for long-lived components.

Use it for:

- HTTP servers
- gRPC servers
- telemetry bootstrap
- background workers

Example:

```go
worker := service.NewLauncher("worker", func(ctx context.Context) error {
	<-ctx.Done()
	return context.Cause(ctx)
})

err := service.Run(log,
	service.WithShutdownTimeout(20*time.Second),
	service.WithService(worker, telemetry, opsSvc, httpSvc, grpcSvc),
)
```

Useful pieces:

- `service.NewLauncher(...)` for wrapping a start function into a managed service
- `service.Compose(...)` for grouping services without making the group itself independently runnable
- `service.WithIgnoreError(...)` for expected shutdown errors

## HTTP Service

`network/http` owns the lifecycle of a standard `http.Server` and re-exports the most common stdlib HTTP types so application code usually does not need a second `net/http` import alias.

Typical wiring:

```go
handler := http.NewServeMux()
handler.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("ok"))
})

svc, err := http.NewServer(
	cfg.HTTP,
	log,
	http.ServiceName("api"),
	http.WithOpenTelemetry(),
	http.ServerOptions(func(srv *http.Server) {
		srv.Handler = handler
	}),
)
```

Important behavior:

- the package opens the listener once and serves on it directly
- TLS is derived from `config.BaseHTTP.TLSConfig`
- graceful shutdown uses `BaseHTTP.ShutdownTimeout` with a safe fallback
- `WithOpenTelemetry()` extracts incoming trace context and creates server spans

## gRPC Service

`network/grpc` owns the lifecycle of a `grpc.Server` and re-exports the most common gRPC server-side types used for registration and descriptors.

Typical wiring:

```go
svc, err := grpc.NewServer(
	cfg.GRPC,
	log,
	grpc.ServiceName("rpc"),
	grpc.WithOpenTelemetry(),
	grpc.RegisterServices(func(server *grpc.Server) {
		healthServer := health.NewServer()
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
		healthpb.RegisterHealthServer(server, healthServer)
	}),
)
```

Important behavior:

- TLS is derived from `config.BaseGRPC.TLSConfig`
- graceful shutdown uses `BaseGRPC.ShutdownTimeout` with a safe fallback
- repeated shutdown calls are serialized internally
- `WithOpenTelemetry()` installs server-side unary and stream interceptors for trace extraction and span continuation

## OPS Service

The OPS server is a ready-to-run HTTP service for diagnostics and runtime observability.

```go
opsSvc, err := http.NewOPSServer(cfg.OpsServer, log)
```

By default, it exposes:

- `/metrics`
- `/debug/pprof`
- `/debug/vars`
- `/version` when `version_enabled=true`

It also exposes named pprof profiles under the same base path, for example:

- `/debug/pprof/goroutine`
- `/debug/pprof/heap`
- `/debug/pprof/block`

When the process is built with `GOEXPERIMENT=goroutineleakprofile`, Go 1.26 also exposes:

- `/debug/pprof/goroutineleak`

Runtime metrics are exported through the standard Prometheus Go collector with `collectors.MetricsAll`, so scheduler, goroutine, GC, memory, and other Go 1.26 runtime metric groups are available.

`OPS` metrics and OpenTelemetry metrics are intentionally separate telemetry paths. Applications may use either one or both.

## OpenTelemetry

`tracer.Init(log, cfg.Tracer)` installs process-wide OpenTelemetry state and returns a lifecycle-managed service.

```go
telemetry := tracer.Init(log, cfg.Tracer)
```

What it configures:

- resource attributes
- text-map propagator
- `TracerProvider`
- optional `MeterProvider`
- optional OTel `LoggerProvider`

Configuration policy:

- standard `OTEL_*` environment variables are the primary contract
- `config.TracerConfig` acts as a repository-local fallback layer
- `Endpoint`, `Insecure`, and `UseHTTP` are only used when the corresponding exporter-specific `OTEL_*` variables are not set

Recommended environment examples:

```bash
export OTEL_ENABLED=true
export OTEL_SEND_LOGS=true
export OTEL_SEND_METRICS=true
export OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4318
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_INSECURE=true
```

Repository-specific fallback config also works:

```yaml
tracer:
  enabled: true
  send_logs: true
  send_metrics: true
  use_http: true
  endpoint: "otel-collector:4318"
  insecure: true
```

To get end-to-end request correlation:

1. Initialize logger with `logger.Init(cfg.Logger)`.
2. Initialize telemetry with `tracer.Init(log, cfg.Tracer)`.
3. Enable transport instrumentation with:
   - `network/http.WithOpenTelemetry()`
   - `network/grpc.WithOpenTelemetry()`
4. Enable log bridging with `logger.open_tracing: true`.

After that:

- HTTP and gRPC transports continue incoming trace context
- logs emitted inside request/RPC contexts include `trace_id` and `span_id`
- traces, logs, and metrics can be exported through OTLP

## Make Targets

| Command              | Description                         |
|----------------------|-------------------------------------|
| `make help`          | Show available targets              |
| `make deps`          | Ensure dependencies are available   |
| `make lint`          | Run `golangci-lint`                 |
| `make vet`           | Run `go vet ./...`                  |
| `make test`          | Run tests                           |
| `make install-tools` | Install development tools           |
