package logger

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	otellog "go.opentelemetry.io/otel/log"
	otelembedded "go.opentelemetry.io/otel/log/embedded"
	logglobal "go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type capturedRecord struct {
	Body         string
	SeverityText string
	TraceID      string
	SpanID       string
	Attributes   map[string]any
}

type captureProcessor struct {
	records []capturedRecord
}

type (
	stubStringer  string
	stubLogValuer string
)

type fakeOTelLogger struct {
	otelembedded.Logger

	enabled bool
	emitted int
}

func (s stubStringer) String() string { return string(s) }
func (s stubLogValuer) LogValue() slog.Value {
	return slog.StringValue(string(s))
}
func (f *fakeOTelLogger) Emit(context.Context, otellog.Record) { f.emitted++ }
func (f *fakeOTelLogger) Enabled(context.Context, otellog.EnabledParameters) bool {
	return f.enabled
}

func (p *captureProcessor) OnEmit(_ context.Context, record *sdklog.Record) error {
	item := capturedRecord{
		Body:         record.Body().AsString(),
		SeverityText: record.SeverityText(),
		TraceID:      record.TraceID().String(),
		SpanID:       record.SpanID().String(),
		Attributes:   make(map[string]any, record.AttributesLen()),
	}

	record.WalkAttributes(func(attr otellog.KeyValue) bool {
		item.Attributes[attr.Key] = otelValue(attr.Value)
		return true
	})

	p.records = append(p.records, item)

	return nil
}

func (*captureProcessor) Enabled(context.Context, sdklog.EnabledParameters) bool { return true }
func (*captureProcessor) Shutdown(context.Context) error                         { return nil }
func (*captureProcessor) ForceFlush(context.Context) error                       { return nil }

func otelValue(value otellog.Value) any {
	if out, ok := scalarOTelValue(value); ok {
		return out
	}

	if out, ok := compositeOTelValue(value); ok {
		return out
	}

	return nil
}

func scalarOTelValue(value otellog.Value) (any, bool) {
	switch value.Kind() {
	case otellog.KindString:
		return value.AsString(), true
	case otellog.KindInt64:
		return value.AsInt64(), true
	case otellog.KindFloat64:
		return value.AsFloat64(), true
	case otellog.KindBool:
		return value.AsBool(), true
	case otellog.KindBytes:
		return value.AsBytes(), true
	default:
		return nil, false
	}
}

func compositeOTelValue(value otellog.Value) (any, bool) {
	switch value.Kind() {
	case otellog.KindSlice:
		items := value.AsSlice()
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, otelValue(item))
		}

		return out, true
	case otellog.KindMap:
		items := value.AsMap()
		out := make(map[string]any, len(items))
		for _, item := range items {
			out[item.Key] = otelValue(item.Value)
		}

		return out, true
	default:
		return nil, false
	}
}

func TestOpenTelemetryBridge(t *testing.T) {
	defer SetOpenTelemetryBridge(false)

	processor := new(captureProcessor)
	logProvider := sdklog.NewLoggerProvider(sdklog.WithProcessor(processor))
	logglobal.SetLoggerProvider(logProvider)
	SetOpenTelemetryBridge(true)

	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(spanRecorder),
		sdktrace.WithIDGenerator(stubIDGenerator(1)),
	)
	tracer := traceProvider.Tracer("test")

	ctx, span := tracer.Start(context.Background(), "main")

	buf := new(bytes.Buffer)
	log := ForTests(TestLoggerWriter(buf))
	log.ErrorContext(
		AddContextAttrs(ctx, String("ctxKey", "ctxVal")),
		"bridge message",
		Err(context.Canceled),
	)

	span.End()

	require.Len(t, processor.records, 1)
	record := processor.records[0]
	require.Equal(t, "bridge message", record.Body)
	require.Equal(t, slog.LevelError.String(), record.SeverityText)
	require.Equal(t, "01000000000000000000000000000000", record.TraceID)
	require.Equal(t, "0200000000000000", record.SpanID)
	require.Equal(t, "ctxVal", record.Attributes["ctxKey"])
	require.Equal(t, context.Canceled.Error(), record.Attributes[defaultErrorKey])
	require.Contains(t, record.Attributes, "trace_id")
	require.Contains(t, record.Attributes, "span_id")

	spans := spanRecorder.Ended()
	require.Len(t, spans, 1)
	require.Empty(
		t,
		spans[0].Events(),
		"log bridge should replace span.AddEvent to avoid duplicate telemetry",
	)
}

func TestOpenTelemetryBridgeDisabled(t *testing.T) {
	defer SetOpenTelemetryBridge(false)

	processor := new(captureProcessor)
	logProvider := sdklog.NewLoggerProvider(sdklog.WithProcessor(processor))
	logglobal.SetLoggerProvider(logProvider)
	SetOpenTelemetryBridge(false)

	log := ForTests()
	log.Info("plain message", String("key", "value"))

	require.Empty(t, processor.records)
}

func TestOpenTelemetryBridgeTransformSkipsWhenBridgeDisabled(t *testing.T) {
	defer SetOpenTelemetryBridge(false)

	prev := globalOpenTelemetryLogger
	defer func() { globalOpenTelemetryLogger = prev }()

	fake := &fakeOTelLogger{enabled: true}
	globalOpenTelemetryLogger = fake
	SetOpenTelemetryBridge(false)

	var record slog.Record
	record.Message = "skipped"
	record.Level = slog.LevelInfo

	out := newOpenTelemetryBridge().Transform(context.Background(), record)

	require.Equal(t, record.Message, out.Message)
	require.Equal(t, record.Level, out.Level)
	require.Zero(t, fake.emitted)
}

func TestOpenTelemetryBridgeLoggerDisabled(t *testing.T) {
	defer SetOpenTelemetryBridge(false)

	prev := globalOpenTelemetryLogger
	defer func() { globalOpenTelemetryLogger = prev }()

	globalOpenTelemetryLogger = &fakeOTelLogger{enabled: false}
	SetOpenTelemetryBridge(true)

	var record slog.Record
	record.Message = "disabled"
	record.Level = slog.LevelInfo

	out := newOpenTelemetryBridge().Transform(context.Background(), record)

	require.Equal(t, record.Message, out.Message)
	require.Equal(t, record.Level, out.Level)
}

func TestOpenTelemetryBridgeLoggerEnabled(t *testing.T) {
	defer SetOpenTelemetryBridge(false)

	prev := globalOpenTelemetryLogger
	defer func() { globalOpenTelemetryLogger = prev }()

	fake := &fakeOTelLogger{enabled: true}
	globalOpenTelemetryLogger = fake
	SetOpenTelemetryBridge(true)

	var record slog.Record
	record.Message = "enabled"
	record.Level = slog.LevelInfo

	newOpenTelemetryBridge().Transform(context.Background(), record)

	require.Equal(t, 1, fake.emitted)
}

func TestRecordError(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		var record slog.Record
		record.Add(Err(context.Canceled))

		attr, ok := recordError(record)

		require.True(t, ok)
		require.Equal(t, defaultErrorKey, attr.Key)
	})

	t.Run("not found", func(t *testing.T) {
		var record slog.Record
		record.Add(String("key", "value"))

		_, ok := recordError(record)

		require.False(t, ok)
	})
}

func TestSpanContextAttrs(t *testing.T) {
	require.Empty(t, spanContextAttrs(trace.SpanContext{}))
}

func TestToOTelSeverity(t *testing.T) {
	require.Equal(t, otellog.SeverityDebug, toOTelSeverity(slog.LevelDebug))
	require.Equal(t, otellog.SeverityInfo, toOTelSeverity(slog.LevelInfo))
	require.Equal(t, otellog.SeverityWarn, toOTelSeverity(slog.LevelWarn))
	require.Equal(t, otellog.SeverityError, toOTelSeverity(slog.LevelError))
	require.Equal(t, otellog.SeverityDebug, toOTelSeverity(slog.Level(-8)))
}

func TestToOTelScalarValue(t *testing.T) {
	now := time.Unix(123, 0).UTC()

	cases := []struct {
		name  string
		value slog.Value
		want  any
	}{
		{name: "string", value: slog.StringValue("value"), want: "value"},
		{name: "int64", value: slog.Int64Value(42), want: int64(42)},
		{name: "uint64", value: slog.Uint64Value(^uint64(0)), want: "18446744073709551615"},
		{name: "float64", value: slog.Float64Value(1.5), want: 1.5},
		{name: "bool", value: slog.BoolValue(true), want: true},
		{name: "duration", value: slog.DurationValue(time.Second), want: "1s"},
		{name: "time", value: slog.TimeValue(now), want: now.Format(time.RFC3339Nano)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value, ok := toOTelScalarValue(tc.value)

			require.True(t, ok)
			require.Equal(t, tc.want, otelValue(value))
		})
	}

	_, ok := toOTelScalarValue(slog.GroupValue())
	require.False(t, ok)
}

func TestToOTelValueUsesStringScalarPath(t *testing.T) {
	require.Equal(t, "value", otelValue(toOTelValue(slog.StringValue("value"))))
}

func TestToOTelCompositeValue(t *testing.T) {
	t.Run("group", func(t *testing.T) {
		value, ok := toOTelCompositeValue(slog.GroupValue(
			slog.String("key", "value"),
			slog.Int("num", 7),
		))

		require.True(t, ok)
		require.Equal(t, map[string]any{"key": "value", "num": int64(7)}, otelValue(value))
	})

	t.Run("log valuer", func(t *testing.T) {
		value, ok := toOTelCompositeValue(slog.AnyValue(stubLogValuer("wrapped")))

		require.True(t, ok)
		require.Equal(t, "wrapped", otelValue(value))
	})

	t.Run("any", func(t *testing.T) {
		value, ok := toOTelCompositeValue(slog.AnyValue([]any{"a", 1}))

		require.True(t, ok)
		require.Equal(t, []any{"a", "1"}, otelValue(value))
	})

	_, ok := toOTelCompositeValue(slog.StringValue("plain"))
	require.False(t, ok)
}

func TestAnyToOTelValue(t *testing.T) {
	require.Equal(t, "<nil>", otelValue(anyToOTelValue(nil)))
	require.Equal(t, "value", otelValue(anyToOTelValue("value")))
	require.Equal(t, []byte("bin"), otelValue(anyToOTelValue([]byte("bin"))))
	require.Equal(t, "stringer", otelValue(anyToOTelValue(stubStringer("stringer"))))
	require.Equal(t, "boom", otelValue(anyToOTelValue(fmt.Errorf("boom"))))
	require.Equal(t, []any{"x", "2"}, otelValue(anyToOTelValue([]any{"x", 2})))
	require.Equal(t, "123", otelValue(anyToOTelValue(123)))
}

func TestToOTelValueFallback(t *testing.T) {
	require.Equal(t, "<nil>", otelValue(toOTelValue(slog.Value{})))
	require.Equal(t, "<unknown slog.Kind>", otelValue(toOTelValue(unknownSlogValue())))
}

func unknownSlogValue() slog.Value {
	var value slog.Value

	field := reflect.ValueOf(&value).Elem().FieldByName("any")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).
		Elem().
		Set(reflect.ValueOf(slog.Kind(255)))

	return value
}
