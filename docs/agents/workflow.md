# Workflow: finishing a change

Read when: before you report a change as done. This page is the
self-documentation protocol: the docs are part of every change, not a
follow-up.

## Definition of done

1. Code is in the right layer ([architecture](architecture.md#layers)).
2. Tests cover it (aim for 100% per package, `-race`), see
   [testing](testing.md).
3. Every new or changed exported symbol has godoc that ends with a period
   (`godot`); a package overview lives in its `doc.go`.
4. Docs are updated in the same change — use the table below.
5. `go test -count=1 -run TestAgentDocs .` passes (the doc-sync guard).
6. `gofmt`/lint clean (`make lint` when `golangci-lint` is installed; say so
   if it is not).
7. Your final message lists the docs you updated, or says
   `docs: no change needed — <reason>`.

## What to update

| You changed | Update |
|---|---|
| added, removed or renamed a non-test `.go` file | the Files table of the package doc in `docs/agents/` |
| added a package | a new `docs/agents/<name>.md` (same shape: Read when, Files, Invariants, Tests), a row in the `AGENTS.md` routing table, [architecture](architecture.md) Layers and Packages, README "Packages" |
| an import between packages | [architecture › Layers](architecture.md#layers) |
| a contract, ordering, invariant, or anything you had to dig out of the code | "Invariants" of the package doc |
| a reviewer finding that is a false positive by design | "Do not flag" in `.github/instructions/<package>.instructions.md` (create it with `applyTo: "<package>/**"`) |
| process-wide state (a global, an OTel/env side effect) | [architecture › Process-wide state](architecture.md#process-wide-state) |
| user-visible behavior or public API | the user guide from the map below; a godoc `Example` for new API |
| a config key, env name or default | [Configuration › Reference](../configuration.md#reference) |
| a new feature | README "What you get" and [architecture › Feature → code](architecture.md#feature--code) |
| commands, tooling, CI | `AGENTS.md` Commands and [testing](testing.md) |
| a doc that turned out wrong | fix it now, in this change |

## User guide map

| Area | Page |
|---|---|
| first service, components, manual wiring | [getting-started](../getting-started.md) |
| config structs, load order, env reference | [configuration](../configuration.md) |
| services, workers, tickers, shutdown and drain | [lifecycle](../lifecycle.md) |
| HTTP/gRPC servers, TLS, instrumentation | [transports](../transports.md) |
| checks, probes, gRPC health, health metrics | [health](../health.md) |
| `/metrics`, pprof, expvar, `/version` | [ops](../ops.md) |
| logging, OpenTelemetry, correlation | [observability](../observability.md) |
| probes, grace periods, Deployment | [kubernetes](../kubernetes.md) |
| testing an app built on go-bones | [testing](../testing.md) |
| comparisons, troubleshooting | [faq](../faq.md) |

## The doc-sync guard

`agents_test.go` (package `bones`) runs with `go test ./...`, `make test` and
CI, and in lefthook on commits that touch `.go`, `.yml` or `.md` files. It
fails when:

- a non-test `.go` file is not named as a backticked repo path (like
  `health/monitor.go`) in some `docs/agents/*.md`;
- a backticked `.go` path in `AGENTS.md` or `docs/agents/` no longer exists;
- a package directory is not named as `` `dir/` `` in `AGENTS.md`;
- a `docs/agents/*.md` page is not linked from `AGENTS.md`;
- a relative link or `#anchor` in `AGENTS.md` or `docs/agents/` is broken;
- `AGENTS.md` or an agent doc outgrows its line budget. Split a page or move
  details down a level; never inline everything into `AGENTS.md`.

The guard checks structure, not truth: a file being named does not mean its
description is right, and links inside the user guides are not checked.
Keeping the prose true is your job.

## Writing agent docs

- Facts an agent cannot get quickly from the code: why, ordering, contracts,
  traps. Link to user guides instead of copying them.
- One topic per page, one screen per section, English, plain sentences.
- Name code by repo-relative path and symbol (`health/check.go` `sweep`). A
  bare file name (`doc.go`) means a file in the repo root; write a generic
  file as `<package>/example_test.go`.

## Repo rules

- Do not break public API or documented behavior unless the user asks.
  When they do, mark it `BREAKING:` in the commit body for the release notes.
- Stdlib first; no new module without a real need; `go.mod`/`go.sum` stay
  `go mod tidy` clean (lefthook checks). Do not run `make deps` unless the
  user asks: it runs `go mod tidy` and vendors into `vendor/`.
- Do not commit or push unless asked. Commit subject:
  `<package>: <lowercase imperative>` (`service: add NewTicker for periodic
  tasks`, `docs: …`).
- `tmp/` and `temp/` are the owner's local scratch (excluded in
  `.git/info/exclude`): do not edit or commit them. `temp/` is still a Go
  package tree, so `./...` includes it while it exists. `_artifacts/` and
  `coverage.txt` are test output.
- Code review: `.github/copilot-instructions.md` ("Code Review") plus the
  package doc's Invariants and `.github/instructions/*` "Do not flag".
