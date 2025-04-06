package tracer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	_ "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Option represents a functional option for configuring tracing settings.
type Option func(*settings)

// ExporterType defines the type of tracing exporter.
type ExporterType string

const (
	ExporterJaeger ExporterType = "jaeger"
	ExporterOTLP   ExporterType = "otlp"
)

// settings holds the tracing configuration parameters.
type settings struct {
	enabled        bool
	exporterType   ExporterType
	pluginSettings map[string]interface{}

	jaegerSettings
}

// WithEnabled enables or disables tracing.
func WithEnabled(enabled bool) Option {
	return func(s *settings) {
		s.enabled = enabled
	}
}

// WithExporterType sets the type of tracing exporter.
func WithExporterType(exporter ExporterType) Option {
	return func(s *settings) {
		s.exporterType = exporter
	}
}

// WithPluginSettings applies additional plugin-specific settings.
func WithPluginSettings(pluginSettings map[string]interface{}) Option {
	return func(s *settings) {
		s.pluginSettings = pluginSettings
	}
}

// NewExporter creates a tracing exporter based on the settings.
//
// Parameters:
//   - cfg: The settings for the exporter.
//
// Returns:
//   - trace.SpanExporter: The configured exporter.
//   - error: An error if initialization fails.
func NewExporter(cfg *settings) (trace.SpanExporter, error) {
	switch cfg.exporterType {
	case ExporterJaeger:
		return nil, errors.New("jaeger exporter is not supported")
	case ExporterOTLP:
		return newOTLPExporter(cfg)
	default:
		return nil, fmt.Errorf("unsupported exporter type: %s", cfg.exporterType)
	}
}

// newOTLPExporter creates an OTLP exporter (gRPC).
func newOTLPExporter(cfg *settings) (trace.SpanExporter, error) {
	endpoint, _ := cfg.pluginSettings["endpoint"].(string)
	if endpoint == "" {
		return nil, errors.New("otlp exporter: endpoint is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := otlptracegrpc.NewClient(
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure())

	return otlptrace.New(ctx, client)
}

// NewTracerProvider initializes an OpenTelemetry TracerProvider.
//
// It uses the provided exporter for sending traces.
//
// Parameters:
//   - opts: Functional options to configure the tracer.
//
// Returns:
//   - trace.TracerProvider: The configured tracer provider.
//   - func(): A function to shutdown the provider when it's no longer needed.
//   - error: An error if initialization fails.
func NewTracerProvider(opts ...Option) (*trace.TracerProvider, func(), error) {
	cfg := &settings{
		pluginSettings: make(map[string]interface{}),
		exporterType:   ExporterJaeger, // Default exporter is Jaeger
	}

	for _, opt := range opts {
		opt(cfg)
	}

	if !cfg.enabled {
		return nil, func() {}, nil
	}

	serviceName, _ := cfg.pluginSettings["serviceName"].(string)
	sampler, _ := cfg.pluginSettings["sampler"].(float64)
	if serviceName == "" {
		return nil, nil, errors.New("opentelemetry: serviceName is required")
	}

	exp, err := NewExporter(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize exporter: %w", err)
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		)),
		trace.WithSampler(trace.TraceIDRatioBased(sampler)),
	)

	otel.SetTracerProvider(tp)

	shutdown := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}

	return tp, shutdown, nil
}
