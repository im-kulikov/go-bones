package config

import (
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/require"
)

func TestHealth_WithDefaults(t *testing.T) {
	cfg := Health{DrainDelay: -1, StaleAfter: -1}.WithDefaults()
	require.Equal(t, DefaultHealthInterval, cfg.Interval)
	require.Equal(t, DefaultHealthInitialInterval, cfg.InitialInterval)
	require.Equal(t, DefaultHealthTimeout, cfg.Timeout)
	require.Equal(t, DefaultHealthMinInterval, cfg.MinInterval)
	require.Equal(t, DefaultHealthLogRepeatInterval, cfg.LogRepeatInterval)
	require.Equal(t, 1, cfg.FailureThreshold)
	require.Equal(t, 1, cfg.SuccessThreshold)
	require.Zero(t, cfg.DrainDelay)
	require.Zero(t, cfg.StaleAfter)

	custom := Health{Interval: time.Minute, DrainDelay: 5 * time.Second}.WithDefaults()
	require.Equal(t, time.Minute, custom.Interval)
	require.Equal(t, 5*time.Second, custom.DrainDelay)
}

func TestHealth_TagDefaultsMatchConstants(t *testing.T) {
	var cfg Health
	require.NoError(t, gonfig.SetDefaults(&cfg))
	require.Equal(t, cfg.WithDefaults(), cfg, "struct tag defaults and WithDefaults must agree")

	var ops Ops
	require.NoError(t, gonfig.SetDefaults(&ops))
	require.True(t, ops.HealthEnabled)
	require.Equal(t, "/livez", ops.LivePath)
	require.Equal(t, "/readyz", ops.ReadyPath)
	require.Equal(t, "/healthz", ops.HealthPath)
}
