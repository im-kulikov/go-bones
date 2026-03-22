package tracer

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

const (
	defaultServiceName = "go-bones"
	serviceNameKey     = "service.name"
	serviceVersionKey  = "service.version"
	tracerServiceName  = "tracer"

	envOTELSDKDisabled = "OTEL_SDK_DISABLED"

	envOTELServiceName   = "OTEL_SERVICE_NAME"
	envOTELResourceAttrs = "OTEL_RESOURCE_ATTRIBUTES"
	envOTELPropagators   = "OTEL_PROPAGATORS"

	envOTELExporterOTLPEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
	envOTELExporterOTLPInsecure = "OTEL_EXPORTER_OTLP_INSECURE"
	envOTELExporterOTLPProtocol = "OTEL_EXPORTER_OTLP_PROTOCOL"

	envOTELExporterOTLPTracesEndpoint = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	envOTELExporterOTLPTracesInsecure = "OTEL_EXPORTER_OTLP_TRACES_INSECURE"
	envOTELExporterOTLPTracesProtocol = "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"

	envOTELExporterOTLPMetricsEndpoint = "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"
	envOTELExporterOTLPMetricsInsecure = "OTEL_EXPORTER_OTLP_METRICS_INSECURE"
	envOTELExporterOTLPMetricsProtocol = "OTEL_EXPORTER_OTLP_METRICS_PROTOCOL"

	envOTELExporterOTLPLogsEndpoint = "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"
	envOTELExporterOTLPLogsInsecure = "OTEL_EXPORTER_OTLP_LOGS_INSECURE"
	envOTELExporterOTLPLogsProtocol = "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL"

	protocolGRPC         = "grpc"
	protocolHTTPProtobuf = "http/protobuf"
)

type hooks []func(context.Context) error

type traceProvider interface {
	trace.TracerProvider
	Shutdown(context.Context) error
}

type meterProvider interface {
	metric.MeterProvider
	Shutdown(context.Context) error
}

type loggerProvider interface {
	otellog.LoggerProvider
	Shutdown(context.Context) error
}

//nolint:gochecknoglobals // test seams for exporter/provider construction in tracer tests
var (
	newResourceFunc       = newResource
	newTraceProviderFunc  = newTraceProvider
	newMeterProviderFunc  = newMeterProvider
	newLoggerProviderFunc = newLoggerProvider
	newTraceExporterFunc  = newTraceExporter
	newMetricExporterFunc = newMetricExporter
	newLogExporterFunc    = newLogExporter
)

type bootstrap struct {
	cfg config.TracerConfig
	log *logger.Logger

	start atomic.Bool
	list  atomic.Pointer[hooks]
}

func (h *hooks) run(ctx context.Context) error {
	if h == nil {
		return nil
	}

	var err error
	for _, hook := range *h {
		err = errors.Join(err, hook(ctx))
	}

	return err
}

// Init creates a lifecycle service that bootstraps process-wide OpenTelemetry state.
//
// The package follows an env-first approach:
// standard OTEL_* environment variables are treated as the primary configuration
// contract, while config.TracerConfig acts as a local fallback for repository
// defaults and explicit enablement.
func Init(log *logger.Logger, cfg config.TracerConfig) service.Service {
	if log == nil {
		log = logger.Default()
	}

	state := &bootstrap{
		cfg: cfg,
		log: logger.Named(log, "go-bones", "tracer"),
	}

	return service.NewLauncher(tracerServiceName, state.run,
		service.WithLauncherLogger(state.log),
		service.WithLauncherShutdownHooks(state.stop))
}

func (b *bootstrap) run(ctx context.Context) error {
	if !b.enabled() {
		return nil
	}

	if !b.start.Swap(true) {
		out, err := bootstrapProviders(ctx, b.cfg)
		if err != nil {
			return err
		}

		b.list.Store(&out)
		b.log.InfoContext(ctx, "tracing initialized")
	}

	<-ctx.Done()

	return context.Cause(ctx)
}

func (b *bootstrap) stop(ctx context.Context) {
	if err := b.list.Load().run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		b.log.ErrorContext(ctx, "tracing shutdown failed", logger.Err(err))
	}
}

func (b *bootstrap) enabled() bool {
	if disabledByEnv() {
		return false
	}

	return b.cfg.Enabled ||
		b.cfg.SendMetrics ||
		b.cfg.SendLogs ||
		hasStandardBootstrapConfiguration()
}

func bootstrapProviders(ctx context.Context, cfg config.TracerConfig) (hooks, error) {
	res, err := newResourceFunc(ctx, cfg)
	if err != nil {
		return nil, err
	}

	logger.SetOpenTelemetryBridge(false)
	otel.SetTextMapPropagator(newPropagator())

	out := make(hooks, 0, 3)

	traceProvider, err := newTraceProviderFunc(ctx, res, cfg)
	if err != nil {
		return nil, err
	}
	otel.SetTracerProvider(traceProvider)
	out = append(out, traceProvider.Shutdown)

	if cfg.SendMetrics {
		meterProvider, errMeter := newMeterProviderFunc(ctx, res, cfg)
		if errMeter != nil {
			return nil, errMeter
		}

		// OTEL metrics export is opt-in and intentionally independent of the
		// OPS Prometheus endpoint, so applications can decide which telemetry path
		// to expose and operate.
		otel.SetMeterProvider(meterProvider)
		out = append(out, meterProvider.Shutdown)
	}

	if cfg.SendLogs {
		loggerProvider, errLogger := newLoggerProviderFunc(ctx, res, cfg)
		if errLogger != nil {
			return nil, errLogger
		}

		logglobal.SetLoggerProvider(loggerProvider)
		logger.SetOpenTelemetryBridge(true)
		out = append(out, loggerProvider.Shutdown)
		out = append(out, func(context.Context) error {
			logger.SetOpenTelemetryBridge(false)
			return nil
		})
	}

	return out, nil
}

func newTraceProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg config.TracerConfig,
) (traceProvider, error) {
	exporter, err := newTraceExporterFunc(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
	), nil
}

func newMeterProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg config.TracerConfig,
) (meterProvider, error) {
	exporter, err := newMetricExporterFunc(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
	), nil
}

func newLoggerProvider(
	ctx context.Context,
	res *resource.Resource,
	cfg config.TracerConfig,
) (loggerProvider, error) {
	exporter, err := newLogExporterFunc(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	), nil
}

func newTraceExporter(ctx context.Context, cfg config.TracerConfig) (sdktrace.SpanExporter, error) {
	if useHTTPForSignal("traces", cfg) {
		opts := make([]otlptracehttp.Option, 0, 2)
		if shouldApplyEndpointFallback("traces") && cfg.Endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
		}
		if shouldApplyInsecureFallback("traces") && cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}

		return otlptracehttp.New(ctx, opts...)
	}

	opts := make([]otlptracegrpc.Option, 0, 2)
	if shouldApplyEndpointFallback("traces") && cfg.Endpoint != "" {
		opts = append(opts, otlptracegrpc.WithEndpoint(cfg.Endpoint))
	}
	if shouldApplyInsecureFallback("traces") && cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	return otlptracegrpc.New(ctx, opts...)
}

func newMetricExporter(ctx context.Context, cfg config.TracerConfig) (sdkmetric.Exporter, error) {
	if useHTTPForSignal("metrics", cfg) {
		opts := make([]otlpmetrichttp.Option, 0, 2)
		if shouldApplyEndpointFallback("metrics") && cfg.Endpoint != "" {
			opts = append(opts, otlpmetrichttp.WithEndpoint(cfg.Endpoint))
		}
		if shouldApplyInsecureFallback("metrics") && cfg.Insecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}

		return otlpmetrichttp.New(ctx, opts...)
	}

	opts := make([]otlpmetricgrpc.Option, 0, 2)
	if shouldApplyEndpointFallback("metrics") && cfg.Endpoint != "" {
		opts = append(opts, otlpmetricgrpc.WithEndpoint(cfg.Endpoint))
	}
	if shouldApplyInsecureFallback("metrics") && cfg.Insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	return otlpmetricgrpc.New(ctx, opts...)
}

func newLogExporter(ctx context.Context, cfg config.TracerConfig) (sdklog.Exporter, error) {
	if useHTTPForSignal("logs", cfg) {
		opts := make([]otlploghttp.Option, 0, 2)
		if shouldApplyEndpointFallback("logs") && cfg.Endpoint != "" {
			opts = append(opts, otlploghttp.WithEndpoint(cfg.Endpoint))
		}
		if shouldApplyInsecureFallback("logs") && cfg.Insecure {
			opts = append(opts, otlploghttp.WithInsecure())
		}

		return otlploghttp.New(ctx, opts...)
	}

	opts := make([]otlploggrpc.Option, 0, 2)
	if shouldApplyEndpointFallback("logs") && cfg.Endpoint != "" {
		opts = append(opts, otlploggrpc.WithEndpoint(cfg.Endpoint))
	}
	if shouldApplyInsecureFallback("logs") && cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}

	return otlploggrpc.New(ctx, opts...)
}

func newResource(ctx context.Context, cfg config.TracerConfig) (*resource.Resource, error) {
	opts := []resource.Option{
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	}

	attrs := fallbackResourceAttributes(cfg)
	if len(attrs) > 0 {
		opts = append(opts, resource.WithAttributes(attrs...))
	}

	return resource.New(ctx, opts...)
}

func fallbackResourceAttributes(cfg config.TracerConfig) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 2)
	keys := envResourceAttributeKeys()

	if _, ok := os.LookupEnv(envOTELServiceName); !ok && !keys[serviceNameKey] {
		name := cfg.AppName()
		if name == "" {
			name = defaultServiceName
		}

		attrs = append(attrs, semconv.ServiceName(name))
	}

	if cfg.AppVersion() != "" && !keys[serviceVersionKey] {
		attrs = append(attrs, semconv.ServiceVersion(cfg.AppVersion()))
	}

	return attrs
}

func envResourceAttributeKeys() map[string]bool {
	raw := os.Getenv(envOTELResourceAttrs)
	if raw == "" {
		return nil
	}

	out := make(map[string]bool)
	for _, pair := range strings.Split(raw, ",") {
		key, _, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && key != "" {
			out[key] = true
		}
	}

	return out
}

func newPropagator() propagation.TextMapPropagator {
	raw := os.Getenv(envOTELPropagators)
	if raw == "" {
		return propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		)
	}

	list := make([]propagation.TextMapPropagator, 0, 2)
	seen := make(map[string]bool)

	for _, item := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "" || seen[name] {
			continue
		}

		seen[name] = true

		switch name {
		case "tracecontext":
			list = append(list, propagation.TraceContext{})
		case "baggage":
			list = append(list, propagation.Baggage{})
		case "none":
			return propagation.NewCompositeTextMapPropagator()
		}
	}

	if len(list) == 0 {
		return propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		)
	}

	return propagation.NewCompositeTextMapPropagator(list...)
}

func disabledByEnv() bool {
	value, ok := os.LookupEnv(envOTELSDKDisabled)
	return ok && strings.EqualFold(value, "true")
}

func hasStandardBootstrapConfiguration() bool {
	for _, key := range [...]string{
		envOTELServiceName,
		envOTELResourceAttrs,
		envOTELExporterOTLPEndpoint,
		envOTELExporterOTLPTracesEndpoint,
		envOTELExporterOTLPMetricsEndpoint,
		envOTELExporterOTLPLogsEndpoint,
	} {
		if _, ok := os.LookupEnv(key); ok {
			return true
		}
	}

	return false
}

func useHTTPForSignal(signal string, cfg config.TracerConfig) bool {
	value, ok := lookupProtocolEnv(signal)
	if ok {
		return strings.EqualFold(value, protocolHTTPProtobuf)
	}

	return cfg.UseHTTP
}

func shouldApplyEndpointFallback(signal string) bool {
	_, exists := lookupEndpointEnv(signal)
	return !exists
}

func shouldApplyInsecureFallback(signal string) bool {
	_, exists := lookupInsecureEnv(signal)
	return !exists
}

func lookupEndpointEnv(signal string) (string, bool) {
	switch signal {
	case "traces":
		if value, ok := os.LookupEnv(envOTELExporterOTLPTracesEndpoint); ok {
			return value, true
		}
	case "metrics":
		if value, ok := os.LookupEnv(envOTELExporterOTLPMetricsEndpoint); ok {
			return value, true
		}
	case "logs":
		if value, ok := os.LookupEnv(envOTELExporterOTLPLogsEndpoint); ok {
			return value, true
		}
	}

	return os.LookupEnv(envOTELExporterOTLPEndpoint)
}

func lookupInsecureEnv(signal string) (string, bool) {
	switch signal {
	case "traces":
		if value, ok := os.LookupEnv(envOTELExporterOTLPTracesInsecure); ok {
			return value, true
		}
	case "metrics":
		if value, ok := os.LookupEnv(envOTELExporterOTLPMetricsInsecure); ok {
			return value, true
		}
	case "logs":
		if value, ok := os.LookupEnv(envOTELExporterOTLPLogsInsecure); ok {
			return value, true
		}
	}

	return os.LookupEnv(envOTELExporterOTLPInsecure)
}

func lookupProtocolEnv(signal string) (string, bool) {
	switch signal {
	case "traces":
		if value, ok := os.LookupEnv(envOTELExporterOTLPTracesProtocol); ok {
			return value, true
		}
	case "metrics":
		if value, ok := os.LookupEnv(envOTELExporterOTLPMetricsProtocol); ok {
			return value, true
		}
	case "logs":
		if value, ok := os.LookupEnv(envOTELExporterOTLPLogsProtocol); ok {
			return value, true
		}
	}

	return os.LookupEnv(envOTELExporterOTLPProtocol)
}
