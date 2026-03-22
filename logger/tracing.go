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

// LogSeverityKey represents the attribute key for the severity level of a log record.
const LogSeverityKey = attribute.Key("log.severity")

// LogMessageKey represents the attribute key for the message content of a log record.
const LogMessageKey = attribute.Key("log.message")

// fetchAllAttributes returns a sequence of all attributes found in the given log record.
// The sequence can be iterated in a for range, allowing inspection or modification of its attributes.
//
// Parameters:
//   - record: The slog.Record from which to extract attributes.
//
// Returns:
//   - A function that yields each attribute in the log record.
func fetchAllAttributes(record slog.Record) iter.Seq[slog.Attr] {
	return func(yield func(slog.Attr) bool) { record.Attrs(yield) }
}

// fromSpanContext extracts span and trace identifiers from the provided span context
// and returns them as a grouped attribute.
//
// Parameters:
//   - ctx: The span context containing trace and span IDs.
//
// Returns:
//   - A grouped attribute containing "span_id" and "trace_id" if available; otherwise an empty attribute.
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

// openTracingTransform adds tracing information to a log record and creates an OpenTelemetry event.
// This function also sets error status when the log level indicates an error.
//
// Parameters:
//   - ctx: The context that may contain an active tracing span.
//   - original: The log record to enhance with tracing information.
//
// Returns:
//   - An updated slog.Record containing trace context attributes. If a trace span is active,
//     a corresponding event is added with caller information.
func openTracingTransform(ctx context.Context, original slog.Record) slog.Record {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return original
	}

	// Prepare an attribute list for the OpenTelemetry event.
	attrs := make([]attribute.KeyValue, 0, original.NumAttrs()+5)
	attrs = append(attrs, LogSeverityKey.String(original.Level.String()))
	attrs = append(attrs, LogMessageKey.String(original.Message))

	// Capture all attributes from the log record.
	for attr := range fetchAllAttributes(original) {
		attrs = append(attrs, attribute.Stringer(attr.Key, attr.Value))
	}

	// Include caller information if available.
	if frame, _ := runtime.CallersFrames([]uintptr{original.PC}).Next(); frame.PC != 0 {
		attrs = append(attrs,
			semconv.CodeFunctionKey.String(frame.Function),
			semconv.CodeFilepathKey.String(frame.File),
			semconv.CodeLineNumberKey.Int(frame.Line),
		)
	}

	// Record the event in the active span.
	if !openTelemetryBridgeEnabled() {
		span.AddEvent("log", trace.WithAttributes(attrs...))
	}
	if original.Level >= slog.LevelError {
		span.SetStatus(codes.Error, original.Message)
	}

	// Attach trace identifiers to the log record itself.
	original.Add(fromSpanContext(span.SpanContext()))

	return original
}
