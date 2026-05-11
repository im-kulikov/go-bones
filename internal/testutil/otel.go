package testutil

import (
	"context"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// RecordedSpan represents a span captured during testing.
type RecordedSpan struct {
	Name       string
	TraceID    trace.TraceID
	SpanID     trace.SpanID
	ParentSpan trace.SpanID
}

// RecordedLog represents a log entry captured during testing.
type RecordedLog struct {
	Body    string
	TraceID trace.TraceID
	SpanID  trace.SpanID
}

// OTelRecorder captures OpenTelemetry spans and logs for assertion in tests.
type OTelRecorder struct {
	mu    sync.Mutex
	spans []RecordedSpan
	logs  []RecordedLog
}

// InstallOTelRecorder sets up a global OpenTelemetry recorder for the duration of the test.
// It replaces the global tracer and logger providers and restores them during test cleanup.
func InstallOTelRecorder(t testing.TB) *OTelRecorder {
	t.Helper()

	prevTraceProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	prevLogProvider := otellogglobal.GetLoggerProvider()

	recorder := &OTelRecorder{}

	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(
			sdktrace.NewSimpleSpanProcessor(spanExporter{recorder: recorder}),
		),
	)

	logProvider := sdklog.NewLoggerProvider(
		sdklog.WithResource(sdkresource.Empty()),
		sdklog.WithProcessor(sdklog.NewSimpleProcessor(logExporter{recorder: recorder})),
	)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	otellogglobal.SetLoggerProvider(logProvider)

	t.Cleanup(func() {
		_ = logProvider.Shutdown(context.Background())
		_ = traceProvider.Shutdown(context.Background())
		otellogglobal.SetLoggerProvider(prevLogProvider)
		otel.SetTextMapPropagator(prevPropagator)
		otel.SetTracerProvider(prevTraceProvider)
	})

	return recorder
}

// FindSpan searches for a captured span by name.
func (r *OTelRecorder) FindSpan(name string) (RecordedSpan, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, span := range r.spans {
		if span.Name == name {
			return span, true
		}
	}

	return RecordedSpan{}, false
}

// FindLog searches for a captured log by its body content.
func (r *OTelRecorder) FindLog(body string) (RecordedLog, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, record := range r.logs {
		if record.Body == body {
			return record, true
		}
	}

	return RecordedLog{}, false
}

type spanExporter struct {
	recorder *OTelRecorder
}

func (e spanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.recorder.mu.Lock()
	defer e.recorder.mu.Unlock()

	for _, span := range spans {
		parent := span.Parent()
		e.recorder.spans = append(e.recorder.spans, RecordedSpan{
			Name:       span.Name(),
			TraceID:    span.SpanContext().TraceID(),
			SpanID:     span.SpanContext().SpanID(),
			ParentSpan: parent.SpanID(),
		})
	}

	return nil
}

func (e spanExporter) Shutdown(context.Context) error { return nil }

type logExporter struct {
	recorder *OTelRecorder
}

func (e logExporter) Export(_ context.Context, records []sdklog.Record) error {
	e.recorder.mu.Lock()
	defer e.recorder.mu.Unlock()

	for _, record := range records {
		e.recorder.logs = append(e.recorder.logs, RecordedLog{
			Body:    record.Body().AsString(),
			TraceID: record.TraceID(),
			SpanID:  record.SpanID(),
		})
	}

	return nil
}

func (e logExporter) Shutdown(context.Context) error { return nil }

func (e logExporter) ForceFlush(context.Context) error { return nil }
