package logger

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type stubTB struct {
	testing.TB
	mock.Mock
}

func (t *stubTB) Log(args ...any) { t.Called(args...) }

func (t *stubTB) Context() context.Context {
	if t.TB == nil {
		return nil
	}

	ctx, cancel := context.WithCancel(t.TB.Context())
	t.Cleanup(cancel)

	return ctx
}

type canceledTB struct {
	testing.TB

	ctx          context.Context
	outputCalled bool
}

func (t *canceledTB) Helper() {}

func (t *canceledTB) Context() context.Context { return t.ctx }

func (t *canceledTB) Output() io.Writer {
	t.outputCalled = true

	return io.Discard
}

func Test_ForTest(t *testing.T) {
	tb := &stubTB{TB: t}
	tb.Test(t)

	tb.On("Log",
		`level=INFO msg="hello world" app.name=test-app-name app.version=test-app-version`).Once()

	log := ForTests(TestLoggerWriteToTB(tb))
	log.Info("hello world")
}

func TestTBWriterSkipsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tb := &canceledTB{ctx: ctx}
	data := []byte("ignored log line")

	n, err := (&tbWriter{TB: tb}).Write(data)
	require.NoError(t, err)
	require.Equal(t, len(data), n)
	require.False(t, tb.outputCalled)
}

func Test_syncBuffer(t *testing.T) {
	buf := NewSyncBuffer()
	log := ForTests(TestLoggerWriter(buf))
	log.InfoContext(t.Context(), "some message")
	require.Contains(t, buf.String(), "some message")
	require.Contains(t, string(buf.Bytes()), "some message")
}
