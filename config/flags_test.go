package config

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var renderedHelp = `Usage:

  -V, --version    show current version
  -h, --help       show this help message
      --markdown   generate env markdown table
      --validate   validate config

Default envs:

SHUTDOWN_TIMEOUT=5s                               # allows to set custom graceful shutdown timeout
OPS_ENABLED=false                                 # allows to enable ops server
OPS_ADDRESS=:8081                                 # allows to set set ops address:port
OPS_NETWORK=tcp                                   # allows to set ops listen network: tcp/udp
OPS_NO_TRACE=true                                 # allows to disable tracing
OPS_METRICS_PATH=/metrics                         # allows to set custom metrics path
OPS_HEALTHY_PATH=/healthy                         # allows to set custom healthy path
OPS_PROFILE_PATH=/debug/pprof                     # allows to set custom profiler path
LOGGER_ENCODING_CONSOLE=false                     # allows to set user-friendly formatting
LOGGER_LEVEL=info                                 # allows to set custom logger level
LOGGER_TRACE=fatal                                # allows to set custom trace level
LOGGER_SAMPLE_RATE=1000                           # allows to set sample rate
TRACER_TYPE=jaeger                                # allows to set trace exporter type
TRACER_ENABLED=false                              # allows to enable tracing
TRACER_SAMPLER=1                                  # allows to choose sampler
TRACER_ENDPOINT=<empty>                           # allows to set jaeger endpoint (one of)
TRACER_AGENT_HOST=<empty>                         # allows to set jaeger agent host (one of)
TRACER_AGENT_PORT=<empty>                         # allows to set jaeger agent port
TRACER_AGENT_RETRY_INTERVAL=15s                   # allows to set retry connection timeout`

func TestFlags(t *testing.T) {
	buf := new(bytes.Buffer)

	var cfg Base
	err := Load(context.Background(), &cfg,
		customOutput(buf),
		WithEnvs([]string{}),
		WithArgs([]string{"--help"}),
		customExit(func(code int) { require.Zero(t, code) }))

	require.EqualError(t, errors.Unwrap(err), errShowHelp.Error())

	require.Equal(t, renderedHelp, strings.TrimSpace(buf.String()))
}
