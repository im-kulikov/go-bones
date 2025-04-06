package logger

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
)

const testDefaultOutput = `
level=DEBUG msg="debug message" app.name=test-app-name app.version=test-app-version
level=INFO msg="info message" app.name=test-app-name app.version=test-app-version
level=WARN msg="warn message" app.name=test-app-name app.version=test-app-version
level=ERROR msg="error message" app.name=test-app-name app.version=test-app-version
`

func Test_default(t *testing.T) {
	var cfg config.Logger
	cfg.Secrets = append(cfg.Secrets, slog.TimeKey)
	cfg.SetAppNameAndVersion("test-app-name", "test-app-version")

	buf := bytes.NewBuffer(nil)
	Init(cfg, slog.NewTextHandler(buf, &HandlerOptions{Level: slog.LevelDebug}))

	buf.WriteString("\n")
	Debug(context.TODO(), "debug message")
	Info(context.TODO(), "info message")
	Warn(context.TODO(), "warn message")
	Error(context.TODO(), "error message")

	require.Equal(t, testDefaultOutput, buf.String())
}
