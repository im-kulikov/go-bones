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
	// HTTPOption for httpService.
	HTTPOption func(*httpOptions)

	// HTTPServerOption for http.Server.
	HTTPServerOption func(*http.Server)
)

const (
	defaultHTTPNetwork     = "tcp"
	defaultHTTPServiceName = "http-service"

	httpServerStarting  = "http server starting"
	httpsServerStarting = "https server starting"

	// ErrHTTPCheckListener fires when could not use provided address.
	ErrHTTPCheckListener bones.Error = "http check listener"

	// ErrHTTPCloseListener fires when could not close test listener.
	ErrHTTPCloseListener bones.Error = "http close listener"

	// ErrHTTPShutdownServer fires when could not close http.Server.
	ErrHTTPShutdownServer bones.Error = "http shutdown server"
)

// HTTPServiceName allows to set httpService name.
func HTTPServiceName(name string) HTTPOption {
	return func(settings *httpOptions) { settings.name = name }
}

// HTTPServerOptions allows to set HTTPServerOptions to http.Server.
func HTTPServerOptions(opts ...HTTPServerOption) HTTPOption {
	return func(s *httpOptions) {
		for _, opt := range opts {
			opt(s.Server)
		}
	}
}

// HTTPOptions allows to set multiple HTTPOption's at once.
func HTTPOptions(opts []HTTPOption) HTTPOption {
	return func(settings *httpOptions) {
		for _, opt := range opts {
			opt(settings)
		}
	}
}

func NewHTTPServer(
	cfg config.HTTPConfig,
	log *logger.Logger,
	handler http.Handler,
	opts ...HTTPOption,
) (service.Service, error) {
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
