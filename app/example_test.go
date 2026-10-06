package app_test

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

// newAPI is a component constructor: its config section and the application environment.
func newAPI(cfg config.HTTP, env service.Env) (service.Service, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello, world"))
	})

	return http.NewServer(cfg, env.Logger, http.ServiceName("api"), http.WithHandler(mux))
}

// A whole application: config, logger, OpenTelemetry, health probes on the OPS
// server, the API and a phased shutdown.
func Example() {
	cfg := app.Init[settings](config.WithVersion("1.0.0"))

	app.Add(cfg.API, newAPI)

	app.Run()
}
