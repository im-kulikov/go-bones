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

func Test_NamedNilUsesDefault(t *testing.T) {
	require.NotPanics(t, func() {
		Named(nil, "service").Info("hello world")
	})
}

func Test_NamedDoesNotAliasParentNames(t *testing.T) {
	buf := new(bytes.Buffer)
	base := ForTests(TestLoggerWriter(buf)).Handler().(*wrappedHandler)

	// Spare capacity would let a plain append share the parent's backing array.
	names := make([]string, 1, 4)
	names[0] = "parent"
	parent := &wrappedHandler{conf: base.conf, next: base.next, list: base.list, name: names}

	first := newLogger(parent.Named("first"))
	newLogger(parent.Named("second"))

	first.Info("hello world")
	require.Contains(t, buf.String(), "[parent:first] hello world")
}

func Test_WithAttrsKeepsNames(t *testing.T) {
	buf := new(bytes.Buffer)
	log := Named(ForTests(TestLoggerWriter(buf)), "svc")

	log.With("k", "v").Info("with")
	log.WithGroup("g").Info("group")

	require.Contains(t, buf.String(), "[svc] with")
	require.Contains(t, buf.String(), "[svc] group")
}
