package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cristalhq/aconfig"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/tracer"
	"github.com/im-kulikov/go-bones/web"
)

var defaultSampleRate = 1000

func TestFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	t.Run("should fail for non pointer", func(t *testing.T) {
		var cfg interface {
			Validate(context.Context) error
		}

		require.EqualError(t, Load(ctx, cfg, WithArgs(nil)), "config variable must be a pointer")
	})
}

func customOutput(w io.Writer) Option {
	return func(c *config) { c.out = w }
}

func customExit(v func(int)) Option {
	return func(c *config) { c.exit = v }
}

func generateDefaultHelp(t *testing.T) string {
	var cfg Base

	buf := new(bytes.Buffer)

	loader := aconfig.LoaderFor(&cfg, aconfig.Config{
		AllowUnknownFields: true,
		SkipFlags:          true,
		Args:               []string{},
	})

	require.NoError(t, loader.Load())

	flags := loader.Flags()
	flags.SetOutput(buf)

	c := config{out: buf}
	c.attachFlags(flags)
	c.renderHelp(loader, flags)

	return strings.TrimSpace(buf.String())
}

func generateUnknownFlagMessage(t *testing.T) string {
	return fmt.Sprintf("flag provided but not defined: -unknown-flag\n%s", generateDefaultHelp(t))
}

func customPWD() Option {
	return func(c *config) { c.pwd = func() (string, error) { return "", errors.New("mocked pwd") } }
}

func customFatalf(t *testing.T) Option {
	return func(c *config) {
		c.fatalf = func(s string, i ...interface{}) {
			assert.Len(t, i, 1)
			assert.IsType(t, validation.Errors{}, i[0])
		}
	}
}

func TestNew(t *testing.T) {
	cases := []struct {
		name string
		args []string
		envs []string
		opts []Option
		code int

		expect Base

		out string
		err error
	}{
		{
			name: "should be ok",
			opts: []Option{WithEnvPath("")},

			expect: Base{
				Shutdown: time.Second * 5,
				Sentry:   logger.Sentry{},
				Logger:   logger.Config{Level: "info", Trace: "fatal", SampleRate: &defaultSampleRate},
				Tracer:   tracer.Config{Type: "jaeger", Jaeger: tracer.Jaeger{Sampler: 1, RetryInterval: time.Second * 15}},

				Ops: web.OpsConfig{
					Address: ":8081",
					Network: "tcp",
					NoTrace: true,

					MetricsPath: "/metrics",
					HealthyPath: "/healthy",
					ProfilePath: "/debug/pprof",
				},
			},
		},
		{
			name: "should be ok when pass valid env path",
			opts: []Option{WithEnvPath("./")},

			expect: Base{
				Shutdown: time.Second * 5,
				Sentry:   logger.Sentry{},
				Logger:   logger.Config{Level: "info", Trace: "fatal", SampleRate: &defaultSampleRate},
				Tracer:   tracer.Config{Type: "jaeger", Jaeger: tracer.Jaeger{Sampler: 1, RetryInterval: time.Second * 15}},

				Ops: web.OpsConfig{
					Address: ":8081",
					Network: "tcp",
					NoTrace: true,

					MetricsPath: "/metrics",
					HealthyPath: "/healthy",
					ProfilePath: "/debug/pprof",
				},
			},
		},
		{
			name: "should validate",
			args: []string{"--validate"},
			code: 0,

			out: "OK",
			err: fmt.Errorf("could not load config: %w", errValidate),
		},
		{
			name: "should show version",
			args: []string{"-V"},
			code: 0,

			out: "vX.Y.Z",
			err: fmt.Errorf("could not load config: %w", errVersion),
		},
		{
			name: "should show version",
			args: []string{"--version"},
			code: 0,

			out: "vX.Y.Z",
			err: fmt.Errorf("could not load config: %w", errVersion),
		},
		{
			name: "should show help",
			args: []string{"-h"},
			code: 0,

			out: generateDefaultHelp(t),
			err: fmt.Errorf("could not load config: %w", errShowHelp),
		},
		{
			name: "should show help",
			args: []string{"--help"},
			code: 0,

			out: generateDefaultHelp(t),
			err: fmt.Errorf("could not load config: %w", errShowHelp),
		},

		{
			name: "should fail for flag parse",
			args: []string{"--unknown-flag"},
			code: 1,

			out: generateUnknownFlagMessage(t),
			err: fmt.Errorf("could not load config: %w",
				fmt.Errorf("could not parse flags: %w",
					fmt.Errorf("flag provided but not defined: -unknown-flag"))),
		},

		{
			name: "should fail on set env",
			opts: []Option{customPWD()},
			code: 1,

			err: fmt.Errorf("could not load config: %w",
				fmt.Errorf("could not get current directory: %w",
					errors.New("mocked pwd"))),
		},

		{
			name: "should fail on validate",
			args: []string{"--validate"},
			envs: []string{"LOGGER_SAMPLE_RATE=0"},
			code: 2,

			err: fmt.Errorf("could not load config: %w", errFailValidate),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	buf := new(bytes.Buffer)
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				var actual Base

				defer buf.Reset()

				require.NoError(t, os.Setenv("APP_NAME", "test-app"))

				tt.opts = append(tt.opts,
					customFatalf(t),
					WithArgs(tt.args),
					WithEnvs(tt.envs),
					customOutput(buf),
					WithVersion("vX.Y.Z"),
					customExit(func(exit int) { assert.Equal(t, tt.code, exit) }))

				require.Equal(t, tt.err, Load(ctx, &actual, tt.opts...))
				require.Equal(t, tt.out, strings.TrimSpace(buf.String()))

				if tt.err != nil {
					return
				}

				require.Equal(t, tt.expect, actual)
			})
		})
	}
}
