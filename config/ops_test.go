package config

import (
	"net"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/internal/testutil"
)

func Test_OpsSettings(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	var cfg Ops
	cfg.Address = lis.Addr().String()
	cfg.ShutdownTimeout = time.Nanosecond

	// check that empty TLS config returns an error
	require.ErrorIs(t, fetchError(cfg.PrepareTLSConfig()), ErrTLSDisabled)

	require.Equal(t, lis.Addr().String(), cfg.Addr())
	require.IsType(t, Network{}, cfg.Base())

	// setup default TLS config
	cfg.TLSConfig = new(TLS)

	require.NoError(t, gonfig.SetDefaults(cfg.TLSConfig))
	cfg.TLSConfig.Enabled = true
	cfg.TLSConfig.KeyFile, cfg.TLSConfig.CertFile = generateTLSKeyPair(t)

	_, err = cfg.PrepareTLSConfig()
	require.NoError(t, err)
}

func TestOps_IsEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  Ops
		want bool
	}{
		{
			name: "disabled",
			cfg: Ops{
				Enabled:        false,
				MetricsEnabled: true,
			},
			want: false,
		},
		{
			name: "enabled_without_endpoints",
			cfg: Ops{
				Enabled: true,
			},
			want: false,
		},
		{
			name: "metrics_enabled",
			cfg: Ops{
				Enabled:        true,
				MetricsEnabled: true,
			},
			want: true,
		},
		{
			name: "profile_enabled",
			cfg: Ops{
				Enabled:        true,
				ProfileEnabled: true,
			},
			want: true,
		},
		{
			name: "exp_vars_enabled",
			cfg: Ops{
				Enabled:        true,
				ExpVarsEnabled: true,
			},
			want: true,
		},
		{
			name: "version_enabled",
			cfg: Ops{
				Enabled:        true,
				VersionEnabled: true,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, tt.cfg.IsEnabled())
		})
	}
}
