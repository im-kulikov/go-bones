package network

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

// ListenOpener defines an interface for creating network listeners.
// Implementations should handle the creation of network listeners with the specified
// context, address, and network protocol.
type ListenOpener interface {
	Listen(ctx context.Context, address, network string) (net.Listener, error)
}

type httpOptions struct {
	name string
	open ListenOpener
	base config.BaseHTTP

	*http.Server
	*logger.Logger
}

type (

	// HTTPOption defines a function type for configuring HTTP server options.
	// It accepts and modifies httpOptions struct.
	HTTPOption func(*httpOptions)

	// HTTPServerOption defines a function type for configuring http.Server instances.
	// It allows direct modification of the standard http.Server settings.
	HTTPServerOption func(*http.Server)
)

const (
	defaultHTTPNetwork     = "tcp"
	defaultHTTPServiceName = "http-service"

	httpServerStarting  = "http server starting"
	httpsServerStarting = "https server starting"

	// ErrHTTPCheckListener indicates a failure during the initialization of the HTTP listener.
	ErrHTTPCheckListener bones.Error = "http check listener"

	// ErrHTTPCloseListener indicates a failure when attempting to close the HTTP listener.
	ErrHTTPCloseListener bones.Error = "http close listener"

	// ErrHTTPShutdownServer indicates a failure during the shutdown process of the HTTP server.
	ErrHTTPShutdownServer bones.Error = "http shutdown server"
)

// HTTPServiceName sets a custom name for the HTTP service.
// This name is used for logging and identification purposes.
func HTTPServiceName(name string) HTTPOption {
	return func(settings *httpOptions) { settings.name = name }
}

// HTTPServerOptions applies a collection of server-specific configurations.
// It allows chaining multiple server options for the internal http.Server instance.
func HTTPServerOptions(opts ...HTTPServerOption) HTTPOption {
	return func(s *httpOptions) {
		for _, opt := range opts {
			opt(s.Server)
		}
	}
}

// HTTPOptions combines multiple HTTP options into a single configuration function.
// It sequentially applies each option in the provided slice.
func HTTPOptions(opts []HTTPOption) HTTPOption {
	return func(settings *httpOptions) {
		for _, opt := range opts {
			opt(settings)
		}
	}
}

// NewHTTPServer creates a new HTTP service with the specified configuration.
// It sets up the server with the provided logger and request handler,
// and allows additional customization through options.
func NewHTTPServer(cfg config.HTTPConfig, log *logger.Logger, handler http.Handler, opts ...HTTPOption) (service.Service, error) {
	options, err := prepareHTTPServer(cfg, log, handler, opts...)
	if err != nil {
		return nil, err
	}

	return service.NewLauncher(options.name, options.listen, func(ctx context.Context) {
		options.InfoContext(ctx, "shutdown http service",
			logger.String("service", options.name),
			logger.String("address", options.Addr))
	}), nil
}

// The newHTTPServer initializes and returns an http.Server configured with the provided HTTPConfig.
// It prepares TLS configuration if enabled and returns an error on failure excluding a disabled TLS scenario.
func newHTTPServer(c config.HTTPConfig) (*http.Server, error) {
	var err error
	base := c.Base()

	var cfg *tls.Config
	if cfg, err = base.PrepareTLSConfig(); err != nil && !errors.Is(err, config.ErrTLSDisabled) {
		return nil, err
	}

	return &http.Server{
		Addr:              c.Addr(),
		TLSConfig:         cfg,
		ReadTimeout:       base.ReadTimeout,
		ReadHeaderTimeout: base.ReadHeaderTimeout,
		WriteTimeout:      base.WriteTimeout,
		IdleTimeout:       base.IdleTimeout,
		MaxHeaderBytes:    base.MaxHeaderBytes,
	}, nil
}

// prepareHTTPServer configures and prepares an HTTP server with provided configuration, logger, handler, and options.
// It returns the configured httpOptions or an error on failure.
func prepareHTTPServer(
	cfg config.HTTPConfig,
	log *logger.Logger,
	handler http.Handler,
	opts ...HTTPOption,
) (*httpOptions, error) {
	srv, err := newHTTPServer(cfg)
	if err != nil {
		return nil, err
	}

	options := &httpOptions{
		Logger: log,
		Server: srv,
		base:   cfg.Base(),
		open:   new(net.ListenConfig),
		name:   defaultHTTPServiceName,
	}
	for _, opt := range opts {
		opt(options)
	}

	options.Server.Handler = handler

	return options, nil
}

// serve starts the HTTP server, selecting between plain HTTP or HTTPS based on the TLS configuration.
func (h *httpOptions) serve() error {
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

// The listen handles the initialization and operation of the HTTP server, including listening, serving, and shutdown.
// Returns an error if the listener setup, server operation, or shutdown process encounters an issue.
func (h *httpOptions) listen(top context.Context) error {
	if lis, err := h.open.Listen(top, defaultHTTPNetwork, h.Addr); err != nil {
		return errors.Join(ErrHTTPCheckListener, err)
	} else if h.Addr, err = lis.Addr().String(), lis.Close(); err != nil {
		return errors.Join(ErrHTTPCloseListener, err)
	}

	ctx, cancel := context.WithCancelCause(top)
	defer cancel(context.Canceled)

	go func() {
		if err := h.serve(); err != nil {
			cancel(err)
		}
	}()

	<-ctx.Done()
	{ // shutdown http.Server
		out, done := context.WithTimeout(context.Background(), h.base.ShutdownTimeout)
		defer done()

		if err := h.Shutdown(out); err != nil {
			return errors.Join(ErrHTTPShutdownServer, err, context.Cause(ctx))
		}
	}

	return context.Cause(ctx)
}