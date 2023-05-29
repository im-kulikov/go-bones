package config

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const renderedMarkdown = `### Envs

| Name                        | Required | Default value | Usage                                          | Example                           |
|-----------------------------|----------|---------------|------------------------------------------------|-----------------------------------|
| SHUTDOWN_TIMEOUT            | false    | 5s            | allows to set custom graceful shutdown timeout |                                   |
| OPS_DISABLE                 | false    | false         | allows to disable ops server                   |                                   |
| OPS_ADDRESS                 | false    | :8081         | allows to set set ops address:port             |                                   |
| OPS_NETWORK                 | false    | tcp           | allows to set ops listen network: tcp/udp      |                                   |
| OPS_NO_TRACE                | false    | true          | allows to disable tracing                      |                                   |
| OPS_METRICS_PATH            | false    | /metrics      | allows to set custom metrics path              |                                   |
| OPS_HEALTHY_PATH            | false    | /healthy      | allows to set custom healthy path              |                                   |
| OPS_PROFILE_PATH            | false    | /debug/pprof  | allows to set custom profiler path             |                                   |
| LOGGER_ENCODING_CONSOLE     | false    | false         | allows to set user-friendly formatting         |                                   |
| LOGGER_LEVEL                | false    | info          | allows to set custom logger level              |                                   |
| LOGGER_TRACE                | false    | fatal         | allows to set custom trace level               |                                   |
| LOGGER_SAMPLE_RATE          | false    | 1000          | allows to set sample rate                      |                                   |
| TRACER_TYPE                 | false    | jaeger        | allows to set trace exporter type              |                                   |
| TRACER_DISABLE              | false    | false         | allows to disable tracing                      |                                   |
| TRACER_SAMPLER              | false    | 1             | allows to choose sampler                       |                                   |
| TRACER_ENDPOINT             | false    |               | allows to set jaeger endpoint (one of)         | http://localhost:14268/api/traces |
| TRACER_AGENT_HOST           | false    |               | allows to set jaeger agent host (one of)       | localhost                         |
| TRACER_AGENT_PORT           | false    |               | allows to set jaeger agent port                | 6831                              |
| TRACER_AGENT_RETRY_INTERVAL | false    | 15s           | allows to set retry connection timeout         |                                   |`

func TestMarkdown(t *testing.T) {
	buf := new(bytes.Buffer)

	var cfg Base
	err := Load(context.Background(), &cfg,
		customOutput(buf),
		WithEnvs([]string{}),
		WithArgs([]string{"--markdown"}),
		customExit(func(code int) { require.Zero(t, code) }))

	require.EqualError(t, errors.Unwrap(err), errMarkdown.Error())

	require.Equal(t, renderedMarkdown, strings.TrimSpace(buf.String()))
}
