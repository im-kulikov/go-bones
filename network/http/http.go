package http

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/network"
	"github.com/im-kulikov/go-bones/service"
)

type serverOptions struct {
	name string
	open network.ListenOpener
	base config.BaseHTTP
	otel bool

	*Server
	*logger.Logger
}

type (

	// Option defines a function type for configuring HTTP server options.
	// It accepts and modifies the serverOptions struct.
	Option func(*serverOptions)

	// ServerOption defines a function type for configuring HTTP server instances.
	// It allows direct modification of the underlying stdlib server.
	ServerOption func(*Server)
)

const (
	defaultHTTPNetwork     = "tcp"
	defaultHTTPServiceName = "http-service"

	httpServerStarting  = "http server starting"
	httpsServerStarting = "https server starting"

	// ErrHTTPCheckListener indicates a failure during the initialization of the HTTP listener.
	ErrHTTPCheckListener bones.Error = "http check listener"

	// ErrHTTPShutdownServer indicates a failure during the shutdown process of the HTTP server.
	ErrHTTPShutdownServer bones.Error = "http shutdown server"
)

const defaultTimeout = time.Second * 15

// ServiceName overrides the lifecycle name used for the HTTP service.
func ServiceName(name string) Option {
	return func(settings *serverOptions) { settings.name = name }
}

// ServerOptions applies raw http.Server mutators to the constructed server.
func ServerOptions(opts ...ServerOption) Option {
	return func(s *serverOptions) {
		for _, opt := range opts {
			opt(s.Server)
		}
	}
}

// Options groups multiple HTTP Option values into one reusable Option.
func Options(opts ...Option) Option {
	return func(settings *serverOptions) {
		for _, opt := range opts {
			opt(settings)
		}
	}
}

// WithOpenTelemetry wraps the configured HTTP handler with repository-local
// OpenTelemetry server instrumentation.
func WithOpenTelemetry() Option {
	return func(settings *serverOptions) { settings.otel = true }
}

// NewServer builds a service.Service that owns an http.Server lifecycle.
func NewServer(
	cfg config.HTTPConfig,
	log *logger.Logger,
	opts ...Option,
) (service.Service, error) {
	options, err := prepareServer(cfg, log, opts...)
	if err != nil {
		return nil, err
	}

	return service.NewLauncher(options.name, options.listen,
		service.WithLauncherLogger(log),
		service.WithLauncherShutdownHooks(func(ctx context.Context) {
			options.InfoContext(ctx, "shutdown gracefully done",
				logger.String("service", options.name),
				logger.String("address", options.Addr))
		})), nil
}

// The newServer initializes and returns an http.Server configured with the provided HTTPConfig.
// It prepares TLS configuration if enabled and returns an error on failure excluding a disabled TLS scenario.
func newServer(c config.HTTPConfig) (*Server, error) {
	var err error
	base := c.Base()

	var cfg *tls.Config
	if cfg, err = base.PrepareTLSConfig(); err != nil && !errors.Is(err, config.ErrTLSDisabled) {
		return nil, err
	}

	return &Server{
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
	if options.otel {
		options.Server.Handler = newOpenTelemetryHandler(options.name, options.Server.Handler)
	}

	return options, nil
}

func newOpenTelemetryHandler(serviceName string, next Handler) Handler {
	if next == nil {
		next = DefaultServeMux
	}

	tracer := otel.Tracer("github.com/im-kulikov/go-bones/network/http")

	return HandlerFunc(func(w ResponseWriter, r *Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagationHeaderCarrier(r.Header))
		spanName := r.Method + " " + r.URL.Path
		if spanName == " " {
			spanName = serviceName
		}

		ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type propagationHeaderCarrier Header

// Get retrieves the value associated with the given key from the header.
func (c propagationHeaderCarrier) Get(key string) string {
	return Header(c).Get(key)
}

// Set sets the header entries associated with key to the single element value.
func (c propagationHeaderCarrier) Set(key, value string) {
	Header(c).Set(key, value)
}

// Keys returns all header keys in the carrier.
func (c propagationHeaderCarrier) Keys() []string {
	header := Header(c)
	keys := make([]string, 0, len(header))
	for key := range header {
		keys = append(keys, key)
	}

	return keys
}

// serve starts the HTTP server using the provided listener.
func (h *serverOptions) serve(lis net.Listener) error {
	if h.base.TLSConfig == nil {
		h.Info(httpServerStarting,
			logger.String("service", h.name),
			logger.String("address", lis.Addr().String()))

		return h.Serve(lis)
	}

	h.Info(httpsServerStarting,
		logger.String("service", h.name),
		logger.String("address", lis.Addr().String()))

	return h.Serve(tls.NewListener(lis, h.TLSConfig))
}

// The listen handles the initialization and operation of the HTTP server, including listening, serving, and shutdown.
// Returns an error if the listener setup, server operation, or shutdown process encounters an issue.
func (h *serverOptions) listen(top context.Context) error {
	lis, err := h.open.Listen(top, defaultHTTPNetwork, h.Addr)
	if err != nil {
		return errors.Join(ErrHTTPCheckListener, err)
	}

	h.Addr = lis.Addr().String()

	ctx, cancel := context.WithCancelCause(top)
	defer cancel(context.Canceled)

	var wg sync.WaitGroup
	wg.Add(1)
	context.AfterFunc(ctx, func() { // shutdown http.Server
		defer wg.Done()

		h.InfoContext(ctx, "try to graceful shutdown",
			logger.String("name", h.name))

		timeout := h.base.ShutdownTimeout
		if timeout <= 0 {
			timeout = defaultTimeout
		}

		out, done := context.WithTimeout(context.Background(), timeout)
		defer done()

		if errStop := h.Shutdown(out); errStop != nil {
			h.ErrorContext(ctx, "something went wrong",
				logger.String("name", h.name),
				logger.Err(errors.Join(ErrHTTPShutdownServer, errStop, context.Cause(ctx))))
		}
	})

	defer wg.Wait()

	if err = h.serve(lis); err != nil && !errors.Is(err, ErrServerClosed) {
		cancel(err)
		return err
	}

	return nil
}
