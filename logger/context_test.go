package logger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddContextAttrs_AppendsExistingAttributes(t *testing.T) {
	ctx := AddContextAttrs(t.Context())
	ctx = AddContextAttrs(ctx, String("rid", "r1"))
	ctx = AddContextAttrs(ctx, String("uid", "u1"))

	attrs := fromContext(ctx)
	require.Len(t, attrs, 2)
	require.Equal(t, String("rid", "r1"), attrs[0])
	require.Equal(t, String("uid", "u1"), attrs[1])
}
