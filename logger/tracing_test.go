package logger

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func Test_tracingTransformer(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sr),
		sdktrace.WithIDGenerator(stubIDGenerator(1)))

	tracer := provider.Tracer("test")

	_, span := tracer.Start(context.TODO(), "main")
	defer span.End()

	{ // use valid context
		attributes := fromSpanContext(span.SpanContext()).Value
		require.Equal(t, slog.KindGroup, attributes.Kind())

		values := attributes.Group()
		require.Len(t, values, 2)
		require.Contains(t, values, String("span_id", "0200000000000000"))
		require.Contains(t, values, String("trace_id", "01000000000000000000000000000000"))
	}

	{ // use invalid context
		attributes := fromSpanContext(trace.SpanContext{}).Value
		require.Empty(t, attributes)
		require.NotEqual(t, slog.KindGroup, attributes.Kind())
	}
}
