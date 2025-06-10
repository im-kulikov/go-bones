package logger

import (
	"bytes"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/im-kulikov/go-bones/config"
)

// testLogWriter is an internal structure providing concurrency-safe writing of log output.
// It holds a list of secret fields to redact from log records and can write to multiple destinations.
type testLogWriter struct {
	io.Writer
	sync.Mutex

	secrets []string
}

// tbWriter wraps testing.TB to allow writing log data directly to test logs.
type tbWriter struct {
	testing.TB
	sync.Mutex
}

// TestLoggerOption defines a functional option for configuring testing log behavior.
type TestLoggerOption func(*testLogWriter)

// Write writes log data to the provided testing.TB, trimming trailing whitespace.
//
// This method satisfies the io.Writer interface.
func (t *tbWriter) Write(data []byte) (int, error) {
	t.Lock()
	defer t.Unlock()

	if !t.Failed() {
		t.Log(string(bytes.TrimSpace(data)))
	}

	return len(data), nil
}

// Write safely writes log data to the underlying writer, ensuring concurrency safety.
//
// This method satisfies the io.Writer interface.
func (t *testLogWriter) Write(data []byte) (int, error) {
	t.Lock()
	defer t.Unlock()

	return t.Writer.Write(data)
}

// TestLoggerWriter configures the test logger to also write outputs to the specified io.Writer.
//
// Parameters:
//   - out: The additional destination for log data.
//
// Returns:
//   - A TestLoggerOption that appends the given writer to the log output.
func TestLoggerWriter(out io.Writer) TestLoggerOption {
	return func(l *testLogWriter) { l.Writer = io.MultiWriter(l.Writer, out) }
}

// TestLoggerWriteToTB configures the test logger to write messages into the provided testing.TB.
//
// Parameters:
//   - t: The test interface to write logs to.
//
// Returns:
//   - A TestLoggerOption that appends the testing.TB output to the log output.
func TestLoggerWriteToTB(t testing.TB) TestLoggerOption {
	return func(l *testLogWriter) { l.Writer = io.MultiWriter(l.Writer, &tbWriter{TB: t}) }
}

// TestLoggerSecrets allows the test logger to redact specified secret fields.
//
// Parameters:
//   - secrets: One or more field names to mask with "REDACTED" in log output.
//
// Returns:
//   - A TestLoggerOption that updates the internal list of secret fields.
func TestLoggerSecrets(secrets ...string) TestLoggerOption {
	return func(l *testLogWriter) { l.secrets = append(l.secrets, secrets...) }
}

// ForTests returns a new logger instance, preconfigured for testing.
// It applies all provided TestLoggerOptions, sets up default secrets,
// and enables tracing by default.
//
// Parameters:
//   - options: Zero or more TestLoggerOption values that customize test logger behavior.
//
// Returns:
//   - A pointer to a Logger preconfigured for use in tests.
func ForTests(options ...TestLoggerOption) *Logger {
	writer := &testLogWriter{Writer: io.Discard, secrets: []string{slog.TimeKey}}
	for _, option := range options {
		option(writer)
	}

	cfg := config.Logger{
		OpenTracingEnabled: true,
		Secrets:            writer.secrets,
	}
	cfg.SetAppNameAndVersion("test-app-name", "test-app-version")

	return New(cfg, slog.NewTextHandler(writer, &HandlerOptions{Level: slog.LevelDebug}))
}
