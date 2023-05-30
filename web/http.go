package web

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

// HTTPConfig provides configuration for http server.
type HTTPConfig struct {
	Enabled bool   `env:"ENABLED" default:"false" usage:"allows to enable http server"`
	Address string `env:"ADDRESS" default:":8080" usage:"HTTP server listen address"`
	Network string `env:"NETWORK" default:"tcp" usage:"HTTP server listen network: tpc/udp"`
	NoTrace bool   `env:"NO_TRACE" default:"false" usage:"allows to disable tracing for HTTP server"`
}

type httpServer struct {
	HTTPConfig

	name   string
	handle http.Handler
	server *http.Server
	logger logger.Logger
}

const defaultHTTPName = "http-server"

// NewHTTPServer creates http server.
func NewHTTPServer(opts ...HTTPOption) service.Service {
	serve := &httpServer{
		name:   defaultHTTPName,
		logger: logger.Default(),

		HTTPConfig: HTTPConfig{Enabled: true},
	}

	for _, o := range opts {
		o(serve)
	}

	return serve
}

// Name returns name of http server.
func (s *httpServer) Name() string { return s.name }

// Enabled returns is service enabled.
func (s *httpServer) Enabled() bool { return s.HTTPConfig.Enabled }

// Start allows starting http server.
func (s *httpServer) Start(ctx context.Context) error {
	log := s.logger.With(
		"name", s.name,
		"address", s.Address,
		"network", s.Network)

	log.Info("prepare listener")

	lis, err := new(net.ListenConfig).Listen(ctx, s.Network, s.Address)
	if err != nil {
		return err
	}

	handler := s.handle
	if !s.NoTrace {
		handler = HTTPTracingMiddleware(s.handle)
	}

	s.server = &http.Server{
		// to prevent default std logger output
		Handler:  handler,
		ErrorLog: log.Std(),

		// G112: Potential Slowloris Attack because ReadHeaderTimeout is not configured in the http.Server
		ReadHeaderTimeout: time.Second * 10,

		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s.server.Serve(lis)
}

// Stop allows stop http server.
func (s *httpServer) Stop(ctx context.Context) {
	if s.server == nil {
		return
	}

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Errorw("could not stop server", "error", err)
	}
}
