package logger

import (
	"context"
	"log/slog"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func BenchmarkAddContextAttrs(b *testing.B) {
	attrs := []Attr{
		String("request_id", "r-1"),
		String("user_id", "u-1"),
		String("tenant_id", "t-1"),
	}

	b.Run("empty", func(b *testing.B) {
		ctx := context.Background()

		for b.Loop() {
			out := AddContextAttrs(ctx, attrs...)
			if got := len(fromContext(out)); got != len(attrs) {
				b.Fatalf("expected %d attrs, got %d", len(attrs), got)
			}
		}
	})

	b.Run("append", func(b *testing.B) {
		ctx := AddContextAttrs(context.Background(),
			String("existing_1", "v1"),
			String("existing_2", "v2"),
		)

		for b.Loop() {
			out := AddContextAttrs(ctx, attrs...)
			if got := len(fromContext(out)); got != 5 {
				b.Fatalf("expected 5 attrs, got %d", got)
			}
		}
	})
}

func BenchmarkOpenTracingTransform(b *testing.B) {
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithIDGenerator(stubIDGenerator(1)),
	)
	b.Cleanup(func() { _ = provider.Shutdown(context.Background()) })

	tracer := provider.Tracer("bench")
	ctx, span := tracer.Start(context.Background(), "bench-span")
	b.Cleanup(func() { span.End() })

	record := slog.NewRecord(time.Now(), slog.LevelError, "benchmark message", 0)
	record.AddAttrs(
		String("component", "bench"),
		Int("status", 500),
		Bool("retry", false),
	)

	b.Run("recording-span", func(b *testing.B) {
		for b.Loop() {
			out := openTracingTransform(ctx, record.Clone())
			if out.NumAttrs() == 0 {
				b.Fatal("expected transformed record to keep attrs")
			}
		}
	})

	b.Run("no-span", func(b *testing.B) {
		noSpanCtx := trace.ContextWithSpanContext(context.Background(), trace.SpanContext{})

		for b.Loop() {
			out := openTracingTransform(noSpanCtx, record.Clone())
			if out.NumAttrs() == 0 {
				b.Fatal("expected original record attrs to remain")
			}
		}
	})
}
