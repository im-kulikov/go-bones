package tracer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	otellognoop "go.opentelemetry.io/otel/log/noop"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	collogpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
)

func TestEnabled(t *testing.T) {
	log := logger.ForTests()

	t.Run("disabled by default", func(t *testing.T) {
		b := &bootstrap{cfg: config.TracerConfig{}, log: log}
		require.False(t, b.enabled())
	})

	t.Run("enabled by config fallback", func(t *testing.T) {
		b := &bootstrap{cfg: config.TracerConfig{Enabled: true}, log: log}
		require.True(t, b.enabled())
	})

	t.Run("metrics config enables bootstrap", func(t *testing.T) {
		b := &bootstrap{cfg: config.TracerConfig{SendMetrics: true}, log: log}
		require.True(t, b.enabled())
	})

	t.Run("logs config enables bootstrap", func(t *testing.T) {
		b := &bootstrap{cfg: config.TracerConfig{SendLogs: true}, log: log}
		require.True(t, b.enabled())
	})

	t.Run("enabled by standard env", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPEndpoint, "http://collector:4318")

		b := &bootstrap{cfg: config.TracerConfig{}, log: log}
		require.True(t, b.enabled())
	})

	t.Run("resource env enables bootstrap", func(t *testing.T) {
		t.Setenv(envOTELServiceName, "checkout-api")

		b := &bootstrap{cfg: config.TracerConfig{}, log: log}
		require.True(t, b.enabled())
	})

	t.Run("propagators alone do not enable bootstrap", func(t *testing.T) {
		t.Setenv(envOTELPropagators, "tracecontext,baggage")

		b := &bootstrap{cfg: config.TracerConfig{}, log: log}
		require.False(t, b.enabled())
	})

	t.Run("sdk disabled env wins", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPEndpoint, "http://collector:4318")
		t.Setenv(envOTELSDKDisabled, "true")

		b := &bootstrap{cfg: config.TracerConfig{Enabled: true}, log: log}
		require.False(t, b.enabled())
	})
}

func TestHooksRun(t *testing.T) {
	t.Run("nil hooks are no-op", func(t *testing.T) {
		var list *hooks
		require.NoError(t, list.run(t.Context()))
	})

	t.Run("joins hook errors", func(t *testing.T) {
		expectedA := fmt.Errorf("shutdown a")
		expectedB := fmt.Errorf("shutdown b")
		list := hooks{
			func(context.Context) error { return expectedA },
			func(context.Context) error { return expectedB },
		}

		err := list.run(t.Context())
		require.ErrorIs(t, err, expectedA)
		require.ErrorIs(t, err, expectedB)
	})
}

func TestUseHTTPForSignal(t *testing.T) {
	t.Run("config fallback selects http", func(t *testing.T) {
		require.True(t, useHTTPForSignal("traces", config.TracerConfig{UseHTTP: true}))
	})

	t.Run("env protocol overrides config fallback", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPTracesProtocol, protocolGRPC)
		require.False(t, useHTTPForSignal("traces", config.TracerConfig{UseHTTP: true}))
	})
}

func TestFallbackOptionsRespectOTELExporterEnv(t *testing.T) {
	t.Run("endpoint fallback disabled by signal-specific env", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPTracesEndpoint, "collector:4317")
		require.False(t, shouldApplyEndpointFallback("traces"))
	})

	t.Run("insecure fallback disabled by generic env", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPInsecure, "true")
		require.False(t, shouldApplyInsecureFallback("metrics"))
	})
}

func TestFallbackOptionsRespectOTELExporterEnvForAllSignals(t *testing.T) {
	t.Run("signal-specific endpoint env wins for all signals", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPTracesEndpoint, "trace-collector:4317")
		t.Setenv(envOTELExporterOTLPMetricsEndpoint, "metric-collector:4317")
		t.Setenv(envOTELExporterOTLPLogsEndpoint, "log-collector:4317")

		require.False(t, shouldApplyEndpointFallback("traces"))
		require.False(t, shouldApplyEndpointFallback("metrics"))
		require.False(t, shouldApplyEndpointFallback("logs"))
	})

	t.Run(
		"signal-specific protocol env wins over config fallback for all signals",
		func(t *testing.T) {
			t.Setenv(envOTELExporterOTLPTracesProtocol, protocolGRPC)
			t.Setenv(envOTELExporterOTLPMetricsProtocol, protocolGRPC)
			t.Setenv(envOTELExporterOTLPLogsProtocol, protocolGRPC)

			cfg := config.TracerConfig{UseHTTP: true}
			require.False(t, useHTTPForSignal("traces", cfg))
			require.False(t, useHTTPForSignal("metrics", cfg))
			require.False(t, useHTTPForSignal("logs", cfg))
		},
	)

	t.Run("generic insecure env disables config fallback for all signals", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPInsecure, "true")

		require.False(t, shouldApplyInsecureFallback("traces"))
		require.False(t, shouldApplyInsecureFallback("metrics"))
		require.False(t, shouldApplyInsecureFallback("logs"))
	})

	t.Run("signal-specific insecure env wins for all signals", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPTracesInsecure, "true")
		t.Setenv(envOTELExporterOTLPMetricsInsecure, "true")
		t.Setenv(envOTELExporterOTLPLogsInsecure, "true")

		require.False(t, shouldApplyInsecureFallback("traces"))
		require.False(t, shouldApplyInsecureFallback("metrics"))
		require.False(t, shouldApplyInsecureFallback("logs"))
	})
}

func TestInitReturnsLifecycleService(t *testing.T) {
	svc := Init(logger.ForTests(), config.TracerConfig{})

	require.NotNil(t, svc)
	require.Equal(t, tracerServiceName, svc.Name())
}

func TestInitWithNilLogger(t *testing.T) {
	svc := Init(nil, config.TracerConfig{})

	require.NotNil(t, svc)
	require.Equal(t, tracerServiceName, svc.Name())
}

func TestFallbackResourceAttributes(t *testing.T) {
	t.Run("uses config app metadata as fallback", func(t *testing.T) {
		var cfg config.TracerConfig
		cfg.SetAppNameAndVersion("billing-api", "1.2.3")

		attrs := fallbackResourceAttributes(cfg)

		require.Contains(t, attrs, semconv.ServiceName("billing-api"))
		require.Contains(t, attrs, semconv.ServiceVersion("1.2.3"))
	})

	t.Run("env values take precedence", func(t *testing.T) {
		t.Setenv(envOTELServiceName, "payments")
		t.Setenv(envOTELResourceAttrs, "service.version=9.9.9,deployment.environment=prod")

		var cfg config.TracerConfig
		cfg.SetAppNameAndVersion("billing-api", "1.2.3")

		attrs := fallbackResourceAttributes(cfg)

		require.NotContains(t, attrs, semconv.ServiceName("billing-api"))
		require.NotContains(t, attrs, semconv.ServiceVersion("1.2.3"))
	})
}

func TestNewPropagator(t *testing.T) {
	t.Run("default propagators include trace context and baggage", func(t *testing.T) {
		props := newPropagator()

		carrier := propagation.MapCarrier{}
		ctx := context.Background()
		ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{
			"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		})
		ctx = propagation.Baggage{}.Extract(ctx, propagation.MapCarrier{
			"baggage": "user_id=42",
		})

		props.Inject(ctx, carrier)

		require.Contains(t, carrier, "traceparent")
		require.Contains(t, carrier, "baggage")
	})

	t.Run("none disables propagators", func(t *testing.T) {
		t.Setenv(envOTELPropagators, "none")

		props := newPropagator()
		carrier := propagation.MapCarrier{}

		ctx := trace.ContextWithSpanContext(
			context.Background(),
			trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    [16]byte{1},
				SpanID:     [8]byte{2},
				TraceFlags: trace.FlagsSampled,
			}),
		)
		props.Inject(ctx, carrier)

		require.Empty(t, carrier)
	})

	t.Run(
		"unknown and duplicate values fall back to supported unique propagators",
		func(t *testing.T) {
			t.Setenv(envOTELPropagators, "tracecontext, baggage, tracecontext, unknown")

			props := newPropagator()
			carrier := propagation.MapCarrier{}

			ctx := trace.ContextWithSpanContext(
				context.Background(),
				trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    [16]byte{9},
					SpanID:     [8]byte{8},
					TraceFlags: trace.FlagsSampled,
				}),
			)
			props.Inject(ctx, carrier)

			require.Contains(t, carrier, "traceparent")
		},
	)

	t.Run("only unknown values fall back to defaults", func(t *testing.T) {
		t.Setenv(envOTELPropagators, "unknown")

		props := newPropagator()
		carrier := propagation.MapCarrier{}

		ctx := propagation.Baggage{}.Extract(context.Background(), propagation.MapCarrier{
			"baggage": "user_id=42",
		})
		props.Inject(ctx, carrier)

		require.Contains(t, carrier, "baggage")
	})
}

func TestNewResource(t *testing.T) {
	var cfg config.TracerConfig
	cfg.SetAppNameAndVersion("inventory-api", "2.0.0")

	res, err := newResource(t.Context(), cfg)
	require.NoError(t, err)

	require.Equal(t, "inventory-api", resourceValue(res, semconv.ServiceNameKey))
	require.Equal(t, "2.0.0", resourceValue(res, semconv.ServiceVersionKey))
}

func TestLifecycleStartStop(t *testing.T) {
	t.Setenv(envOTELServiceName, "checkout-api")

	svc := Init(logger.ForTests(), config.TracerConfig{})

	ctx, cancel := context.WithCancelCause(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- svc.Start(ctx)
	}()

	cancel(context.Canceled)
	require.ErrorIs(t, <-done, context.Canceled)

	svc.Stop(context.Background())
}

func TestBootstrapRunDisabledReturnsNil(t *testing.T) {
	state := &bootstrap{
		cfg: config.TracerConfig{},
		log: logger.ForTests(),
	}

	require.NoError(t, state.run(t.Context()))
}

func TestBootstrapRunReturnsBootstrapError(t *testing.T) {
	t.Cleanup(overrideProviderFactories())
	expected := fmt.Errorf("bootstrap failed")

	newTraceProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (traceProvider, error) {
		return nil, expected
	}

	state := &bootstrap{
		cfg: config.TracerConfig{Enabled: true},
		log: logger.ForTests(),
	}

	err := state.run(t.Context())
	require.ErrorIs(t, err, expected)
}

func TestBootstrapStopLogsShutdownError(t *testing.T) {
	expected := fmt.Errorf("shutdown failed")
	buf := logger.NewSyncBuffer()
	log := logger.ForTests(logger.TestLoggerWriter(buf))
	list := hooks{func(context.Context) error { return expected }}

	var state bootstrap
	state.log = log
	state.list.Store(&list)

	state.stop(context.Background())
	require.Contains(t, buf.String(), "tracing shutdown failed")
}

func TestBootstrapProvidersShutdownHooksForEnabledSignals(t *testing.T) {
	t.Cleanup(overrideProviderFactories())

	var (
		traceShutdowns  atomic.Int32
		metricShutdowns atomic.Int32
		logShutdowns    atomic.Int32
	)

	newTraceProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (traceProvider, error) {
		return fakeTraceProvider{shutdowns: &traceShutdowns}, nil
	}
	newMeterProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (meterProvider, error) {
		return fakeMeterProvider{shutdowns: &metricShutdowns}, nil
	}
	newLoggerProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (loggerProvider, error) {
		return fakeLoggerProvider{shutdowns: &logShutdowns}, nil
	}

	out, err := bootstrapProviders(t.Context(), config.TracerConfig{
		Enabled:     true,
		SendMetrics: true,
		SendLogs:    true,
	})
	require.NoError(t, err)
	require.Len(t, out, 4)

	require.NoError(t, out.run(context.Background()))
	require.Equal(t, int32(1), traceShutdowns.Load())
	require.Equal(t, int32(1), metricShutdowns.Load())
	require.Equal(t, int32(1), logShutdowns.Load())
}

func TestBootstrapProvidersReturnsProviderErrors(t *testing.T) {
	t.Run("resource error is returned", func(t *testing.T) {
		t.Cleanup(overrideProviderFactories())
		expected := fmt.Errorf("resource")

		newResourceFunc = func(context.Context, config.TracerConfig) (*resource.Resource, error) {
			return nil, expected
		}

		_, err := bootstrapProviders(t.Context(), config.TracerConfig{Enabled: true})
		require.ErrorIs(t, err, expected)
	})

	t.Run("trace provider error is returned", func(t *testing.T) {
		t.Cleanup(overrideProviderFactories())
		expected := fmt.Errorf("trace provider")

		newTraceProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (traceProvider, error) {
			return nil, expected
		}

		_, err := bootstrapProviders(t.Context(), config.TracerConfig{Enabled: true})
		require.ErrorIs(t, err, expected)
	})

	t.Run("meter provider error is returned", func(t *testing.T) {
		t.Cleanup(overrideProviderFactories())
		expected := fmt.Errorf("meter provider")

		newTraceProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (traceProvider, error) {
			return fakeTraceProvider{}, nil
		}
		newMeterProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (meterProvider, error) {
			return nil, expected
		}

		_, err := bootstrapProviders(
			t.Context(),
			config.TracerConfig{Enabled: true, SendMetrics: true},
		)
		require.ErrorIs(t, err, expected)
	})

	t.Run("logger provider error is returned", func(t *testing.T) {
		t.Cleanup(overrideProviderFactories())
		expected := fmt.Errorf("logger provider")

		newTraceProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (traceProvider, error) {
			return fakeTraceProvider{}, nil
		}
		newLoggerProviderFunc = func(context.Context, *resource.Resource, config.TracerConfig) (loggerProvider, error) {
			return nil, expected
		}

		_, err := bootstrapProviders(
			t.Context(),
			config.TracerConfig{Enabled: true, SendLogs: true},
		)
		require.ErrorIs(t, err, expected)
	})
}

func TestProviderConstructors(t *testing.T) {
	res := resource.Empty()
	cfg := config.TracerConfig{
		Endpoint: "127.0.0.1:4317",
		Insecure: true,
	}

	t.Run("trace provider", func(t *testing.T) {
		provider, err := newTraceProvider(t.Context(), res, cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	t.Run("meter provider", func(t *testing.T) {
		provider, err := newMeterProvider(t.Context(), res, cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)
	})

	t.Run("logger provider", func(t *testing.T) {
		provider, err := newLoggerProvider(t.Context(), res, cfg)
		require.NoError(t, err)
		require.NotNil(t, provider)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	t.Run("trace provider returns exporter error", func(t *testing.T) {
		t.Cleanup(overrideProviderFactories())
		expected := fmt.Errorf("trace exporter")

		newTraceExporterFunc = func(context.Context, config.TracerConfig) (sdktrace.SpanExporter, error) {
			return nil, expected
		}

		provider, err := newTraceProvider(t.Context(), res, cfg)
		require.Nil(t, provider)
		require.ErrorIs(t, err, expected)
	})

	t.Run("meter provider returns exporter error", func(t *testing.T) {
		t.Cleanup(overrideProviderFactories())
		expected := fmt.Errorf("metric exporter")

		newMetricExporterFunc = func(context.Context, config.TracerConfig) (sdkmetric.Exporter, error) {
			return nil, expected
		}

		provider, err := newMeterProvider(t.Context(), res, cfg)
		require.Nil(t, provider)
		require.ErrorIs(t, err, expected)
	})

	t.Run("logger provider returns exporter error", func(t *testing.T) {
		t.Cleanup(overrideProviderFactories())
		expected := fmt.Errorf("log exporter")

		newLogExporterFunc = func(context.Context, config.TracerConfig) (sdklog.Exporter, error) {
			return nil, expected
		}

		provider, err := newLoggerProvider(t.Context(), res, cfg)
		require.Nil(t, provider)
		require.ErrorIs(t, err, expected)
	})
}

func TestExporterConstructors(t *testing.T) {
	t.Run("trace exporters support http and grpc", func(t *testing.T) {
		httpExporter, err := newTraceExporter(t.Context(), config.TracerConfig{
			Endpoint: "127.0.0.1:4318",
			Insecure: true,
			UseHTTP:  true,
		})
		require.NoError(t, err)
		require.NotNil(t, httpExporter)
		require.NoError(t, httpExporter.Shutdown(context.Background()))

		grpcExporter, err := newTraceExporter(t.Context(), config.TracerConfig{
			Endpoint: "127.0.0.1:4317",
			Insecure: true,
		})
		require.NoError(t, err)
		require.NotNil(t, grpcExporter)
		require.NoError(t, grpcExporter.Shutdown(context.Background()))
	})

	t.Run("metric exporters support http and grpc", func(t *testing.T) {
		httpExporter, err := newMetricExporter(t.Context(), config.TracerConfig{
			Endpoint: "127.0.0.1:4318",
			Insecure: true,
			UseHTTP:  true,
		})
		require.NoError(t, err)
		require.NotNil(t, httpExporter)
		require.NoError(t, httpExporter.Shutdown(context.Background()))

		grpcExporter, err := newMetricExporter(t.Context(), config.TracerConfig{
			Endpoint: "127.0.0.1:4317",
			Insecure: true,
		})
		require.NoError(t, err)
		require.NotNil(t, grpcExporter)
		require.NoError(t, grpcExporter.Shutdown(context.Background()))
	})

	t.Run("log exporters support http and grpc", func(t *testing.T) {
		httpExporter, err := newLogExporter(t.Context(), config.TracerConfig{
			Endpoint: "127.0.0.1:4318",
			Insecure: true,
			UseHTTP:  true,
		})
		require.NoError(t, err)
		require.NotNil(t, httpExporter)
		require.NoError(t, httpExporter.Shutdown(context.Background()))

		grpcExporter, err := newLogExporter(t.Context(), config.TracerConfig{
			Endpoint: "127.0.0.1:4317",
			Insecure: true,
		})
		require.NoError(t, err)
		require.NotNil(t, grpcExporter)
		require.NoError(t, grpcExporter.Shutdown(context.Background()))
	})
}

func TestLookupSignalSpecificEnvHelpers(t *testing.T) {
	t.Run("lookup endpoint env resolves generic and per-signal values", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPEndpoint, "generic")
		t.Setenv(envOTELExporterOTLPMetricsEndpoint, "metrics")
		t.Setenv(envOTELExporterOTLPLogsEndpoint, "logs")

		value, ok := lookupEndpointEnv("traces")
		require.True(t, ok)
		require.Equal(t, "generic", value)

		value, ok = lookupEndpointEnv("metrics")
		require.True(t, ok)
		require.Equal(t, "metrics", value)

		value, ok = lookupEndpointEnv("logs")
		require.True(t, ok)
		require.Equal(t, "logs", value)
	})

	t.Run("lookup insecure env resolves generic and per-signal values", func(t *testing.T) {
		t.Setenv(envOTELExporterOTLPInsecure, "generic")
		t.Setenv(envOTELExporterOTLPMetricsInsecure, "metrics")
		t.Setenv(envOTELExporterOTLPLogsInsecure, "logs")

		value, ok := lookupInsecureEnv("traces")
		require.True(t, ok)
		require.Equal(t, "generic", value)

		value, ok = lookupInsecureEnv("metrics")
		require.True(t, ok)
		require.Equal(t, "metrics", value)

		value, ok = lookupInsecureEnv("logs")
		require.True(t, ok)
		require.Equal(t, "logs", value)
	})
}

func resourceValue(res *resource.Resource, key attribute.Key) string {
	value, ok := res.Set().Value(key)
	if !ok {
		return ""
	}

	return value.AsString()
}

func overrideProviderFactories() func() {
	prevResource := newResourceFunc
	prevTrace := newTraceProviderFunc
	prevMetric := newMeterProviderFunc
	prevLogger := newLoggerProviderFunc
	prevTraceExporter := newTraceExporterFunc
	prevMetricExporter := newMetricExporterFunc
	prevLogExporter := newLogExporterFunc

	return func() {
		newResourceFunc = prevResource
		newTraceProviderFunc = prevTrace
		newMeterProviderFunc = prevMetric
		newLoggerProviderFunc = prevLogger
		newTraceExporterFunc = prevTraceExporter
		newMetricExporterFunc = prevMetricExporter
		newLogExporterFunc = prevLogExporter
	}
}

type fakeTraceProvider struct {
	tracenoop.TracerProvider
	shutdowns *atomic.Int32
}

func (f fakeTraceProvider) Shutdown(context.Context) error {
	f.shutdowns.Add(1)
	return nil
}

type fakeMeterProvider struct {
	metricnoop.MeterProvider
	shutdowns *atomic.Int32
}

func (f fakeMeterProvider) Shutdown(context.Context) error {
	f.shutdowns.Add(1)
	return nil
}

type fakeLoggerProvider struct {
	otellognoop.LoggerProvider
	shutdowns *atomic.Int32
}

func (f fakeLoggerProvider) Shutdown(context.Context) error {
	f.shutdowns.Add(1)
	return nil
}

type fakeOTLPCollector struct {
	mu     sync.Mutex
	server *httptest.Server

	logs   []*collogpb.ExportLogsServiceRequest
	traces []*coltracepb.ExportTraceServiceRequest
	errs   []error
}

type testApp struct {
	cancel    context.CancelCauseFunc
	collector *fakeOTLPCollector
	done      chan error
	log       *logger.Logger
	svc       serviceStarterStopper
	t         *testing.T
}

type serviceStarterStopper interface {
	Start(context.Context) error
	Stop(context.Context)
}

func newFakeOTLPCollector(t *testing.T) *fakeOTLPCollector {
	t.Helper()

	collector := &fakeOTLPCollector{}
	collector.server = httptest.NewServer(http.HandlerFunc(collector.handle))
	t.Cleanup(collector.server.Close)

	return collector
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()

	prevTraceProvider := otel.GetTracerProvider()
	prevMeterProvider := otel.GetMeterProvider()
	prevPropagator := otel.GetTextMapPropagator()
	prevLogProvider := otellogglobal.GetLoggerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTraceProvider)
		otel.SetMeterProvider(prevMeterProvider)
		otel.SetTextMapPropagator(prevPropagator)
		otellogglobal.SetLoggerProvider(prevLogProvider)
		logger.SetOpenTelemetryBridge(false)
	})

	collector := newFakeOTLPCollector(t)
	collector.configureEnv(t)

	var cfg config.TracerConfig
	cfg.SendLogs = true
	cfg.UseHTTP = true
	cfg.SetAppNameAndVersion("payments-api", "1.2.3")

	buf := logger.NewSyncBuffer()
	log := logger.ForTests(logger.TestLoggerWriter(buf))
	svc := Init(log, cfg)

	ctx, cancel := context.WithCancelCause(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- svc.Start(ctx)
	}()

	require.Eventually(t, func() bool {
		return bytes.Contains(buf.Bytes(), []byte("tracing initialized"))
	}, time.Second, 10*time.Millisecond)

	return &testApp{
		cancel:    cancel,
		collector: collector,
		done:      done,
		log:       log,
		svc:       svc,
		t:         t,
	}
}

func (a *testApp) processPayment() {
	ctx, span := otel.Tracer("payments-service").Start(context.Background(), "process-payment")
	a.log.InfoContext(ctx, "payment completed",
		logger.String("order_id", "A-42"),
		logger.Bool("bridge_expected", true),
	)
	span.End()
}

func (a *testApp) stop() {
	a.cancel(context.Canceled)
	require.ErrorIs(a.t, <-a.done, context.Canceled)
	a.svc.Stop(context.Background())
	a.collector.waitForExport(a.t)
	require.Empty(a.t, a.collector.errors())
}

func (c *fakeOTLPCollector) url(path string) string {
	return c.server.URL + path
}

func (c *fakeOTLPCollector) configureEnv(t *testing.T) {
	t.Helper()

	t.Setenv(envOTELExporterOTLPTracesEndpoint, c.url("/v1/traces"))
	t.Setenv(envOTELExporterOTLPLogsEndpoint, c.url("/v1/logs"))
}

func (c *fakeOTLPCollector) waitForExport(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		return c.traceCount() > 0 && c.logCount() > 0
	}, time.Second, 10*time.Millisecond)
}

func (c *fakeOTLPCollector) handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		c.addError(fmt.Errorf("read OTLP request body: %w", err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer func() { _ = r.Body.Close() }()

	switch r.URL.Path {
	case "/v1/logs":
		req := new(collogpb.ExportLogsServiceRequest)
		if err = proto.Unmarshal(body, req); err != nil {
			c.addError(fmt.Errorf("unmarshal OTLP logs request: %w", err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		c.mu.Lock()
		c.logs = append(c.logs, req)
		c.mu.Unlock()

		payload, marshalErr := proto.Marshal(new(collogpb.ExportLogsServiceResponse))
		if marshalErr != nil {
			c.addError(fmt.Errorf("marshal OTLP logs response: %w", marshalErr))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	case "/v1/traces":
		req := new(coltracepb.ExportTraceServiceRequest)
		if err = proto.Unmarshal(body, req); err != nil {
			c.addError(fmt.Errorf("unmarshal OTLP traces request: %w", err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		c.mu.Lock()
		c.traces = append(c.traces, req)
		c.mu.Unlock()

		payload, marshalErr := proto.Marshal(new(coltracepb.ExportTraceServiceResponse))
		if marshalErr != nil {
			c.addError(fmt.Errorf("marshal OTLP traces response: %w", marshalErr))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	default:
		c.addError(fmt.Errorf("unexpected OTLP path: %s", r.URL.Path))
		w.WriteHeader(http.StatusNotFound)
	}
}

func (c *fakeOTLPCollector) addError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.errs = append(c.errs, err)
}

func (c *fakeOTLPCollector) errors() []error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]error(nil), c.errs...)
}

func (c *fakeOTLPCollector) traceCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.traces)
}

func (c *fakeOTLPCollector) logCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return len(c.logs)
}

func (c *fakeOTLPCollector) firstTraceRequest() *coltracepb.ExportTraceServiceRequest {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.traces) == 0 {
		return nil
	}

	return c.traces[0]
}

func (c *fakeOTLPCollector) firstLogRequest() *collogpb.ExportLogsServiceRequest {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.logs) == 0 {
		return nil
	}

	return c.logs[0]
}

func (c *fakeOTLPCollector) resourceLogAttrs(t *testing.T) map[string]string {
	t.Helper()

	req := c.firstLogRequest()
	require.NotNil(t, req)
	require.NotEmpty(t, req.ResourceLogs)
	require.NotNil(t, req.ResourceLogs[0].Resource)

	return protoAttrs(req.ResourceLogs[0].Resource.Attributes)
}

func findExportedSpan(
	t *testing.T,
	req *coltracepb.ExportTraceServiceRequest,
	name string,
) *tracepb.Span {
	t.Helper()

	require.NotNil(t, req)

	for _, resourceSpans := range req.ResourceSpans {
		for _, scopeSpans := range resourceSpans.ScopeSpans {
			for _, span := range scopeSpans.Spans {
				if span.Name == name {
					return span
				}
			}
		}
	}

	t.Fatalf("span %q not found in exported traces", name)
	return nil
}

func findExportedLog(
	t *testing.T,
	req *collogpb.ExportLogsServiceRequest,
	body string,
) *logspb.LogRecord {
	t.Helper()

	require.NotNil(t, req)

	for _, resourceLogs := range req.ResourceLogs {
		for _, scopeLogs := range resourceLogs.ScopeLogs {
			for _, record := range scopeLogs.LogRecords {
				if protoValue(record.Body) == body {
					return record
				}
			}
		}
	}

	t.Fatalf("log record %q not found in exported logs", body)
	return nil
}

func protoAttrs(attrs []*commonpb.KeyValue) map[string]string {
	out := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = protoValue(attr.Value)
	}

	return out
}

func protoValue(value *commonpb.AnyValue) string {
	if value == nil {
		return ""
	}

	switch typed := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		if typed.BoolValue {
			return "true"
		}

		return "false"
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", typed.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%v", typed.DoubleValue)
	case *commonpb.AnyValue_BytesValue:
		return string(typed.BytesValue)
	default:
		return value.String()
	}
}
