# Configuration

- [The model](#the-model)
- [Load order](#load-order)
- [Config files](#config-files)
- [Help and printing the config](#help-and-printing-the-config)
- [Secrets](#secrets)
- [Config in code and tests](#config-in-code-and-tests)
- [Reference](#reference): [`logger`](#logger) · [`ops`](#ops) · [`health`](#health) · [`tracer`](#tracer) · [`http` / `grpc`](#http--grpc) · [TLS](#tls)

## The model

One struct describes the whole service. Embed `config.Base` and add a section per component:

```go
type settings struct {
	config.Base // logger, ops, tracer, health

	API    config.HTTP   `env:"API"    yaml:"api"`
	Outbox outbox.Config `env:"OUTBOX" yaml:"outbox"`
}

// internal/outbox
type Config struct {
	Every time.Duration `env:"EVERY" yaml:"every" default:"15s"`
}
```

| Type | Contains | Use with |
|---|---|---|
| `config.Base` | `Logger`, `OpsServer`, `Tracer`, `Health` | `app.Init[settings]` (required: it does not compile without it) or manual wiring |
| `config.HTTP`, `config.GRPC` | `Address` (`:8080` / `:9090`) + `config.Network` (timeouts, TLS) | `http.NewServer`, `grpc.NewServer` |
| `config.Network` | timeouts, `shutdown_timeout`, TLS | embed it in your own server section and implement `Addr() string` |

Loading — with the facade, or by hand:

```go
cfg := app.Init[settings](config.WithVersion(version))

var cfg settings
err := config.Load(&cfg, config.WithName("orders"), config.WithVersion(version))
```

Each component gets only its own section: `app.Add(cfg.Outbox, outbox.New)`. Component packages never import `settings`, which lives in `main`.

Name and version are propagated into every section that needs them (log attributes, `/version`, OTel resource). By default they come from the build info: the module path and version, or the VCS revision (`0123456789ab`, `-dirty` with local changes) for a build from a working tree, so `-ldflags` is rarely needed.

Under the hood `config.Load` uses [gonfig](https://github.com/im-kulikov/gonfig) for env, YAML, JSON and TOML parsing.

**Naming rule.** The env name of a field is its path, upper-cased and joined with `_`:

```text
api.address            → API_ADDRESS
api.tls.enabled        → API_TLS_ENABLED
outbox.every           → OUTBOX_EVERY
```

Embedded structs (`config.Base`, `config.Network`) add no prefix.

## Load order

Later sources win:

1. defaults from `default` struct tags;
2. the config file, if a config-path flag is given (`--config ./config.yaml`);
3. environment variables;
4. command-line flags.

The usual production setup: a baseline in a file baked into the image or a ConfigMap, per-environment values and secrets in env, flags only for local experiments.

## Config files

YAML is enabled by default; add `config.WithJSON()` or `config.WithTOML()` for other formats. `config.Base` gives every service the `--config` / `-c` flag with the path of the file:

```bash
./orders --config /etc/orders/config.yaml
```

The path comes only from the flag: in Kubernetes pass it in the container `args` (see [Kubernetes](kubernetes.md)). A field of your own tagged `flag:"config"` clashes with it (`flag redefined: --config`).

A representative file:

```yaml
logger:
  level: info
  format: json
  add_app_info: true
  open_tracing: true
  secrets: [authorization, api_key]

ops:
  address: ":8090"
  profile_enabled: false
  exp_vars_enabled: false
  version_enabled: true

health:
  drain_delay: 5s

tracer:
  enabled: true
  send_logs: true
  send_metrics: true

api:
  address: ":8080"
  shutdown_timeout: 10s
```

## Help and printing the config

`config.Base` also gives two flags that exit right after loading, with code 0:

- `--help` lists every flag and every environment variable with its type and default.
- `--print-config[=yaml|json|toml|env]` prints the loaded config (defaults, file, env and flags applied) with a comment above every value. Without a value the format is the one of the file loader, YAML by default. Required fields are not checked, so on a fresh setup it prints a documented template.

```bash
./orders --print-config > config.yaml   # a template to start from
./orders --print-config=env > .env      # for docker --env-file or a ConfigMap
```

YAML, JSON and TOML output loads back through `--config` as it is, once the loader for that format is enabled (YAML is on by default; `config.WithJSON()`, `config.WithTOML()`). The `env` output is not a config file: pass it as environment variables (`docker --env-file`, a ConfigMap).

## Secrets

- Tag a field with a secret value `secret:"true"` (a database password, a token): `--help` shows no default for it and `--print-config` leaves it empty.
- Log keys listed in `logger.secrets` are replaced with `REDACTED` in every log record, including groups and attributes bound with `log.With` — see [Observability › Logging](observability.md#logging).
- Keep secret values in env (or files mounted by your orchestrator and read by your code), not in the baked-in config file.

## Config in code and tests

`config.Defaults[T]()` returns a section with its `default` tags applied, without reading env or files — what `config.Load` would produce with no other sources:

```go
ops := config.Defaults[config.Ops]()
ops.Address = "127.0.0.1:0"
```

Prefer it to a struct literal: `config.Ops{}` has every `*_enabled` switch `false`, so `http.NewOPSServer` returns a `nil` service.

---

## Reference

### `logger`

| Key | Env | Default | Description |
|---|---|---|---|
| `level` | `LOGGER_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `format` | `LOGGER_FORMAT` | `json` | `json`, `text`, `console` (colored, for local development), or `journal` (stdout read by journald under systemd) |
| `add_source` | `LOGGER_ADD_SOURCE` | `false` | Add file:line to each record |
| `add_app_info` | `LOGGER_ADD_APP_INFO` | `false` | Add the `app` group with name and version |
| `open_tracing` | `LOGGER_OPEN_TRACING_ENABLED` | `false` | Add the `trace` group (`trace_id`, `span_id`) to records logged with a traced context, and mirror them as span events when OTLP log export is off |
| `secrets` | `LOGGER_SECRETS` | — | Comma-separated keys to mask |

OTLP export of log records is switched on by `tracer.send_logs`, not by `open_tracing`.

### `ops`

The ops server runs on its own port. Keep it private — see [Ops server › Security](ops.md#security).

| Key | Env | Default | Description |
|---|---|---|---|
| `enabled` | `OPS_ENABLED` | `true` | Disable the ops server entirely |
| `address` | `OPS_ADDRESS` | `:8090` | Listen address |
| `metrics_enabled` / `metrics_path` | `OPS_METRICS_ENABLED` / `OPS_METRICS_PATH` | `true` / `/metrics` | Prometheus endpoint and Go runtime collector |
| `profile_enabled` / `profile_path` | `OPS_PROFILE_ENABLED` / `OPS_PROFILE_PATH` | `true` / `/debug/pprof` | pprof handlers |
| `exp_vars_enabled` / `exp_vars_path` | `OPS_EXP_VARS_ENABLED` / `OPS_EXP_VARS_PATH` | `true` / `/debug/vars` | expvar |
| `version_enabled` / `version_path` | `OPS_VERSION_ENABLED` / `OPS_VERSION_PATH` | `false` / `/version` | Build info |
| `health_enabled` | `OPS_HEALTH_ENABLED` | `true` | Health endpoints |
| `live_path` / `ready_path` / `health_path` | `OPS_LIVE_PATH` / `OPS_READY_PATH` / `OPS_HEALTH_PATH` | `/livez` / `/readyz` / `/healthz` | Probe paths |

Plus all [network settings](#http--grpc) with the `OPS_` prefix.

The `*_enabled` defaults come from struct tags and are applied by `config.Load` and `config.Defaults`. A `config.Ops{}` built as a literal has every switch `false`, and `http.NewOPSServer` then returns a nil service.

### `health`

Defaults for every check; override per check with `health.WithInterval`, `WithTimeout`, `WithThresholds`.

| Key | Env | Default | Description |
|---|---|---|---|
| `interval` | `HEALTH_INTERVAL` | `10s` | Poll period once a check has passed |
| `initial_interval` | `HEALTH_INITIAL_INTERVAL` | `1s` | Poll period until the first success (services start in parallel, first checks often fail) |
| `start_period` | `HEALTH_START_PERIOD` | `0` (off) | After start, a check that has not passed yet stays `unknown` on failure: not ready, but not logged as failing. Like Docker's `start_period`; useful under systemd, where nothing else waits for a service to start |
| `timeout` | `HEALTH_TIMEOUT` | `2s` | Deadline of one `Check` call (a cold TLS handshake often takes more than 1s) |
| `min_interval` | `HEALTH_MIN_INTERVAL` | `1s` | Rate limit for `Trigger` |
| `stale_after` | `HEALTH_STALE_AFTER` | `0` (= `2×interval + timeout` per check) | Result older than this counts as failing. Never shorter than a check's interval + 10% jitter + timeout, so a small global value cannot make slow checks flap |
| `failure_threshold` | `HEALTH_FAILURE_THRESHOLD` | `1` | Consecutive failures to go failing |
| `success_threshold` | `HEALTH_SUCCESS_THRESHOLD` | `1` | Consecutive successes to go passing |
| `drain_delay` | `HEALTH_DRAIN_DELAY` | `0` | Pause between readiness off and stop on SIGTERM; use `5s` in Kubernetes |
| `log_repeat_interval` | `HEALTH_LOG_REPEAT_INTERVAL` | `5m` | Reminder period for a still-failing check |

### `tracer`

Standard OpenTelemetry variables **always win**; the keys below are a fallback for when they are absent.

| Key | Env | Default | Description |
|---|---|---|---|
| `enabled` | `OTEL_ENABLED` | `false` | Turn on the OTel bootstrap |
| `send_logs` | `OTEL_SEND_LOGS` | `false` | Export log records over OTLP |
| `send_metrics` | `OTEL_SEND_METRICS` | `false` | Export metrics over OTLP |
| `endpoint` | `OTEL_ENDPOINT` | `localhost:4317` | Fallback OTLP endpoint |
| `insecure` | `OTEL_INSECURE` | `true` | Fallback: plaintext OTLP. With a non-local endpoint a warning is logged per exported signal (host only, credentials redacted) |
| `use_http` | `OTEL_USE_HTTP` | `false` | Fallback: OTLP/HTTP instead of gRPC |

Standard variables honored by the bootstrap:

| Env | Meaning |
|---|---|
| `OTEL_SERVICE_NAME` | Service name in the resource |
| `OTEL_RESOURCE_ATTRIBUTES` | Extra resource attributes, e.g. `deployment.environment=prod` |
| `OTEL_PROPAGATORS` | Propagators, e.g. `tracecontext,baggage` |
| `OTEL_EXPORTER_OTLP_ENDPOINT`, `_INSECURE`, `_PROTOCOL` | Common OTLP settings for all signals |
| `OTEL_EXPORTER_OTLP_{TRACES,METRICS,LOGS}_ENDPOINT`, `_INSECURE`, `_PROTOCOL` | Per-signal overrides |

### `http` / `grpc`

`config.HTTP` and `config.GRPC`, and every section embedding `config.Network`. The env prefix is the section's (`API_`, `OPS_`, …).

| Key | Env suffix | Default | Description |
|---|---|---|---|
| `address` | `ADDRESS` | `:8080` (`config.HTTP`) / `:9090` (`config.GRPC`) | Listen address |
| `read_timeout` | `READ_TIMEOUT` | `0` | HTTP only |
| `read_header_timeout` | `READ_HEADER_TIMEOUT` | `0` | HTTP only |
| `write_timeout` | `WRITE_TIMEOUT` | `0` | HTTP only |
| `idle_timeout` | `IDLE_TIMEOUT` | `0` | HTTP only |
| `max_header_bytes` | `MAX_HEADER_BYTES` | `0` | HTTP only |
| `shutdown_timeout` | `SHUTDOWN_TIMEOUT` | `30s` | Graceful stop budget |
| `tls.*` | `TLS_*` | — | See [TLS](#tls) |

### TLS

| Key | Env suffix | Default | Description |
|---|---|---|---|
| `enabled` | `TLS_ENABLED` | `false` | |
| `cert_file`, `key_file` | `TLS_CERT_FILE`, `TLS_KEY_FILE` | — | PEM files |
| `min_version` | `TLS_MIN_VERSION` | `TLS13` | `TLS10` … `TLS13`; prefer `TLS12`+ |
| `client_auth` | `TLS_CLIENT_AUTH` | `no-client-cert` | `request-client-cert`, `require-any-client-cert`, `verify-client-cert-if-given`, `require-and-verify-client-cert` (mTLS) |
| `ca_cert_file` | `TLS_CA_CERT_FILE` | — | Required when client certificates are verified |
| `cipher_suites` | `TLS_CIPHER_SUITES` | — | TLS ≤ 1.2 only; rejected together with `min_version: TLS13` |

`tls` is a pointer section: with no `tls` key and no `*_TLS_*` variable it stays `nil` (TLS off). Once a file, env or flag sets any of its fields, the other fields start from the defaults above, so `min_version` and `client_auth` can be left out.
