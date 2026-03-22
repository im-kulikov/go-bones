package logger

import (
	"bytes"
	"fmt"
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

	secrets []string
}

// tbWriter wraps testing.TB to allow writing log data directly to test logs.
type tbWriter struct {
	testing.TB
	sync.Mutex
}

// syncWriter is a concurrency-safe wrapper around an io.Writer that uses a mutex to synchronize write operations.
type syncWriter struct {
	io.Writer
	sync.Mutex
}

// TestLoggerOption defines a functional option for configuring testing log behavior.
type TestLoggerOption func(*testLogWriter)

// syncBytesBuffer is a thread-safe implementation of bytes.Buffer using a sync.Mutex
// for concurrent access control.
type syncBytesBuffer struct {
	sync.Mutex
	bytes.Buffer
}

// SyncBuffer is an interface for a thread-safe buffer that supports writing,
// string conversion, and byte slice retrieval.
type SyncBuffer interface {
	io.Writer
	fmt.Stringer
	Bytes() []byte
}

// Write writes the provided byte slice to the buffer in a thread-safe manner
// and returns the number of bytes written and an error.
func (s *syncBytesBuffer) Write(data []byte) (int, error) {
	s.Lock()
	defer s.Unlock()

	return s.Buffer.Write(data)
}

// String returns the contents of the buffer as a string in a thread-safe manner.
func (s *syncBytesBuffer) String() string {
	s.Lock()
	defer s.Unlock()

	return s.Buffer.String()
}

// Bytes return a copy of the unread portion of the buffer's data in a thread-safe manner.
func (s *syncBytesBuffer) Bytes() []byte {
	s.Lock()
	defer s.Unlock()

	buf := s.Buffer.Bytes()
	out := make([]byte, len(buf))
	copy(out, buf)

	return out
}

// NewSyncBuffer creates a new instance of a thread-safe SyncBuffer backed
// by a syncBytesBuffer.
func NewSyncBuffer() SyncBuffer {
	return new(syncBytesBuffer)
}

// NewSyncWriter wraps an io.Writer with a mutex to ensure safe concurrent access
// and returns the concurrency-safe writer.
func NewSyncWriter(rw io.Writer) io.Writer {
	return new(syncWriter{Writer: rw})
}

// Write writes log data to the provided testing.TB output stream.
//
// This method satisfies the io.Writer interface.
func (t *tbWriter) Write(data []byte) (int, error) {
	t.Lock()
	defer t.Unlock()

	t.Helper()

	select {
	case <-t.Context().Done():
		return len(data), nil
	default:
		_, err := t.Output().Write(data)
		return len(data), err
	}
}

// Write safely writes log data to the underlying writer, ensuring concurrency safety.
//
// This method satisfies the io.Writer interface.
func (t *syncWriter) Write(data []byte) (int, error) {
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
	return func(l *testLogWriter) { l.Writer = NewSyncWriter(io.MultiWriter(l.Writer, out)) }
}

// TestLoggerWriteToTB configures the test logger to write messages into the provided testing.TB.
//
// Parameters:
//   - t: The test interface to write logs to.
//
// Returns:
//   - A TestLoggerOption that appends the testing.TB output to the log output.
func TestLoggerWriteToTB(t testing.TB) TestLoggerOption {
	return func(l *testLogWriter) { l.Writer = NewSyncWriter(io.MultiWriter(l.Writer, &tbWriter{TB: t})) }
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
	writer := &testLogWriter{
		Writer:  NewSyncWriter(io.Discard),
		secrets: []string{slog.TimeKey, "Time"},
	}
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
