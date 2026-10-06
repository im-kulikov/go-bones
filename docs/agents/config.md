# config

Read when: adding or changing a config field or section, loading, TLS.
User view: [Configuration](../configuration.md) — its
[Reference](../configuration.md#reference) is the env/key table users rely on.

## Files

| File | What it holds |
|---|---|
| `config/doc.go` | package godoc with an example |
| `config/config.go` | `Load`, `Option`s (`WithName`, `WithVersion`, `WithYAML/JSON/TOML`, gonfig passthroughs), `buildVersion` |
| `config/base.go` | `Base` (root of every app config, `Bones()`), `Network`, `INetwork`, `appSettings` and the reflection walk `setAppSettings` |
| `config/alias.go` | `DefaultConfigFlag` (`--config`/`-c`), `PrintConfigFlag` (`--print-config`) from gonfig |
| `config/defaults.go` | `Defaults[T]()` — a section with `default` tags applied |
| `config/logger.go` | `Logger` section |
| `config/ops.go` | `Ops` section, `IsEnabled` |
| `config/health.go` | `Health` section, `WithDefaults`, `DefaultHealth*` constants |
| `config/server.go` | ready-made `HTTP` (`:8080`) and `GRPC` (`:9090`) sections |
| `config/tls.go` | `TLS` section, `Prepare` → `*tls.Config`, TLS errors |
| `config/tracer.go` | `TracerConfig` (fallbacks for `OTEL_*`) |

## How loading works

- `Load` = gonfig: `default` tags → config file (`--config`) → env → flags.
  YAML is on unless another parser option is given.
- Env name = section env tag + field env tag joined by `_`; embedded structs
  (`Base`, `Network`) add no prefix. `Base` maps sections to `LOGGER`, `OPS`,
  `OTEL` (tracer!) and `HEALTH`.
- After loading, `setAppSettings` walks nested struct fields (not pointers,
  slices, maps) and calls `SetAppNameAndVersion` on every embedded
  `appSettings` (`Logger`, `TracerConfig`, `Network`). Name/version default to
  build info: module path and version, or the VCS revision (`-dirty`).

## Invariants

- Every field carries `env`, `yaml`, `json` and `toml` tags. Follow the style of
  the struct you edit (`Network`/`TLS` use camelCase json, newer sections
  snake_case); new sections use snake_case everywhere. Tag-heavy structs carry
  `//nolint:lll`.
- Defaults live in `default` tags. A struct literal has zero values — code and
  tests use `Defaults[T]()`. `Ops{}` has every switch false, so
  `NewOPSServer` returns a nil service.
- `Health` has defaults twice: tags and `DefaultHealth*` used by `WithDefaults`
  (so `health.New(config.Health{})` works). `TestHealth_TagDefaultsMatchConstants`
  keeps them equal — change both.
- `Base.Bones()` is what `app.Init` uses to find the go-bones part; `Base` must
  stay embeddable and keep that method.
- `TLS.Prepare` validates before touching files; `ErrTLSDisabled` means "no
  TLS" and callers treat it as a non-error. Cipher suites with `TLS13` are
  rejected (`ErrCipherSuitesIneffectiveAtTLS13`) on purpose.
- A pointer section (`Network.TLSConfig *TLS`) stays nil until some source
  sets one of its fields; then gonfig (v0.7.0+) applies its `default` tags
  (`config/tls_test.go` `TestNetwork_TLSFromFileUsesDefaults`).
- Mark secret values with `secret:"true"` so `--help`/`--print-config` hide them.

## When you change something

- New or renamed key → update [Configuration › Reference](../configuration.md#reference)
  (key, env, default, description) and the example YAML there if relevant.
- A new section in `Base` → `app.Init` wiring ([architecture](architecture.md#application-lifecycle))
  and `docs/configuration.md` "The model" table.
- Tests: `go test -race ./config/...`; table tests per section live next to it
  (`config/*_test.go`), the godoc example is `config/example_test.go`.
