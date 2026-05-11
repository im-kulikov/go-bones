package logger

import (
	"bytes"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Discard(t *testing.T) {
	require.NotPanics(t, func() {
		log := ForTests() // should use io.Discard
		log.Info("hello world")
	})
}

func Test_Named(t *testing.T) {
	buf := new(bytes.Buffer)
	out := io.MultiWriter(buf, &tbWriter{TB: t})

	log := ForTests(TestLoggerWriter(out), TestLoggerWriteToTB(t))
	Named(log, "service", "named", "").Info("hello world")

	require.Contains(t, buf.String(), "[service:named]")
	require.NotContains(t, buf.String(), "[service:named:]")
}

func Test_NoneNamed(t *testing.T) {
	buf := new(bytes.Buffer)
	out := io.MultiWriter(buf, &tbWriter{TB: t})

	log := newLogger(slog.NewTextHandler(out, &HandlerOptions{}))
	Named(log, "service").Info("hello world")

	require.NotContains(t, buf.String(), "[service]")
}
