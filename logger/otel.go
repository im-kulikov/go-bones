package logger

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	openTelemetryLoggerName = "github.com/im-kulikov/go-bones/logger"
	defaultErrorKey         = "error"
)

// openTelemetryBridge is process-wide state, deliberately: SetOpenTelemetryBridge
// toggles export for every Logger in the process (see defaultLogger in default.go
// for the same tradeoff), so multiple independently configured loggers cannot have
// this enabled selectively.
// nolint:gochecknoglobals
var openTelemetryBridge atomic.Bool

type openTelemetryTransformer struct{}

// SetOpenTelemetryBridge enables or disables exporting logger records to the
// process-wide OpenTelemetry LoggerProvider.
func SetOpenTelemetryBridge(enabled bool) {
	openTelemetryBridge.Store(enabled)
}

func openTelemetryLogger() otellog.Logger {
	return otel.Logger(openTelemetryLoggerName)
}

func openTelemetryBridgeEnabled() bool {
	return openTelemetryBridge.Load()
}

func newOpenTelemetryBridge() slogTransformer {
	return openTelemetryTransformer{}
}

func (openTelemetryTransformer) Transform(ctx context.Context, record Record) Record {
	if !openTelemetryBridgeEnabled() {
		return record
	}

	logger := openTelemetryLogger()
	params := otellog.EnabledParameters{Severity: toOTelSeverity(record.Level)}

	if !logger.Enabled(ctx, params) {
		return record
	}

	logger.Emit(ctx, toOTelRecord(ctx, record))

	return record
}

func toOTelRecord(ctx context.Context, record Record) otellog.Record {
	var out otellog.Record

	out.SetTimestamp(record.Time)
	out.SetObservedTimestamp(time.Now())
	out.SetSeverity(toOTelSeverity(record.Level))
	out.SetSeverityText(record.Level.String())
	out.SetBody(attribute.StringValue(record.Message))

	if errAttr, ok := recordError(record); ok {
		if errVal, isErr := errAttr.Value.Any().(error); isErr {
			out.SetErr(errVal)
		} else {
			out.SetErr(fmt.Errorf("%v", errAttr.Value.Any()))
		}
	}

	attrs := make([]attribute.KeyValue, 0, record.NumAttrs()+3)
	for attr := range fetchAllAttributes(record) {
		attrs = append(attrs, toOTelAttr(attr))
	}

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		attrs = append(attrs, spanContextAttrs(spanCtx)...)
	}

	if frame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next(); frame.PC != 0 {
		attrs = append(attrs,
			attribute.String(string(semconv.CodeFunctionKey), frame.Function),
			attribute.String(string(semconv.CodeFilepathKey), frame.File),
			attribute.Int(string(semconv.CodeLineNumberKey), frame.Line),
		)
	}

	out.AddAttributes(attrs...)

	return out
}

func recordError(record Record) (slog.Attr, bool) {
	var (
		out slog.Attr
		ok  bool
	)

	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == defaultErrorKey {
			out = attr
			ok = true
			return false
		}

		return true
	})

	return out, ok
}

func spanContextAttrs(spanCtx trace.SpanContext) []attribute.KeyValue {
	out := make([]attribute.KeyValue, 0, 2)
	if spanCtx.HasTraceID() {
		out = append(out, attribute.String("trace_id", spanCtx.TraceID().String()))
	}
	if spanCtx.HasSpanID() {
		out = append(out, attribute.String("span_id", spanCtx.SpanID().String()))
	}

	return out
}

func toOTelAttr(attr slog.Attr) attribute.KeyValue {
	return attribute.KeyValue{
		Key:   attribute.Key(attr.Key),
		Value: toOTelValue(attr.Value),
	}
}

func toOTelValue(value slog.Value) attribute.Value {
	if out, ok := toOTelScalarValue(value); ok {
		return out
	}

	if out, ok := toOTelCompositeValue(value); ok {
		return out
	}

	return attribute.StringValue(stringValueFallback(value))
}

func toOTelScalarValue(value slog.Value) (attribute.Value, bool) {
	switch value.Kind() {
	case slog.KindString:
		return attribute.StringValue(value.String()), true
	case slog.KindInt64:
		return attribute.Int64Value(value.Int64()), true
	case slog.KindUint64:
		return attribute.StringValue(strconv.FormatUint(value.Uint64(), 10)), true
	case slog.KindFloat64:
		return attribute.Float64Value(value.Float64()), true
	case slog.KindBool:
		return attribute.BoolValue(value.Bool()), true
	case slog.KindDuration:
		return attribute.StringValue(value.Duration().String()), true
	case slog.KindTime:
		return attribute.StringValue(value.Time().Format(time.RFC3339Nano)), true
	default:
		return attribute.Value{}, false
	}
}

func toOTelCompositeValue(value slog.Value) (attribute.Value, bool) {
	switch value.Kind() {
	case slog.KindGroup:
		group := value.Group()
		attrs := make([]attribute.KeyValue, 0, len(group))
		for _, item := range group {
			attrs = append(attrs, toOTelAttr(item))
		}

		return attribute.MapValue(attrs...), true
	case slog.KindLogValuer:
		return toOTelValue(value.Resolve()), true
	case slog.KindAny:
		return anyToOTelValue(value.Any()), true
	default:
		return attribute.Value{}, false
	}
}

func anyToOTelValue(value any) attribute.Value {
	switch v := value.(type) {
	case nil:
		return attribute.StringValue("<nil>")
	case string:
		return attribute.StringValue(v)
	case []byte:
		return attribute.ByteSliceValue(v)
	case error:
		return attribute.StringValue(v.Error())
	case fmt.Stringer:
		return attribute.StringValue(v.String())
	case []any:
		items := make([]attribute.Value, 0, len(v))
		for _, item := range v {
			items = append(items, anyToOTelValue(item))
		}

		return attribute.SliceValue(items...)
	default:
		return attribute.StringValue(fmt.Sprint(v))
	}
}

func stringValueFallback(value slog.Value) (out string) {
	defer func() {
		if recover() != nil {
			out = value.Kind().String()
		}
	}()

	return value.String()
}

func toOTelSeverity(level slog.Level) otellog.Severity {
	switch {
	case level <= slog.LevelDebug:
		return otellog.SeverityDebug
	case level < slog.LevelWarn:
		return otellog.SeverityInfo
	case level < slog.LevelError:
		return otellog.SeverityWarn
	default:
		return otellog.SeverityError
	}
}
