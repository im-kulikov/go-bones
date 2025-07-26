package logger

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/im-kulikov/go-bones/config"
)

func TestTest(t *testing.T) {
	//
}

type stubIDGenerator int

func (s stubIDGenerator) NewIDs(context.Context) (trace.TraceID, trace.SpanID) {
	return trace.TraceID{0x01}, trace.SpanID{0x02}
}

func (s stubIDGenerator) NewSpanID(context.Context, trace.TraceID) trace.SpanID {
	return trace.SpanID{0x02}
}

// nolint:lll
const attributesLogExpected = `
level=INFO msg="group test" app.name=test-app-name app.version=test-app-version group.key=value
level=INFO msg=test1 app.name=test-app-name app.version=test-app-version
level=INFO msg=test2 app.name=test-app-name app.version=test-app-version
level=INFO msg=test3 app.name=test-app-name app.version=test-app-version error="test error"
level=INFO msg=test3 app.name=test-app-name app.version=test-app-version err="test error"
level=INFO msg="[service] message from some service" app.name=test-app-name app.version=test-app-version key=value
level=ERROR msg="tracing message" app.name=test-app-name app.version=test-app-version error="context canceled" ctxKey=ctxVal trace.span_id=0200000000000000 trace.trace_id=01000000000000000000000000000000
level=INFO msg="message with multi attributes" app.name=test-app-name app.version=test-app-version String=string-value Int64=9223372036854775807 Int=9223372036854775807 Uint64=18446744073709551615 Float64=1.7976931348623157e+308 Bool=true Time=1970-01-01T03:01:40.000+03:00 Duration=1s Any=val`

func attrsToMap(attributes []attribute.KeyValue) map[attribute.Key]any {
	out := make(map[attribute.Key]any, len(attributes))
	for _, attr := range attributes {
		out[attr.Key] = attr.Value.AsInterface()
	}

	return out
}

func Test_Logger(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(sr),
		sdktrace.WithIDGenerator(stubIDGenerator(1)))

	tracer := provider.Tracer("test")

	ctx, span := tracer.Start(context.TODO(), "main")

	buf := new(bytes.Buffer)
	log := ForTests(TestLoggerWriter(buf))

	log.WithGroup("group").Info("group test", String("key", "value"))

	log.InfoContext(context.TODO(), "test1", Err(nil))
	log.Info("test2", NamedError("err", nil))

	log.Info("test3", Err(fmt.Errorf("test error")))
	log.Info("test3", NamedError("err", fmt.Errorf("test error")))

	Named(log, "service").Info("message from some service", String("key", "value"))

	log.ErrorContext(
		AddContextAttrs(ctx, String("ctxKey", "ctxVal")),
		"tracing message",
		Err(context.Canceled),
	)

	log.With([]any{
		String("String", "string-value"),
		Int64("Int64", math.MaxInt64),
		Int("Int", math.MaxInt),
		Uint64("Uint64", math.MaxUint64),
		Float64("Float64", math.MaxFloat64),
		Bool("Bool", true),
		Time("Time", time.Unix(100, 0).In(time.FixedZone("Moscow", 3*60*60))),
		Duration("Duration", time.Second),
		Any("Any", "val"),
	}...).Info("message with multi attributes")

	out := buf.String()
	require.Equal(t, strings.TrimSpace(attributesLogExpected), strings.TrimSpace(out))

	span.End()
	spans := sr.Ended()
	require.Equal(t, 1, len(spans))

	events := spans[0].Events()
	require.Equal(t, 1, len(events))

	event := events[0]
	require.Equal(t, "log", event.Name)

	values := attrsToMap(event.Attributes)
	require.Contains(t, values, LogSeverityKey)
	require.Equal(t, slog.LevelError.String(), values[LogSeverityKey])

	require.Contains(t, values, LogMessageKey)
	require.Equal(t, "tracing message", values[LogMessageKey])

	require.Contains(t, values, semconv.CodeFunctionKey)
	require.Contains(t, values[semconv.CodeFunctionKey], "go-bones/logger.Test_Logger")

	require.Contains(t, values, semconv.CodeFilepathKey)
	require.Contains(t, values[semconv.CodeFilepathKey], "go-bones/logger/logger_test.go")
}

const testLoggerConfig = `
logger:
  open-tracing: true
  secrets: [secret, password]
`

func errGetter[K comparable](_ K, err error) error {
	return err
}

func TestWithConfig(t *testing.T) {
	var example struct {
		config.Base `yaml:",inline" env:",squash"`

		ConfigPath string `flag:"config,short:c,config:true"`
	}

	file, err := os.CreateTemp(t.TempDir(), "config.yaml")
	require.NoError(t, err)
	require.NoError(t, errGetter(file.WriteString(testLoggerConfig)))
	require.NoError(t, file.Close())

	require.NoError(t,
		config.Load(&example,
			config.WithName("test"),
			config.WithVersion("dev"),
			config.WithCustomizeLoaderConfig(func(c *gonfig.Config) {
				c.Args = append(c.Args, "--config", file.Name())
			})))
}

type testHandler struct {
	Handler
	mock.Mock
}

func (t *testHandler) WithAttrs(attrs []Attr) slog.Handler {
	t.Called(attrs)

	return t.Handler.WithAttrs(attrs)
}

func Test_applyHandler(t *testing.T) {
	t.Run("should call with attrs", func(t *testing.T) {
		var conf config.Logger
		conf.SetAppNameAndVersion("test-app-name", "test-app-version")

		handler := new(testHandler)
		handler.Test(t)
		handler.Handler = slog.NewTextHandler(io.Discard, &HandlerOptions{Level: slog.LevelDebug})

		handler.On(
			"WithAttrs",
			[]Attr{Group("app",
				String("name", conf.AppName()),
				String("version", conf.AppVersion()),
			)},
		).Once()

		New(conf, handler).Info("call with attrs")
	})

	t.Run("should call without attrs", func(t *testing.T) { // should do nothing
		var conf config.Logger

		handler := new(testHandler)
		handler.Test(t)
		handler.Handler = slog.NewTextHandler(io.Discard, &HandlerOptions{Level: slog.LevelDebug})

		New(conf, handler).Info("call without attrs")
	})
}
