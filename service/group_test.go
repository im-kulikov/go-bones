package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroup(t *testing.T) {
	cases := []struct {
		name string
		opts []Service
		nums int
	}{
		{
			name: "should ignore empty",
			nums: 1,
			opts: []Service{
				nil,
				testService("test-enabled"),
			},
		},
		{
			name: "should ignore disabled",
			nums: 1,
			opts: []Service{
				testService("test-enabled"),
				testService("test-disabled"),
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc, ok := NewGroup(tt.name, tt.opts...).(*Group)
			require.True(t, ok)
			require.Equal(t, tt.name, svc.Name())
			require.Len(t, svc.Services(), tt.nums)
			require.NoError(t, svc.Start(context.TODO()), "should do nothing")
			require.NotPanics(t, func() { svc.Stop(context.TODO()) }, "should do nothing")
		})
	}
}
