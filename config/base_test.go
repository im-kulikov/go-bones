package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
