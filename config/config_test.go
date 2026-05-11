package config

import (
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/require"
)

type TestConfig struct {
	Base `yaml:",inline" toml:",inline" json:",inline" env:",squash"`

	Config string `flag:"config,config:true"`
}

type testCase struct {
	name string
	body string
	opts Option
}

type testParser int

type testWriter struct {
	*testing.T
	sync.Mutex
}

const (
	exampleConfigYAML = `---
logger:
  open_tracing: true
  secrets: ["test"]
`
	exampleFailConfigYAML = `---
logger;
  open_tracing: true
  secrets: ["test"]
`

	exampleConfigTOML = `
[logger]
open_tracing = true
secrets = [ "test" ]
`
	exampleConfigJSON = `{
	"logger": {
		"open_tracing": true,
		"secrets": ["test"]
	}
}`
)

func (t *testWriter) Write(p []byte) (n int, err error) {
	t.Lock()
	defer t.Unlock()
	t.Logf("%s", p)

	return len(p), nil
}

func (t *testParser) Load(any) error { return nil }

func (t *testParser) Type() gonfig.ParserType { return "test" }

func fetchError(args ...any) error {
	if len(args) == 0 {
		return nil
	}

	if err, ok := args[len(args)-1].(error); ok {
		return err
	}

	return nil
}

func validateConfig(t *testing.T, c testCase, handle func(cfg TestConfig, err error)) {
	tmp, err := os.CreateTemp(t.TempDir(), "*.config")
	require.NoError(t, err)
	require.NoError(t, fetchError(strings.NewReader(c.body).WriteTo(tmp)))
	require.NoError(t, tmp.Close())

	var cfg TestConfig
	err = Load(&cfg,
		c.opts,
		WithName("name"),
		WithVersion("test"),
		WithParsers(new(testParser(1))),
		WithParserInit(func(c gonfig.Config) (gonfig.Parser, error) {
			var p testParser

			return &p, nil
		}),
		WithLoaderOptions(gonfig.WithCustomOutput(&testWriter{T: t})),
		WithCustomizeLoaderConfig(func(c *gonfig.Config) {
			c.Args = append(c.Args, "--config", tmp.Name())
		}))
	handle(cfg, err)
}

func Test_config(t *testing.T) {
	cases := []testCase{
		{name: "json", body: exampleConfigJSON, opts: WithJSON()},
		{name: "yaml", body: exampleConfigYAML, opts: WithYAML()},
		{name: "default", body: exampleConfigYAML, opts: func(*settings) {}},
		{name: "toml", body: exampleConfigTOML, opts: WithTOML()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			validateConfig(t, c, func(cfg TestConfig, err error) {
				require.NoError(t, err)

				require.Equal(t, "name", cfg.OpsServer.AppName())
				require.Equal(t, "test", cfg.OpsServer.AppVersion())

				require.Equal(t, "name", cfg.Logger.name)
				require.Equal(t, "test", cfg.Logger.version)

				require.Equal(t, "name", cfg.Tracer.AppName())
				require.Equal(t, "test", cfg.Tracer.AppVersion())

				require.True(t, cfg.Logger.OpenTracingEnabled)
				require.Equal(t, []string{"test"}, cfg.Logger.Secrets)
				require.NoError(t, new(slog.Level).UnmarshalText([]byte(cfg.Logger.Level)))
			})
		})
	}

	t.Run("should fail", func(t *testing.T) {
		validateConfig(t,
			testCase{
				body: exampleFailConfigYAML,
				opts: func(*settings) {},
			}, func(cfg TestConfig, err error) { require.ErrorIs(t, err, gonfig.ErrCantParse) })
	})
}
