# AGENTS.md

go-bones (`github.com/im-kulikov/go-bones`, Go 1.26) is a library: the
production skeleton for Go services — config, `log/slog` logging, a lifecycle
runner with phased shutdown, a non-blocking health monitor, HTTP/gRPC/OPS
servers and OpenTelemetry bootstrap. Stdlib-first: no DI container, no
reflection-based wiring, no codegen. Pre-1.0.

## How to read the docs

Read this file whole. Then open **only** the pages your task needs, from the
routing table below — usually one package page. Go further (user guides in
`docs/`, code) only when that page is not enough. Do not preload everything.

## Routing

| Task or path you touch | Read | Then, if needed |
|---|---|---|
| first time here, cross-package change | [architecture](docs/agents/architecture.md) | |
| `app/`, root package files | [architecture › lifecycle](docs/agents/architecture.md#application-lifecycle) | [service](docs/agents/service.md) |
| `config/` | [config](docs/agents/config.md) | [Configuration](docs/configuration.md) |
| `logger/` | [logger](docs/agents/logger.md) | [logger review rules](.github/instructions/logger.instructions.md) |
| `service/` | [service](docs/agents/service.md) | [service review rules](.github/instructions/service.instructions.md) |
| `health/` | [health](docs/agents/health.md) | [Health checks](docs/health.md) |
| `network/`, `network/http/`, `network/grpc/` | [transports](docs/agents/transports.md) | [HTTP & gRPC](docs/transports.md), [Ops](docs/ops.md) |
| `tracer/` | [tracer](docs/agents/tracer.md) | [tracer review rules](.github/instructions/tracer.instructions.md) |
| `internal/` | [architecture › packages](docs/agents/architecture.md#packages) | |
| `internal/testutil/`, writing tests | [testing](docs/agents/testing.md) | [Testing guide](docs/testing.md) |
| finishing any change | [workflow](docs/agents/workflow.md) | |
| code review | package page "Invariants" + `.github/instructions/` | [review checklist](.github/copilot-instructions.md#code-review) |
| Go version, style, lint | [Copilot instructions](.github/copilot-instructions.md) | `.golangci.yml` |

## Commands

| Goal | Command |
|---|---|
| test a package | `go test -race -count=1 ./health/...` |
| all tests | `make test` — always `./...` with `-race` and a 70% gate; CI runs `go test -race ./...` on the two latest Go releases + lint |
| doc-sync guard | `go test -count=1 -run TestAgentDocs .` |
| vet, lint | `make vet`, `make lint` (golangci-lint v2) |

Do not run `make deps` unless the user asks: it runs `go mod tidy` and vendors
modules into `vendor/` (README lists it for humans).

## Rules

- Imports go down the layers: `health` never imports `service`, nothing
  imports `app`, `internal/` is not public API.
- Health readers (probes, gRPC health, metrics) never do I/O; they read the
  monitor's snapshot.
- Shutdown order is a contract: drain → drain delay → stop services → stop
  OPS and the monitor last.
- Secrets are masked before any log export; raw health errors never leave the
  process.
- Aim for 100% coverage per package (the maintainer keeps it; it is not
  required from contributors, tooling enforces 70%) and stay `-race` clean;
  no new modules without a real need.
- A new global needs `//nolint:gochecknoglobals // <reason>`; prefer seams as
  struct fields.
- Do not commit or push unless asked; leave `tmp/`, `temp/`, `_artifacts/`
  alone.

## Self-documentation

Every change updates the docs it affects, in the same change: godoc, the
package page in `docs/agents/`, the user guide, `AGENTS.md` routing for a new
package. The checklist is in [workflow](docs/agents/workflow.md#what-to-update).
When you learn something non-obvious about the code, write it down in the
package page's "Invariants" before you finish. `agents_test.go` fails on an
undocumented file or package, an unlinked page, a broken link or an oversized
page. End your report with the docs you updated, or
`docs: no change needed — <reason>`.
