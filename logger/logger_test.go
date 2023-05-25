package logger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"testing"

	"github.com/getsentry/sentry-go"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestSafeLevel(t *testing.T) {
	cases := []struct {
		name string

		level  string
		expect zapcore.Level
	}{
		{name: "expect InfoLevel", level: "info", expect: zapcore.InfoLevel},
		{name: "expect InfoLevel for unknown", level: "unknown", expect: zapcore.InfoLevel},
		{name: "expect DebugLevel", level: "debug", expect: zapcore.DebugLevel},
		{name: "expect WarnLevel", level: "warn", expect: zapcore.WarnLevel},
		{name: "expect ErrorLevel", level: "error", expect: zapcore.ErrorLevel},
		{name: "expect DPanicLevel", level: "dpanic", expect: zapcore.DPanicLevel},
		{name: "expect PanicLevel", level: "panic", expect: zapcore.PanicLevel},
		{name: "expect FatalLevel", level: "fatal", expect: zapcore.FatalLevel},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			actual := safeLevel(tt.level)
			require.Equal(t, tt.expect, actual)
		})
	}
}

func TestDefault(t *testing.T) {
	require.NotPanics(t, func() {
		Default().With("key", "val").Info("test")
	})
}

func TestForTests(t *testing.T) {
	require.NotPanics(t, func() {
		ForTests(t).With("key", "val").Info("test")
	})
}

func TestSugaredLogger(t *testing.T) {
	require.NotPanics(t, func() {
		require.IsType(t, &SugaredLogger{}, ForTests(t).Sugar())
	})
}

func TestWithCustomOutput(t *testing.T) {
	require.Panics(t, func() {
		_, err := New(Config{}, WithCustomOutput("unsupported_format", &fakeSink{Writer: io.Discard}))
		require.NoError(t, err)
	})
}

func TestConfig_Validate(t *testing.T) {
	var empty int
	cases := []struct {
		name   string
		config Config
		error  error
	}{
		{
			name: "valid config",
			config: Config{
				Level: zapcore.InfoLevel.String(),
				Trace: zapcore.FatalLevel.String(),
			},
		},
		{
			name: "fail for invalid sample rate",
			config: Config{
				Level:      zapcore.InfoLevel.String(),
				Trace:      zapcore.FatalLevel.String(),
				SampleRate: new(int),
			},
			error: validation.Errors{
				"SampleRate": (validation.ErrorObject{}).
					SetCode("validation_nil_or_not_empty_required").
					SetMessage("cannot be blank"),
			},
		},
		{
			name: "fail for empty sample rate",
			config: Config{
				Level:      zapcore.InfoLevel.String(),
				Trace:      zapcore.FatalLevel.String(),
				SampleRate: &empty,
			},
			error: validation.Errors{
				"SampleRate": (validation.ErrorObject{}).
					SetCode("validation_nil_or_not_empty_required").
					SetMessage("cannot be blank"),
			},
		},
		{
			name: "fail for invalid level value",
			config: Config{
				Level:      "unknown",
				Trace:      zapcore.FatalLevel.String(),
				SampleRate: &defaultSampleRate,
			},
			error: validation.Errors{
				"Level": (validation.ErrorObject{}).
					SetCode("validation_in_invalid").
					SetMessage("must be a valid value"),
			},
		},
		{
			name: "fail for invalid trace level value",
			config: Config{
				Trace:      "unknown",
				Level:      zapcore.FatalLevel.String(),
				SampleRate: &defaultSampleRate,
			},
			error: validation.Errors{
				"Trace": (validation.ErrorObject{}).
					SetCode("validation_in_invalid").
					SetMessage("must be a valid value"),
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.error, tt.config.Validate(context.Background()), tt.config.Validate(context.Background()))
		})
	}
}

func shouldFailOnBuildLogger() Option {
	return func(l *logger) {
		l.config.EncoderConfig.TimeKey = "should return an error"
		l.config.EncoderConfig.EncodeTime = nil
	}
}

type fakeSink struct{ io.Writer }

func (f *fakeSink) Close() error { return nil }

func (f *fakeSink) Sync() error { return nil }

func TestNew(t *testing.T) {
	cases := []struct {
		name   string
		config Config
		output string
		option []Option
		error  error
	}{
		{
			name: "should be ok",
			config: Config{
				Level: zapcore.InfoLevel.String(),
				Trace: zapcore.FatalLevel.String(),
			},

			output: `timestamp`,

			option: []Option{
				WithAppName("app-name"),
				WithAppVersion("app-version"),
				WithTimeKey("timestamp"),
				WithZapOption(zap.WithCaller(true)),
			},
		},

		{
			name: "should fail on build logger",
			config: Config{
				EncodingConsole: true,
				Level:           zapcore.InfoLevel.String(),
				Trace:           zapcore.FatalLevel.String(),
			},

			error:  errors.New("missing EncodeTime in EncoderConfig"),
			option: []Option{shouldFailOnBuildLogger(), WithConsoleColored()},
		},

		{
			name: "should fail on sentry invalid dsn",
			config: Config{
				EncodingConsole: true,
				Level:           zapcore.InfoLevel.String(),
				Trace:           zapcore.FatalLevel.String(),
			},

			option: []Option{WithSentry(Sentry{DSN: "unknown", Env: "unknown"})},
			error:  fmt.Errorf("sentry config: %w", &sentry.DsnParseError{Message: "invalid scheme"}),
		},

		{
			name: "should be ok with sentry zap.Hooks",
			config: Config{
				EncodingConsole: true,
				Level:           zapcore.InfoLevel.String(),
				Trace:           zapcore.FatalLevel.String(),
			},

			option: []Option{WithSentry(Sentry{DSN: "https://public@sentry.example.com/1", Env: "unknown"})},
		},
	}

	buf := new(bytes.Buffer)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			defer buf.Reset()

			// current, err := os.Getwd()
			// require.NoError(t, err)
			//
			// defer func() {
			// 	name := strings.ReplaceAll(tt.name, " ", "-")
			// 	require.NoError(t, os.Remove(path.Join(current, name)))
			// }()

			level := zap.NewAtomicLevelAt(zapcore.DebugLevel)
			re := regexp.MustCompile(`[^\w\-]`)
			name := re.ReplaceAllString(tt.name, "-")

			tt.option = append(tt.option,
				WithCustomOutput(name, &fakeSink{Writer: buf}),
				WithCustomLevel(level))

			log, err := New(tt.config, tt.option...)
			require.Equal(t, tt.error, err)

			if tt.error == nil {
				log.With("key", "val").Error("hello world")

				log.Std().Println("hello world")

				require.Contains(t, buf.String(), tt.output)
			}
		})
	}
}
