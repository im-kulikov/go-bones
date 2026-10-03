package config

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/require"
)

func TestServerSections(t *testing.T) {
	var _ INetwork = HTTP{}
	var _ INetwork = GRPC{}

	h, g := Defaults[HTTP](), Defaults[GRPC]()
	require.Equal(t, ":8080", h.Addr())
	require.Equal(t, ":9090", g.Addr())
	require.Equal(t, 30*time.Second, h.Base().ShutdownTimeout)
}

// The embedded Network of a section needs no inline or squash tags: gonfig
// inlines embedded structs in every source.
func TestServerSections_EmbeddedNetwork(t *testing.T) {
	type settings struct {
		DefaultConfigFlag

		API HTTP `env:"API" yaml:"api" json:"api" toml:"api"`
		Ops Ops  `env:"OPS" yaml:"ops" json:"ops" toml:"ops"`
	}

	check := func(t *testing.T, args, envs []string, opts ...Option) {
		t.Helper()

		var cfg settings

		setArgs := WithCustomizeLoaderConfig(func(c *gonfig.Config) { c.Args, c.Envs = args, envs })
		require.NoError(t, Load(&cfg, append(opts, setArgs)...))
		require.Equal(t, ":1", cfg.API.Address)
		require.Equal(t, 3*time.Second, cfg.API.ReadTimeout)
		require.Equal(t, ":2", cfg.Ops.Address)
		require.Equal(t, 4*time.Second, cfg.Ops.IdleTimeout)
		require.Equal(t, 30*time.Second, cfg.Ops.ShutdownTimeout, "defaults of the embedded struct")
	}

	files := map[string]struct {
		body string
		opt  Option
	}{
		"yaml": {
			"api: {address: ':1', read_timeout: 3s}\nops: {address: ':2', idle_timeout: 4s}\n",
			WithYAML(),
		},
		"json": {
			`{"api": {"address": ":1", "readTimeout": "3s"}, "ops": {"address": ":2", "idleTimeout": "4s"}}`,
			WithJSON(),
		},
		"toml": {
			"[api]\naddress = ':1'\nread_timeout = '3s'\n[ops]\naddress = ':2'\nidle_timeout = '4s'\n",
			WithTOML(),
		},
	}

	for name, file := range files {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config."+name)
			require.NoError(t, os.WriteFile(path, []byte(file.body), 0o600))
			check(t, []string{"--config", path}, []string{}, file.opt)
		})
	}

	t.Run("env", func(t *testing.T) {
		check(
			t,
			[]string{},
			[]string{
				"API_ADDRESS=:1",
				"API_READ_TIMEOUT=3s",
				"OPS_ADDRESS=:2",
				"OPS_IDLE_TIMEOUT=4s",
			},
		)
	})
}

// A TLS section created by the environment starts from its default tags; a
// variable that sets none of its fields does not create it.
func TestServerSections_TLSFromEnv(t *testing.T) {
	load := func(envs ...string) *TLS {
		t.Helper()

		var cfg struct {
			API HTTP `env:"API"`
		}

		require.NoError(t, Load(&cfg, WithCustomizeLoaderConfig(func(c *gonfig.Config) {
			c.Args, c.Envs = []string{}, envs
		})))

		return cfg.API.TLSConfig
	}

	tls := load("API_TLS_CERT_FILE=/tls/crt")
	require.NotNil(t, tls)
	require.Equal(t, "/tls/crt", tls.CertFile)
	require.Equal(t, "TLS13", tls.MinVersion)
	require.Equal(t, "no-client-cert", tls.ClientAuth)

	require.Nil(t, load("API_TLS_OTHER=x"), "not configured")
}

func TestBuildVersion(t *testing.T) {
	info := func(version string, settings ...debug.BuildSetting) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: version}, Settings: settings}
	}
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "0123456789abcdef0123"}
	clean := debug.BuildSetting{Key: "vcs.modified", Value: "false"}
	dirty := debug.BuildSetting{Key: "vcs.modified", Value: "true"}

	require.Equal(t, "v1.2.3", buildVersion(info("v1.2.3", rev)), "module version wins")
	require.Equal(t, "0123456789ab", buildVersion(info("(devel)", rev, clean)))
	require.Equal(t, "0123456789ab-dirty", buildVersion(info("(devel)", rev, dirty)))
	require.Equal(
		t,
		"abc",
		buildVersion(info("", debug.BuildSetting{Key: "vcs.revision", Value: "abc"})),
	)
	require.Equal(t, "(devel)", buildVersion(info("(devel)")), "no VCS info")
}
