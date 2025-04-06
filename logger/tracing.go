package logger

import (
	"context"
	"iter"
	"log/slog"
	"runtime"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Log attributes commonly used for structured logging.
//
// These attributes help standardize log metadata for better observability.
const (
	// LogSeverityKey represents the severity level of the log message.
	LogSeverityKey = attribute.Key("log.severity")

	// LogMessageKey contains the actual log message.
	LogMessageKey = attribute.Key("log.message")
)

func fetchAllAttributes(record slog.Record) iter.Seq[slog.Attr] {
	return func(yield func(slog.Attr) bool) { record.Attrs(yield) }
}

func fromSpanContext(ctx trace.SpanContext) Attr {
	out := make([]any, 0, 2)
	if ctx.HasSpanID() {
		out = append(out, String("span_id", ctx.SpanID().String()))
	}

	if ctx.HasTraceID() {
		out = append(out, String("trace_id", ctx.TraceID().String()))
	}

	if len(out) > 0 {
		return Group("trace", out...)
	}

	return Attr{}
}

// openTracingTransform transforms original slog.Record and apply opentracing.
func openTracingTransform(ctx context.Context, original slog.Record) slog.Record {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return original
	}

	attrs := make(
		[]attribute.KeyValue,
		0,
		original.NumAttrs()+5,
	) // 2 (severity, message) + 3 (func,file,line).

	attrs = append(attrs, LogSeverityKey.String(original.Level.String()))
	attrs = append(attrs, LogMessageKey.String(original.Message))

	for attr := range fetchAllAttributes(original) {
		attrs = append(attrs, attribute.Stringer(attr.Key, attr.Value))
	}

	// add caller if exists
	if frame, _ := runtime.CallersFrames([]uintptr{original.PC}).Next(); frame.PC != 0 {
		attrs = append(attrs,
			semconv.CodeFunctionKey.String(frame.Function),
			semconv.CodeFilepathKey.String(frame.File),
			semconv.CodeLineNumberKey.Int(frame.Line))
	}

	span.AddEvent("log", trace.WithAttributes(attrs...))
	if original.Level >= slog.LevelError {
		span.SetStatus(codes.Error, original.Message)
	}

	// try to add span_id and trace_id from span
	original.Add(fromSpanContext(span.SpanContext()))

	return original
}
