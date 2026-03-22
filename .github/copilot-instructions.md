# Copilot instructions for go-bones

This repository is a small application foundation for Go services. Keep suggestions aligned with the existing package boundaries and public API style.

## Project shape

- Treat `config`, `logger`, `service`, `network/http`, `network/grpc`, and `tracer` as the core packages.
- Prefer small, composable changes over broad refactors.
- Preserve backward compatibility unless the user explicitly asks for a breaking change.

## Go conventions

- Target the Go version declared in `go.mod`.
- Prefer standard library solutions when they are sufficient.
- Keep code idiomatic, explicit, and easy to test.
- Use context-aware APIs for long-running work, startup, shutdown, and network operations.

## Golang version and updates

- The minimum supported Go version for this repository is `1.26`.
- Treat the following version features as the only safe language/tooling assumptions when generating code or examples.
- Go `1.23`:
  - `for range` over iterator functions.
  - Preview support for generic type aliases.
  - New `iter`, `unique`, and `structs` packages.
  - `slices` and `maps` gain iterator-friendly helpers.
  - Go telemetry is available as an opt-in toolchain feature.
- Go `1.24`:
  - Generic type aliases are fully supported.
  - `go.mod` can track tool dependencies with `tool` directives.
  - `go tool` can run those tracked tools.
  - Runtime and map performance improvements, including Swiss Table-based maps.
- Go `1.25`:
  - No language changes that affect Go programs.
  - Experimental Green Tea garbage collector.
  - Experimental `encoding/json/v2` and `encoding/json/jsontext`.
  - Broad toolchain, runtime, and library improvements with compatibility preserved.
- Go `1.26`:
  - `new` now accepts an expression operand for initializing a new variable.
  - Generic types may refer to themselves in their own type parameter list.
  - Green Tea garbage collector is enabled by default.
  - Baseline cgo call overhead is reduced.

When in doubt, prefer APIs and examples that are available in Go `1.26` and documented in the official release notes.

## Golang CI Linter

- Follow the rules in `.golangci.yml`; treat them as the source of truth for formatting and code style.
- The active formatter set is `gci`, `gofmt`, `gofumpt`, `goimports`, and `golines`; prefer code that stays stable after all of them run.
- The active linter set includes `unparam`, `whitespace`, `unconvert`, `bodyclose`, `gocritic`, `godot`, `prealloc`, `rowserrcheck`, `lll`, `cyclop`, `gosec`, `gochecknoglobals`, and `funlen`.
- Avoid overly long lines; keep code readable without relying on `golines` to reflow it later.
- Keep functions focused and small enough to avoid `funlen` and `cyclop` violations.
- Avoid package-level mutable state to satisfy `gochecknoglobals`.
- Prefer preallocated slices and buffers where the size is known or easy to estimate to satisfy `prealloc`.
- Close response bodies, readers, and other resources explicitly to satisfy `bodyclose`.
- Write straightforward, explicit error handling so `rowserrcheck` and similar checks stay quiet.
- Be careful with unnecessary conversions and redundant code patterns so `unconvert`, `gocritic`, and `whitespace` do not flag the result.
- Keep security-sensitive code conservative and avoid introducing obvious `gosec` findings.
- Test files may be exempt from some linter rules, but production code should still follow the same general style.

## Code Review

- When performing a code review, verify the correctness of the code, carefully look for potential issues, and review the changes against best practices.
- Focus first on correctness, regressions, compatibility, security, and test coverage.
- Call out concrete bugs, missing edge cases, API breaks, and violations of the repository conventions above.
- Verify that new code follows the Go version baseline and the `.golangci.yml` rules.
- Prefer minimal, pragmatic solutions and apply YAGNI, DRY, and KISS.
- Prefer short, actionable review comments over general style feedback unless the style issue would cause a lint failure or a maintenance problem.
- If there are no significant findings, say so explicitly and mention any residual risks or testing gaps.

## Configuration

- Follow the existing pattern of one top-level app config embedding `config.Base`.
- Keep env, YAML, JSON, and TOML tags consistent with the existing style.
- Prefer `config.Load` as the entry point for application configuration.
- When adding new config options, document sensible defaults and propagation rules.

## Services and transports

- Use `service.Run` and the service composition helpers for lifecycle management.
- For HTTP and gRPC transports, keep the options-based API style already used in the repo.
- Preserve the built-in OPS, health, and OpenTelemetry integration patterns.
- Keep shutdown logic graceful and deterministic.

## Logging and telemetry

- Initialize logging once at startup.
- Use the `logger` package for structured logs and keep secret redaction behavior intact.
- Prefer `OTEL_*` environment variables and the existing tracer bootstrap conventions.
- Do not add telemetry abstractions unless they are clearly needed by the repository.

## Testing

- Prefer table-driven tests for behavior-heavy code.
- Keep tests hermetic and avoid network or time dependencies unless they are intentional integration tests.
- Reuse helpers from `internal/testutil` when adding integration coverage.
- Add or update tests when changing public behavior.

## Documentation

- Update `README.md` or package `doc.go` files when public behavior changes.
- Keep examples short and consistent with the exported API.
- Avoid restating the same implementation details in multiple places.
