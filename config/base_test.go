package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_setAppSettings_error(t *testing.T) {
	require.ErrorIs(t,
		setAppSettings(Ops{}, "name", "test"),
		ErrPointerExpected)
}
