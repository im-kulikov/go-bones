# Testing

Read when: writing or fixing tests in this repo. For testing an application
built on go-bones see the user guide [Testing](../testing.md).

## Commands

| Goal | Command |
|---|---|
| one package | `go test -race -count=1 ./health/...` |
| one test | `go test -race -count=1 -run 'TestRegisterValidation' ./health/` |
| everything | `make test`: always `./...` (a package argument is only printed), `-race`, 70% gate |
| coverage of a package | `go test -coverprofile=/tmp/c.out ./health/ && go tool cover -func=/tmp/c.out \| tail -1` |
| vet / lint | `make vet`, `make lint` |

CI runs `go test ./...` without `-race`, plus golangci-lint. Coverage: aim for
100% per package. The maintainer keeps it in his commits; it is not required
from contributors, and tooling enforces only 70%. Check your package with the
command above. Tests must pass with `-race`.

## Style

- White-box tests in the package itself (`package health`); godoc examples in
  `<package>/example_test.go` with the external package (`package health_test`).
- `testify`: `require` for preconditions and fatal checks, `assert` for the
  rest. Prefer hand-written fakes to `testify/mock`.
- Table tests as `map[string]struct{…}` or a slice of cases with `t.Run`.
- Names: `TestType_Behavior` (newer) or `Test_function` (older); follow the file.
- Lint rules mostly apply to tests too; `.golangci.yml` relaxes only
  `funlen`, `gosec`, `errcheck`, `dupl`, `gocyclo` for `_test.go`.

## Time and concurrency

- Anything with timers, intervals or timeouts runs in `testing/synctest`
  (`synctest.Test`, `synctest.Wait`) — no `time.Sleep`, no wall-clock waits.
- Randomness is behind seams: set `Monitor.jitter` to identity; `NewTicker`
  has no jitter unless `WithTickerJitter` is given.
- Signals and listeners are injected: `settings.newSignal`/`notify` in
  `service`, `ListenOpener` in transports.
- Tests that touch process-wide state (default logger, OTel globals and
  bridge, OPS registry, `app` application, env vars) restore it in
  `t.Cleanup` and do not call `t.Parallel`.

## Helpers

| File | Helper |
|---|---|
| `internal/testutil/doc.go` | package godoc |
| `internal/testutil/integration.go` | `RequireNetworkIntegration` (tags the test, skips only inside the Codex seatbelt sandbox), `FreeTCPAddr`, `WriteArtifact` (`t.ArtifactDir()`; with `go test -artifacts` files land in `_artifacts/`, gitignored) |
| `internal/testutil/otel.go` | `InstallOTelRecorder`: swaps OTel globals, records spans and logs, restores on cleanup |

From `logger`: `ForTests`, `TestLoggerWriteToTB(t)`, `NewSyncBuffer`. From
`service`: `TestEnv(t, fakes...)`. From `config`: `Defaults[T]()`.

Socket tests go to `*_integration_test.go`, bind `127.0.0.1` via
`FreeTCPAddr`, and call `RequireNetworkIntegration` first.
