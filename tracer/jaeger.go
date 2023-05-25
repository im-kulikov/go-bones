package tracer

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

// Jaeger config options.
type Jaeger struct {
	Sampler       float64       `env:"SAMPLER" default:"1" usage:"allows to choose sampler"`
	Endpoint      string        `env:"ENDPOINT" usage:"allows to set jaeger endpoint (one of)" example:"http://localhost:14268/api/traces"`
	AgentHost     string        `env:"AGENT_HOST" usage:"allows to set jaeger agent host (one of)" example:"localhost"`
	AgentPort     string        `env:"AGENT_PORT" usage:"allows to set jaeger agent port" example:"6831"`
	RetryInterval time.Duration `env:"AGENT_RETRY_INTERVAL" default:"15s" usage:"allows to set retry connection timeout"`
}

type jaegerExporterService struct {
	cfg Jaeger
	opt []Option
	log logger.Logger

	*trace.TracerProvider
}

// Name of the service.
func (j *jaegerExporterService) Name() string { return "jaeger-trace-exporter" }

// Start does nothing.
func (j *jaegerExporterService) Start(ctx context.Context) error {
	<-ctx.Done()

	return nil
}

// Flush immediately exports all spans that have not yet been exported for
// all the registered span processors.
func (j *jaegerExporterService) Flush(ctx context.Context) error {
	return j.TracerProvider.ForceFlush(ctx)
}

// Stop should gracefully teardown jaeger tracer2.TracerProvider.
func (j *jaegerExporterService) Stop(ctx context.Context) {
	if err := j.TracerProvider.Shutdown(ctx); err != nil {
		j.log.Errorw("could not shutdown export provider", "error", err)
	}
}

const errCantUploadTraceSpan = "could not upload spans to Jaeger"

// prepareJaeger prepares jaeger module.
// nolint:funlen
func prepareJaeger(log logger.Logger, cfg Jaeger, opts ...Option) (service.Service, error) {
	var (
		err error
		val customOptions
		opt jaeger.EndpointOption
	)

	val.customErrorHandleFunc = func(err error) {
		if err == nil {
			return
		}

		log.Errorw(errCantUploadTraceSpan, "error", err)
	}

	for _, o := range opts {
		o(&val)
	}

	switch {
	case cfg.Endpoint != "":
		opt = jaeger.WithCollectorEndpoint(
			jaeger.WithEndpoint(cfg.Endpoint))
	case cfg.AgentHost != "":
		agentReconnectOption := jaeger.WithAttemptReconnectingInterval(cfg.RetryInterval)
		if cfg.RetryInterval <= 0 {
			agentReconnectOption = jaeger.WithDisableAttemptReconnecting()
		}

		opt = jaeger.WithAgentEndpoint(
			agentReconnectOption,
			jaeger.WithLogger(log.Std()),
			jaeger.WithAgentHost(cfg.AgentHost),
			jaeger.WithAgentPort(cfg.AgentPort))
	default:
		return nil, errUnknownType
	}

	var exporter *jaeger.Exporter
	if exporter, err = jaeger.New(opt); err != nil {
		return nil, err
	}

	provider := trace.NewTracerProvider(
		// 1. Drop will not record the span and all attributes/events will be dropped.
		// 2. Record indicates the span's `IsRecording() == true`, but `Sampled` flag
		// 3. RecordAndSample has span's `IsRecording() == true` and `Sampled` flag
		trace.WithSampler(trace.TraceIDRatioBased(cfg.Sampler)),
		// Always be sure to batch in production.
		trace.WithBatcher(exporter),
		// Record information about this application in a Resource.
		trace.WithResource(resource.NewWithAttributes(semconv.SchemaURL, val.jaegerResourceOptions...)))

	otel.SetTracerProvider(provider)
	otel.SetErrorHandler(val.customErrorHandleFunc)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // propagator that supports the W3C Trace Context format
		propagation.Baggage{},      // propagator that supports the W3C Baggage format
		JaegerPropagator{}))        // propagator serializes SpanContext to/from Jaeger Header

	return &jaegerExporterService{
		cfg:            cfg,
		log:            log,
		opt:            opts,
		TracerProvider: provider,
	}, nil
}
