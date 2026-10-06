package logger

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
)

type testTransformer int

func (t testTransformer) Transform(_ context.Context, record Record) Record {
	record.Add(Any("test-transformer", t))

	return record
}

const testDefaultOutput = `
level=DEBUG msg="debug message" app.name=test-app-name app.version=test-app-version test-transformer=1
level=INFO msg="info message" app.name=test-app-name app.version=test-app-version test-transformer=1
level=WARN msg="warn message" app.name=test-app-name app.version=test-app-version test-transformer=1
level=ERROR msg="error message" app.name=test-app-name app.version=test-app-version test-transformer=1
`

func Test_default(t *testing.T) {
	var cfg config.Logger
	require.NoError(t, config.Load(&cfg))

	require.NotEmpty(t, Default())

	cfg.Format = "text" // the expected output is in the text format
	cfg.AddAppInfo = true
	cfg.Secrets = append(cfg.Secrets, slog.TimeKey)
	cfg.SetAppNameAndVersion("test-app-name", "test-app-version")

	buf := bytes.NewBuffer(nil)
	Init(cfg,
		WithOutput(buf),
		WithLevel(slog.LevelDebug),
		WithTransformers(testTransformer(1)))

	buf.WriteString("\n")
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	require.Equal(t, testDefaultOutput, buf.String())
}

func Test_defaultWithDefaultLevel(t *testing.T) {
	var cfg config.Logger
	cfg.Format = "text" // the expected output is in the text format
	cfg.AddAppInfo = true
	cfg.Secrets = append(cfg.Secrets, slog.TimeKey)
	cfg.SetAppNameAndVersion("test-app-name", "test-app-version")

	buf := bytes.NewBuffer(nil)
	Init(cfg,
		WithOutput(buf),
		WithTransformers(testTransformer(1)))

	buf.WriteString("\n")
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	expect := make([]string, 0, 5)
	for _, line := range strings.Split(testDefaultOutput, "\n") {
		if strings.Contains(line, "debug message") {
			continue
		}

		expect = append(expect, line)
	}

	require.Equal(t, strings.Join(expect, "\n"), buf.String())
}

func Test_WithHandler(t *testing.T) {
	var cfg config.Logger
	cfg.Format = "text" // the expected output is in the text format
	cfg.AddAppInfo = true
	cfg.Secrets = append(cfg.Secrets, slog.TimeKey)
	cfg.SetAppNameAndVersion("test-app-name", "test-app-version")

	buf := bytes.NewBuffer(nil)
	Init(cfg,
		WithTransformers(testTransformer(1)),
		WithHandler(slog.NewTextHandler(buf, &HandlerOptions{
			Level: slog.LevelDebug,
		})))

	buf.WriteString("\n")
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	require.Equal(t, testDefaultOutput, buf.String())
}

func TestLogger_fromConfig(t *testing.T) {
	t.Run("could not parse logger.level", func(t *testing.T) {
		buf := new(bytes.Buffer)
		def := defaultLogger.Load()
		defaultLogger.Store(slog.New(slog.NewTextHandler(buf, nil)))

		defer func() { defaultLogger.Store(def) }()

		var cfg config.Logger
		require.NoError(t, config.Load(&cfg))

		cfg.Level = "unknown"

		for range optionsFromConfig(cfg, nil) {
		}

		require.Contains(t, buf.String(), "could not parse logger.level")
	})

	t.Run("could not parse logger.format", func(t *testing.T) {
		buf := new(bytes.Buffer)
		def := defaultLogger.Load()
		defaultLogger.Store(slog.New(slog.NewTextHandler(buf, nil)))

		defer func() { defaultLogger.Store(def) }()

		var cfg config.Logger
		require.NoError(t, config.Load(&cfg))

		cfg.Format = "unknown"

		for range optionsFromConfig(cfg, nil) {
		}

		require.Contains(t, buf.String(), "could not parse logger.format")
	})

	t.Run("break on", func(t *testing.T) {
		var cfg config.Logger
		require.NoError(t, config.Load(&cfg))

		cfg.Format = "json" // set JSON formatter, for example

		buf := new(bytes.Buffer)
		cnt := len(slices.Collect(optionsFromConfig(cfg, []Option{WithOutput(buf)})))
		for i := range cnt {
			var idx int
			for range optionsFromConfig(cfg, []Option{WithOutput(buf)}) {
				if idx == i {
					break
				}

				idx++
			}

			require.NotPanics(t, func() { Init(cfg) })
		}
	})
}

func TestRegisterFormat(t *testing.T) {
	var built bool
	ctor := func(w io.Writer, o *HandlerOptions) Handler {
		built = true

		return slog.NewTextHandler(w, o)
	}

	RegisterFormat("test-registered", ctor)
	t.Cleanup(func() { formats.Delete("test-registered") })
	formatFor("test-registered")(io.Discard, nil)
	require.True(t, built, "a registered format is picked by name")

	require.Panics(t, func() { RegisterFormat("test-registered", ctor) }, "twice")
	require.Panics(t, func() { RegisterFormat("journal", ctor) }, "built-in")
	require.Panics(t, func() { RegisterFormat("test-nil", nil) }, "nil")
}
