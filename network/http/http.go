package http

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

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

type serverOptions struct {
	name string
	open ListenOpener
	base config.BaseHTTP

	*http.Server
	*logger.Logger
}

type (

	// Option defines a function type for configuring HTTP server options.
	// It accepts and modifies the serverOptions struct.
	Option func(*serverOptions)

	// ServerOption defines a function type for configuring http.Server instances.
	// It allows direct modification of the standard http.Server settings.
	ServerOption func(*http.Server)
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

// ServiceName sets a custom name for the HTTP service.
// This name is used for logging and identification.
func ServiceName(name string) Option {
	return func(settings *serverOptions) { settings.name = name }
}

// ServerOptions applies a collection of server-specific configurations.
// It allows chaining multiple server options for the internal http.Server instance.
func ServerOptions(opts ...ServerOption) Option {
	return func(s *serverOptions) {
		for _, opt := range opts {
			opt(s.Server)
		}
	}
}

// Options combines multiple HTTP options into a single configuration function.
// It sequentially applies each option in the provided slice.
func Options(opts ...Option) Option {
	return func(settings *serverOptions) {
		for _, opt := range opts {
			opt(settings)
		}
	}
}

// NewServer creates a new HTTP service with the specified configuration.
// It sets up the server with the provided logger and request handler
// and allows additional customization through options.
func NewServer(
	cfg config.HTTPConfig,
	log *logger.Logger,
	opts ...Option,
) (service.Service, error) {
	options, err := prepareServer(cfg, log, opts...)
	if err != nil {
		return nil, err
	}

	return service.NewLauncher(options.name, options.listen, func(ctx context.Context) {
		options.InfoContext(ctx, "shutdown gracefully done",
			logger.String("service", options.name),
			logger.String("address", options.Addr))
	}), nil
}

// The newServer initializes and returns an http.Server configured with the provided HTTPConfig.
// It prepares TLS configuration if enabled and returns an error on failure excluding a disabled TLS scenario.
func newServer(c config.HTTPConfig) (*http.Server, error) {
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

// prepareServer configures and prepares an HTTP server with provided configuration, logger, handler, and options.
// It returns the configured serverOptions or an error on failure.
func prepareServer(
	cfg config.HTTPConfig,
	log *logger.Logger,
	opts ...Option,
) (*serverOptions, error) {
	srv, err := newServer(cfg)
	if err != nil {
		return nil, err
	}

	options := &serverOptions{
		Logger: log,
		Server: srv,

		base: cfg.Base(),
		open: new(net.ListenConfig),
		name: defaultHTTPServiceName,
	}

	for _, opt := range opts {
		opt(options)
	}

	// add prefixes:
	options.Logger = logger.Named(log, "go-bones", "http-server")

	return options, nil
}

// serve starts the HTTP server, selecting between plain HTTP or HTTPS based on the TLS configuration.
func (h *serverOptions) serve() error {
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
func (h *serverOptions) listen(top context.Context) error {
	if lis, err := h.open.Listen(top, defaultHTTPNetwork, h.Addr); err != nil {
		return errors.Join(ErrHTTPCheckListener, err)
	} else if h.Addr, err = lis.Addr().String(), lis.Close(); err != nil {
		return errors.Join(ErrHTTPCloseListener, err)
	}

	ctx, cancel := context.WithCancelCause(top)
	defer cancel(context.Canceled)

	var wg sync.WaitGroup

	wg.Add(1)
	context.AfterFunc(ctx, func() { // shutdown http.Server
		defer wg.Done()

		h.InfoContext(ctx, "try to graceful shutdown",
			logger.String("name", h.name))

		out, done := context.WithTimeout(context.Background(), time.Millisecond)
		defer done()

		if err := h.Shutdown(out); err != nil {
			h.ErrorContext(ctx, "something went wrong",
				logger.String("name", h.name),
				logger.Err(errors.Join(ErrHTTPShutdownServer, err, context.Cause(ctx))))
		}
	})

	defer wg.Wait()

	if err := h.serve(); err != nil {
		cancel(err)
	}

	return nil
}
