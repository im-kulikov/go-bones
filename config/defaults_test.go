package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaults(t *testing.T) {
	ops := Defaults[Ops]()
	require.True(t, ops.Enabled)
	require.True(t, ops.MetricsEnabled)
	require.Equal(t, ":8090", ops.Address)
	require.Equal(t, "/readyz", ops.ReadyPath)
	require.Equal(t, 30*time.Second, ops.ShutdownTimeout)

	require.Equal(t, 10*time.Second, Defaults[Base]().Health.Interval, "nested sections too")
}

func TestDefaults_InvalidTagPanics(t *testing.T) {
	type broken struct {
		Every time.Duration `default:"not a duration"`
	}

	defer func() {
		err, ok := recover().(error)
		require.True(t, ok)
		require.ErrorContains(t, err, "config: defaults of config.broken")
	}()

	Defaults[broken]()
}
