package logger

import (
	"bytes"
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

func Test_NoneNamed(t *testing.T) {
	buf := new(bytes.Buffer)

	log := newLogger(slog.NewTextHandler(buf, &HandlerOptions{}))
	Named(log, "service").Info("hello world")

	require.NotContains(t, buf.String(), "[service]")
}
