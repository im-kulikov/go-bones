# HTTP & gRPC

- [HTTP server](#http-server)
- [gRPC server](#grpc-server)
- [TLS and mTLS](#tls-and-mtls)
- [Instrumenting your own code](#instrumenting-your-own-code)

Both packages re-export the stdlib / gRPC types you need (`http.Handler`, `http.ServeMux`, `http.StatusOK`, `grpc.Server`, …), so one import is enough and there is no `nethttp` alias dance.

## HTTP server

```go
api, err := http.NewServer(cfg.API, log,
	http.ServiceName("api"),
	http.WithHandler(mux),
	http.WithOpenTelemetry(),
)
```

`cfg.API` is a `config.HTTP` (address `:8080` by default) or any section implementing `config.INetwork` (`Addr() string` plus an embedded `config.Network`). With the facade this call lives in a component constructor, `func New(cfg config.HTTP, env service.Env) (service.Service, error)`, added with `app.Add(cfg.API, api.New)`.

Behavior worth knowing:

- the listener is opened in `Start`; a busy port fails the application immediately;
- graceful stop uses `shutdown_timeout`, then force-closes connections;
- `http.WithOpenTelemetry()` continues incoming trace context and creates server spans;
- any other `*http.Server` field can be set with `http.ServerOptions(func(s *http.Server) { ... })`.

Any router works — `http.WithHandler` takes an `http.Handler`, so the stdlib `ServeMux`, chi, gorilla/mux or ConnectRPC handlers all plug in.

## gRPC server

```go
rpc, err := grpc.NewServer(cfg.GRPC, log,
	grpc.ServiceName("rpc"),
	grpc.WithOpenTelemetry(),
	grpc.WithHealth(hc),
	grpc.RegisterServices(func(s *grpc.Server) {
		ordersv1.RegisterOrdersServer(s, orders)
	}),
)
```

With the facade: the same call in a component constructor, with `grpc.WithHealth(env.Health)`.

- `grpc.WithHealth(hc)` registers `grpc.health.v1` backed by the health monitor — see [Health › gRPC](health.md#grpc-health). Don't register another health server in `RegisterServices`: `NewServer` returns `grpc.ErrGRPCHealthRegistered`.
- `grpc.WithOpenTelemetry()` installs unary and stream server interceptors that continue incoming trace context.
- On shutdown: health switches to `NOT_SERVING`, then `GracefulStop` within `shutdown_timeout`, then `Stop`.
- Extra `grpc.ServerOption`s: `grpc.ServerOptions(grpc.MaxRecvMsgSize(16 << 20))`, e.g. your own interceptors.

## TLS and mTLS

Every server reads TLS from its `tls` section:

```yaml
grpc:
  address: ":9443"
  tls:
    enabled: true
    cert_file: /etc/tls/tls.crt
    key_file: /etc/tls/tls.key
    client_auth: require-and-verify-client-cert # mTLS
    ca_cert_file: /etc/tls/ca.crt
```

Misconfiguration (missing key pair, mTLS without a CA, cipher suites with TLS 1.3, unknown suite names) fails at startup with a clear error. Full key list: [Configuration › TLS](configuration.md#tls).

## Instrumenting your own code

Incoming requests are traced by servers built with `WithOpenTelemetry()`. Outgoing calls need an instrumented client, otherwise traces stop at the service boundary — use the OpenTelemetry contrib packages with the global providers that `tracer.Init` installs:

```go
client := &http.Client{Transport: otelhttp.NewTransport(nil)} // nil: http.DefaultTransport

conn, err := grpc.NewClient("payments:9090",
	grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	grpc.WithTransportCredentials(insecure.NewCredentials()),
)
```

`otelhttp` is `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp`, `otelgrpc` is `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc`; `grpc` in this snippet is `google.golang.org/grpc`. These are your dependencies, not go-bones'.
