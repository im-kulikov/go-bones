# Testing

## A logger for tests

```go
func TestHandler(t *testing.T) {
	log := logger.ForTests(logger.TestLoggerWriteToTB(t)) // output goes to `go test -v`
	// ...
}
```

Capture output to assert on it:

```go
buf := logger.NewSyncBuffer()
log := logger.ForTests(logger.TestLoggerWriter(buf))

doSomething(log)

require.Contains(t, string(buf.Bytes()), `msg="order created"`)
```

`logger.TestLoggerSecrets("token")` adds keys to mask, so you can test that secrets never leak. `ForTests` also masks `time` and `Time` by default, so the output is stable — don't assert on timestamps.

## Testing one component

A component constructor is a plain function of its config and a `service.Env`. `service.TestEnv` gives it the test context, a test logger, a fresh health monitor, and the values `service.Get` should find — usually fakes:

```go
func TestResolver(t *testing.T) {
	store := &fakeStore{domains: []string{"example.com"}}

	svc, err := resolver.New(resolver.Config{Servers: []string{dnsAddr}}, service.TestEnv(t, store))
	require.NoError(t, err)
	// ...
}
```

A missing fake fails the constructor with `needs storage.DNS, nothing built before provides it`.

## Assembling a stack in tests

`service.Build` is what `app.Add` does, minus exiting the process — handy for end-to-end tests that assemble several components:

```go
env := service.TestEnv(t) // values built below accumulate in env

manager, err := service.Build(env, broadcast.Config{}, broadcast.New)
require.NoError(t, err)
_, err = service.Build(env, storage.Config{}, storage.New) // gets manager with service.Get
require.NoError(t, err)
```

## Health checks in tests

The monitor is a service: start it, then poll `Snapshot`:

```go
hc := health.New(config.Health{}, logger.ForTests())
require.NoError(t, hc.Register("db", health.CheckerFunc(func(context.Context) error { return errDown })))

go func() { _ = hc.Start(t.Context()) }()

require.Eventually(t, func() bool {
	return hc.Snapshot().Checks["db"].Status == health.StatusFailing
}, time.Second, 10*time.Millisecond)
require.False(t, hc.Snapshot().Ready)
```

A component under test registers its checks in `env.Health` from `service.TestEnv`, so the same polling works there.

## Without helpers

To run real servers, bind them to `127.0.0.1:0` or a free port and run them with `service.RunContext`; it stops when the context is canceled:

```go
ctx, cancel := context.WithCancel(t.Context())
errCh := make(chan error, 1)
go func() { errCh <- service.RunContext(ctx, log, service.WithService(svc)) }()

// ... exercise svc ...

cancel()
require.NoError(t, <-errCh) // context.Canceled is a clean shutdown
```
