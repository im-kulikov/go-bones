package grpc

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/network"
	"github.com/im-kulikov/go-bones/service"
)

// serverOptions contains both construction-time configuration and runtime state for
// the gRPC transport service built by NewServer.
//
// The split between prepareServer, listen, and shutdown is intentional:
// preparation resolves config and builds grpc.Server once, listen owns socket
// startup, and shutdown serializes graceful termination from all exit paths.
type serverOptions struct {
	name string
	addr string
	mu   sync.RWMutex
	open network.ListenOpener
	base config.BaseGRPC

	grpc *Server
	log  *logger.Logger
	once sync.Once
	opts []ServerOption
	init []func(*Server)
}

type (
	// Option configures serverOptions before grpc.Server is created.
	Option func(*serverOptions)
)

const (
	defaultGRPCNetwork     = "tcp"
	defaultGRPCServiceName = "grpc-service"
	defaultTimeout         = 15 * time.Second

	grpcServerStarting = "grpc server starting"

	// ErrGRPCCheckListener indicates a failure during the initialization of the gRPC listener.
	ErrGRPCCheckListener bones.Error = "grpc check listener"
)

// ServiceName sets the lifecycle/logging name used by the wrapped service launcher.
func ServiceName(name string) Option {
	return func(settings *serverOptions) { settings.name = name }
}

// ServerOptions appends raw grpc.ServerOption values to grpc.NewServer.
func ServerOptions(opts ...ServerOption) Option {
	return func(s *serverOptions) { s.opts = append(s.opts, opts...) }
}

// RegisterServices registers callbacks that receive the constructed gRPC server.
// This is where callers attach health, reflection, and business services.
func RegisterServices(list ...func(*Server)) Option {
	return func(s *serverOptions) { s.init = append(s.init, list...) }
}

// Options groups multiple Option values into one reusable Option.
func Options(opts ...Option) Option {
	return func(settings *serverOptions) {
		for _, opt := range opts {
			opt(settings)
		}
	}
}

// WithOpenTelemetry attaches repository-local OpenTelemetry server interceptors
// for unary and stream RPCs.
func WithOpenTelemetry() Option {
	return func(s *serverOptions) {
		s.opts = append(s.opts,
			chainUnaryInterceptor(openTelemetryUnaryServerInterceptor()),
			chainStreamInterceptor(openTelemetryStreamServerInterceptor()),
		)
	}
}

// NewServer builds a Service that owns a grpc.Server lifecycle.
//
// The returned service is backed by service.NewLauncher:
//   - Start opens the listener and blocks in grpc.Server.Serve.
//   - Stop triggers graceful shutdown through the launcher's shutdown hook.
//
// Shutdown is also wired from the parent context via listen, and both paths are
// serialized through serverOptions.once. This keeps parent-cancel and explicit Stop
// behavior aligned without letting grpc.Server shutdown run twice.
func NewServer(
	cfg config.GRPCConfig,
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
			options.shutdown(ctx)

			options.log.InfoContext(ctx, "shutdown gracefully done",
				logger.String("service", options.name),
				logger.String("address", options.address()))
		})), nil
}

func (h *serverOptions) address() string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.addr
}

// setAddress updates the bound address after a listener is created.
// This matters when callers pass :0 and need the actual port for logging/tests.
func (h *serverOptions) setAddress(addr string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.addr = addr
}

// prepareServer resolves config and creates the concrete grpc.Server instance.
// It performs all non-blocking setup up front so Start can focus on runtime work.
func prepareServer(
	cfg config.GRPCConfig,
	log *logger.Logger,
	opts ...Option,
) (*serverOptions, error) {
	base := cfg.Base()
	options := &serverOptions{
		addr: cfg.Addr(),
		base: base,
		open: new(net.ListenConfig),
		name: defaultGRPCServiceName,
		log:  logger.Named(log, "go-bones", "grpc-server"),
	}

	tlsCfg, err := base.PrepareTLSConfig()
	if err != nil && !errors.Is(err, config.ErrTLSDisabled) {
		return nil, err
	}

	if tlsCfg != nil {
		options.opts = append(options.opts, withTransportCredentials(credentials.NewTLS(tlsCfg)))
	}

	for _, opt := range opts {
		opt(options)
	}

	options.grpc = newServer(options.opts...)
	for _, init := range options.init {
		if init != nil {
			init(options.grpc)
		}
	}

	return options, nil
}

// The listen owns the runtime of grpc.Server.Serve.
//
// It opens the listener once, records the final bound address, registers shutdown
// on parent-context cancellation, and then blocks in Serve until the server exits.
func (h *serverOptions) listen(top context.Context) error {
	lis, err := h.open.Listen(top, defaultGRPCNetwork, h.address())
	if err != nil {
		return errors.Join(ErrGRPCCheckListener, err)
	}

	h.setAddress(lis.Addr().String())

	h.log.Info(grpcServerStarting,
		logger.String("service", h.name),
		logger.String("address", h.address()))

	context.AfterFunc(top, func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(top), h.shutdownTimeout())
		defer cancel()

		h.shutdown(ctx)
	})

	if err = h.grpc.Serve(lis); err != nil && !errors.Is(err, ErrServerStopped) {
		return err
	}

	return nil
}

// shutdownTimeout returns the configured graceful shutdown timeout with a safe fallback.
func (h *serverOptions) shutdownTimeout() time.Duration {
	timeout := h.base.ShutdownTimeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	return timeout
}

// The shutdown gracefully stops the `grpc.Server` once.
//
// GracefulStop may block while in-flight RPCs finish, so a timer escalates to Stop
// after shutdownTimeout. The sync.Once guard is important because shutdown can be
// requested from both explicit Stop and parent-context cancellation.
func (h *serverOptions) shutdown(ctx context.Context) {
	h.once.Do(func() {
		stopDone := make(chan struct{})
		timeout := h.shutdownTimeout()

		h.log.InfoContext(ctx, "try to graceful shutdown", logger.String("name", h.name))

		defer time.AfterFunc(timeout, func() {
			h.forceStop(stopDone)
		}).Stop()

		h.grpc.GracefulStop()
		close(stopDone)
	})
}

// forceStop escalates graceful shutdown to grpc.Server.Stop unless shutdown already finished.
func (h *serverOptions) forceStop(stopDone <-chan struct{}) {
	select {
	case <-stopDone:
	default:
		h.grpc.Stop()
	}
}

func openTelemetryUnaryServerInterceptor() UnaryServerInterceptor {
	tracer := otel.Tracer("github.com/im-kulikov/go-bones/network/grpc")

	return func(
		ctx context.Context,
		req any,
		info *UnaryServerInfo,
		handler UnaryHandler,
	) (any, error) {
		ctx = extractIncomingContext(ctx)
		ctx, span := tracer.Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		return handler(ctx, req)
	}
}

func openTelemetryStreamServerInterceptor() StreamServerInterceptor {
	tracer := otel.Tracer("github.com/im-kulikov/go-bones/network/grpc")

	return func(
		srv any,
		ss ServerStream,
		info *StreamServerInfo,
		handler StreamHandler,
	) error {
		ctx := extractIncomingContext(ss.Context())
		ctx, span := tracer.Start(ctx, info.FullMethod, trace.WithSpanKind(trace.SpanKindServer))
		defer span.End()

		return handler(srv, &serverStreamWithContext{ServerStream: ss, ctx: ctx})
	}
}

func extractIncomingContext(ctx context.Context) context.Context {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}

	return otel.GetTextMapPropagator().Extract(ctx, metadataCarrier(md))
}

type serverStreamWithContext struct {
	ServerStream
	ctx context.Context
}

// Context returns the context associated with the server stream.
func (s *serverStreamWithContext) Context() context.Context { return s.ctx }

type metadataCarrier metadata.MD

// Get retrieves the first value associated with the given key from the metadata.
func (c metadataCarrier) Get(key string) string {
	values := metadata.MD(c).Get(key)
	if len(values) == 0 {
		return ""
	}

	return strings.Join(values, ",")
}

// Set sets the metadata entries associated with key to the single element value.
func (c metadataCarrier) Set(key, value string) {
	metadata.MD(c).Set(key, value)
}

// Keys returns all metadata keys in the carrier.
func (c metadataCarrier) Keys() []string {
	md := metadata.MD(c)
	keys := make([]string, 0, len(md))
	for key := range md {
		keys = append(keys, key)
	}

	return keys
}
