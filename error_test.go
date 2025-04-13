package bones

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const ErrTest Error = "test error"

func shouldCatchAnError() (int, error) {
	return 42, ErrTest
}

func TestOnlyError(t *testing.T) {
	require.NotEmpty(t, ErrTest.Error())
	require.ErrorIs(t, OnlyError(shouldCatchAnError()), ErrTest)
}
