package config

import (
	"testing"
	"time"

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
