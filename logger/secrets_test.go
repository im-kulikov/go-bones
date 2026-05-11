package logger

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_secretTransformer(t *testing.T) {
	buf := new(bytes.Buffer)
	log := ForTests(TestLoggerWriter(buf), TestLoggerSecrets("my-password"))
	log.Info("hello world", String("my-password", "should be hidden"))

	require.NotContains(t, buf.String(), "should be hidden")
	require.Contains(t, buf.String(), "REDACTED")
}
