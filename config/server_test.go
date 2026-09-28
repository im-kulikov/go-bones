package config

import (
	"runtime/debug"
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
