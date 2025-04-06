package logger

import (
	"bytes"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/im-kulikov/go-bones/config"
)

type testLogWriter struct {
	io.Writer
	sync.Mutex

	secrets []string
}

type tbWriter struct{ testing.TB }

// TestLoggerOption allows to customize test logger.
type TestLoggerOption func(*testLogWriter)

func (t tbWriter) Write(data []byte) (int, error) {
	t.Log(string(bytes.TrimSpace(data)))

	return len(data), nil
}

// Write log message to testing.TB.
func (t *testLogWriter) Write(data []byte) (int, error) {
	t.Lock()
	defer t.Unlock()

	return t.Writer.Write(data)
}

// TestLoggerWriter allows to add output to io.MultiWriter.
func TestLoggerWriter(out io.Writer) TestLoggerOption {
	return func(l *testLogWriter) { l.Writer = io.MultiWriter(l.Writer, out) }
}

// TestLoggerWriteToTB allows to write log message to testing.TB.
func TestLoggerWriteToTB(t testing.TB) TestLoggerOption {
	return func(l *testLogWriter) { l.Writer = io.MultiWriter(l.Writer, tbWriter{TB: t}) }
}

// TestLoggerSecrets allows to set secret fields.
func TestLoggerSecrets(secrets ...string) TestLoggerOption {
	return func(l *testLogWriter) { l.secrets = append(l.secrets, secrets...) }
}

// ForTests wrapped logger for tests.
func ForTests(options ...TestLoggerOption) *Logger {
	writer := &testLogWriter{Writer: io.Discard, secrets: []string{slog.TimeKey}}
	for _, option := range options {
		option(writer)
	}

	cfg := config.Logger{OpenTracingEnabled: true, Secrets: writer.secrets}
	cfg.SetAppNameAndVersion("test-app-name", "test-app-version")

	return New(cfg, slog.NewTextHandler(writer, &HandlerOptions{Level: slog.LevelDebug}))
}
