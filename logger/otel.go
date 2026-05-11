package logger

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	otellog "go.opentelemetry.io/otel/log"
	logglobal "go.opentelemetry.io/otel/log/global"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	openTelemetryLoggerName = "github.com/im-kulikov/go-bones/logger"
	defaultErrorKey         = "error"
)

var (
	// nolint:gochecknoglobals
	openTelemetryBridge atomic.Bool
	// nolint:gochecknoglobals
	globalOpenTelemetryLogger = logglobal.Logger(openTelemetryLoggerName)
)

type openTelemetryTransformer struct{}

// SetOpenTelemetryBridge enables or disables exporting logger records to the
// process-wide OpenTelemetry LoggerProvider.
func SetOpenTelemetryBridge(enabled bool) {
	openTelemetryBridge.Store(enabled)
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

	params := otellog.EnabledParameters{Severity: toOTelSeverity(record.Level)}
	if !globalOpenTelemetryLogger.Enabled(ctx, params) {
		return record
	}

	globalOpenTelemetryLogger.Emit(ctx, toOTelRecord(ctx, record))

	return record
}

func toOTelRecord(ctx context.Context, record Record) otellog.Record {
	var out otellog.Record

	out.SetTimestamp(record.Time)
	out.SetObservedTimestamp(time.Now())
	out.SetSeverity(toOTelSeverity(record.Level))
	out.SetSeverityText(record.Level.String())
	out.SetBody(otellog.StringValue(record.Message))

	if errAttr, ok := recordError(record); ok {
		out.SetErr(fmt.Errorf("%v", errAttr.Value.Any()))
	}

	attrs := make([]otellog.KeyValue, 0, record.NumAttrs()+3)
	for attr := range fetchAllAttributes(record) {
		attrs = append(attrs, toOTelAttr(attr))
	}

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		attrs = append(attrs, spanContextAttrs(spanCtx)...)
	}

	if frame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next(); frame.PC != 0 {
		attrs = append(attrs,
			otellog.String(string(semconv.CodeFunctionKey), frame.Function),
			otellog.String(string(semconv.CodeFilepathKey), frame.File),
			otellog.Int(string(semconv.CodeLineNumberKey), frame.Line),
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

func spanContextAttrs(spanCtx trace.SpanContext) []otellog.KeyValue {
	out := make([]otellog.KeyValue, 0, 2)
	if spanCtx.HasTraceID() {
		out = append(out, otellog.String("trace_id", spanCtx.TraceID().String()))
	}
	if spanCtx.HasSpanID() {
		out = append(out, otellog.String("span_id", spanCtx.SpanID().String()))
	}

	return out
}

func toOTelAttr(attr slog.Attr) otellog.KeyValue {
	return otellog.KeyValue{
		Key:   attr.Key,
		Value: toOTelValue(attr.Value),
	}
}

func toOTelValue(value slog.Value) otellog.Value {
	if out, ok := toOTelScalarValue(value); ok {
		return out
	}

	if out, ok := toOTelCompositeValue(value); ok {
		return out
	}

	return otellog.StringValue(stringValueFallback(value))
}

func toOTelScalarValue(value slog.Value) (otellog.Value, bool) {
	switch value.Kind() {
	case slog.KindString:
		return otellog.StringValue(value.String()), true
	case slog.KindInt64:
		return otellog.Int64Value(value.Int64()), true
	case slog.KindUint64:
		return otellog.StringValue(strconv.FormatUint(value.Uint64(), 10)), true
	case slog.KindFloat64:
		return otellog.Float64Value(value.Float64()), true
	case slog.KindBool:
		return otellog.BoolValue(value.Bool()), true
	case slog.KindDuration:
		return otellog.StringValue(value.Duration().String()), true
	case slog.KindTime:
		return otellog.StringValue(value.Time().Format(time.RFC3339Nano)), true
	default:
		return otellog.Value{}, false
	}
}

func toOTelCompositeValue(value slog.Value) (otellog.Value, bool) {
	switch value.Kind() {
	case slog.KindGroup:
		group := value.Group()
		attrs := make([]otellog.KeyValue, 0, len(group))
		for _, item := range group {
			attrs = append(attrs, toOTelAttr(item))
		}

		return otellog.MapValue(attrs...), true
	case slog.KindLogValuer:
		return toOTelValue(value.Resolve()), true
	case slog.KindAny:
		return anyToOTelValue(value.Any()), true
	default:
		return otellog.Value{}, false
	}
}

func anyToOTelValue(value any) otellog.Value {
	switch v := value.(type) {
	case nil:
		return otellog.StringValue("<nil>")
	case string:
		return otellog.StringValue(v)
	case []byte:
		return otellog.BytesValue(v)
	case fmt.Stringer:
		return otellog.StringValue(v.String())
	case error:
		return otellog.StringValue(v.Error())
	case []any:
		items := make([]otellog.Value, 0, len(v))
		for _, item := range v {
			items = append(items, anyToOTelValue(item))
		}

		return otellog.SliceValue(items...)
	default:
		return otellog.StringValue(fmt.Sprint(v))
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
