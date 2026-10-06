package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/require"
)

// A config with Base gets --config / -c and --print-config without fields of its own.
func TestBase_Flags(t *testing.T) {
	type settings struct {
		Base

		API HTTP `env:"API" yaml:"api"`
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("api: {address: 127.0.0.1:1}\n"), 0o600))

	var (
		cfg settings
		out bytes.Buffer
	)

	err := Load(&cfg,
		WithLoaderOptions(gonfig.WithCustomOutput(&out), gonfig.WithCustomExit(func(int) {})),
		WithCustomizeLoaderConfig(func(c *gonfig.Config) {
			c.Args, c.Envs = []string{"-c", path, "--print-config"}, []string{}
		}))
	require.ErrorIs(t, err, gonfig.ErrTestExit, "printed and exited")
	require.Contains(t, out.String(), "address: 127.0.0.1:1", "the file given with -c")
	require.Contains(t, out.String(), "\nops:\n", "the sections of Base")
}

type nestedSettings struct {
	Base Base
}

type pointerContainer struct {
	Base *Base
}

type appSettingsFixture struct {
	Base struct {
		OpsServer Ops
		Nested    nestedSettings
		Pointer   pointerContainer
	}
}

func Test_setAppSettings_error(t *testing.T) {
	require.ErrorIs(t,
		setAppSettings(Ops{}, "name", "test"),
		ErrPointerExpected)
}

func Test_setAppSettings_UsesSupportedShapesOnly(t *testing.T) {
	cfg := new(appSettingsFixture)

	require.NoError(t, setAppSettings(cfg, "name", "test"))

	require.Equal(t, "name", cfg.Base.OpsServer.AppName())
	require.Equal(t, "test", cfg.Base.OpsServer.AppVersion())

	require.Equal(t, "name", cfg.Base.Nested.Base.OpsServer.AppName())
	require.Equal(t, "test", cfg.Base.Nested.Base.OpsServer.AppVersion())

	require.Equal(t, "name", cfg.Base.Nested.Base.Tracer.AppName())
	require.Equal(t, "test", cfg.Base.Nested.Base.Tracer.AppVersion())

	require.Nil(
		t,
		cfg.Base.Pointer.Base,
		"pointer fields are intentionally ignored by setAppSettings",
	)
}

func TestBase_BonesIsPromoted(t *testing.T) {
	var app struct{ Base }

	require.Same(t, &app.Base, app.Bones())
}
