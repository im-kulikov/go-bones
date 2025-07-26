package config

import (
	"net"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/require"
)

type TestGRPC struct {
	Address string

	BaseGRPC
}

func (c TestGRPC) Addr() string { return c.Address }

func Test_ExampleGRPCSettings(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	var cfg TestGRPC
	cfg.Address = lis.Addr().String()
	cfg.ShutdownTimeout = time.Nanosecond

	// check that empty TLS config returns an error
	require.ErrorIs(t, fetchError(cfg.PrepareTLSConfig()), ErrTLSDisabled)

	require.Equal(t, lis.Addr().String(), cfg.Addr())
	require.IsType(t, BaseGRPC{}, cfg.Base())

	// setup default TLS config
	cfg.TLSConfig = new(TLS)

	require.NoError(t, gonfig.SetDefaults(cfg.TLSConfig))
	cfg.TLSConfig.Enabled = true
	cfg.TLSConfig.KeyFile, cfg.TLSConfig.CertFile = generateTLSKeyPair(t)

	_, err = cfg.PrepareTLSConfig()
	require.NoError(t, err)
}
