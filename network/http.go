package network

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

type httpServer struct {
	name string
	done chan struct{}
	base config.BaseHTTP

	*http.Server
	*logger.Logger
}

type (
	// HTTPOption for httpService.
	HTTPOption func(*httpServer)

	// HTTPServerOption for http.Server.
	HTTPServerOption func(*http.Server)
)

const (
	defaultHTTPNetwork     = "tcp"
	defaultHTTPServiceName = "http-service"

	httpServerStarting  = "http server starting"
	httpServerStopped   = "http server stopped"
	httpsServerStarting = "https server starting"
	httpsServerStopped  = "https server stopped"

	// ErrHTTPCheckListener fires when could not use provided address.
	ErrHTTPCheckListener bones.Error = "http check listener"

	// ErrHTTPCloseListener fires when could not close test listener.
	ErrHTTPCloseListener bones.Error = "http close listener"
)

// HTTPServiceName allows to set httpService name.
func HTTPServiceName(name string) HTTPOption {
	return func(settings *httpServer) { settings.name = name }
}

// HTTPServerOptions allows to set HTTPServerOptions to http.Server.
func HTTPServerOptions(opts ...HTTPServerOption) HTTPOption {
	return func(s *httpServer) {
		for _, opt := range opts {
			opt(s.Server)
		}
	}
}

// HTTPOptions allows to set multiple HTTPOption's at once.
func HTTPOptions(opts []HTTPOption) HTTPOption {
	return func(settings *httpServer) {
		for _, opt := range opts {
			opt(settings)
		}
	}
}

// Name of httpService.
func (h *httpServer) Name() string { return h.name }

// Start runs http.Server and wait for stop.
func (h *httpServer) Start(ctx context.Context) error {
	if l, err := new(net.ListenConfig).Listen(ctx, defaultHTTPNetwork, h.name); err != nil {
		return errors.Join(err, ErrHTTPCheckListener)
	} else if err = l.Close(); err != nil {
		return errors.Join(err, ErrHTTPCloseListener)
	}

	// should be called when http.Server will be running.
	go func() { close(h.done) }()

	if h.base.TLSConfig == nil {
		h.Info(httpServerStarting,
			logger.String("service", h.name),
			logger.String("address", h.Addr))

		return h.ListenAndServe()
	}

	h.Info(httpsServerStarting,
		logger.String("service", h.name),
		logger.String("address", h.Addr))

	return h.ListenAndServeTLS(
		h.base.TLSConfig.CertFile,
		h.base.TLSConfig.KeyFile,
	)
}

func (h *httpServer) Stop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-h.done:
	}

	msg := httpServerStopped
	if h.base.TLSConfig != nil {
		msg = httpsServerStopped
	}

	h.Info(msg,
		logger.String("service", h.name),
		logger.String("address", h.Addr),
		logger.Err(h.Shutdown(ctx)))
}

func NewHTTPServer(
	cfg config.HTTPConfig,
	log *logger.Logger,
	handler http.Handler,
	opts ...HTTPOption,
) (service.Service, error) {
	srv, err := cfg.PrepareHTTPServer()
	if err != nil {
		return nil, err
	}

	serve := &httpServer{name: defaultHTTPServiceName, Logger: log, Server: srv}
	for _, opt := range opts {
		opt(serve)
	}

	serve.Server.Handler = handler
	serve.done = make(chan struct{})

	return serve, nil
}
