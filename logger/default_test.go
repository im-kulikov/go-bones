package logger

import (
	"bytes"
	"context"
	"log/slog"
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
