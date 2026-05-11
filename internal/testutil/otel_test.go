package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	otellog "go.opentelemetry.io/otel/log"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	otellognoop "go.opentelemetry.io/otel/log/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
)

func TestInstallOTelRecorder(t *testing.T) {
	prevTraceProvider := nooptrace.NewTracerProvider()
	prevPropagator := propagation.Baggage{}
	prevLogProvider := otellognoop.NewLoggerProvider()

	otel.SetTracerProvider(prevTraceProvider)
	otel.SetTextMapPropagator(prevPropagator)
	otellogglobal.SetLoggerProvider(prevLogProvider)

	var recorder *OTelRecorder
	var spanCtx trace.SpanContext

	t.Run("records spans and logs", func(t *testing.T) {
		recorder = InstallOTelRecorder(t)

		parentSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    trace.TraceID{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			SpanID:     trace.SpanID{2, 2, 2, 2, 2, 2, 2, 2},
			TraceFlags: trace.FlagsSampled,
			Remote:     true,
		})

		ctx := trace.ContextWithSpanContext(context.Background(), parentSpanCtx)
		ctx, span := otel.Tracer("testutil/otel").Start(ctx, "demo-span")
		spanCtx = span.SpanContext()
		span.End()

		record := new(otellog.Record)
		record.SetBody(otellog.StringValue("demo-log"))
		otellogglobal.GetLoggerProvider().Logger("testutil/otel").Emit(ctx, *record)

		gotSpan, ok := recorder.FindSpan("demo-span")
		require.True(t, ok)
		require.Equal(t, spanCtx.TraceID(), gotSpan.TraceID)
		require.Equal(t, spanCtx.SpanID(), gotSpan.SpanID)
		require.Equal(t, parentSpanCtx.SpanID(), gotSpan.ParentSpan)

		gotLog, ok := recorder.FindLog("demo-log")
		require.True(t, ok)
		require.Equal(t, spanCtx.TraceID(), gotLog.TraceID)
		require.Equal(t, spanCtx.SpanID(), gotLog.SpanID)
	})

	require.Equal(t, prevTraceProvider, otel.GetTracerProvider())
	require.Equal(t, prevLogProvider, otellogglobal.GetLoggerProvider())
	require.Equal(t, prevPropagator, otel.GetTextMapPropagator())
}

func TestOTelRecorderFindMiss(t *testing.T) {
	recorder := new(OTelRecorder)

	_, ok := recorder.FindSpan("missing")
	require.False(t, ok)

	_, ok = recorder.FindLog("missing")
	require.False(t, ok)
}

func TestOTelExporterNoopMethods(t *testing.T) {
	t.Run("span exporter shutdown returns nil", func(t *testing.T) {
		require.NoError(t, (spanExporter{}).Shutdown(context.Background()))
	})

	t.Run("log exporter methods return nil", func(t *testing.T) {
		exporter := logExporter{}
		require.NoError(t, exporter.Shutdown(context.Background()))
		require.NoError(t, exporter.ForceFlush(context.Background()))
	})
}
