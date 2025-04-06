package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testService string

var errTest = errors.New("test")

func (t testService) Name() string { return string(t) }

func (t testService) Stop(context.Context) {}

func (t testService) Enabled() bool { return strings.Contains(string(t), "enabled") }

func (t testService) Start(ctx context.Context) error {
	if strings.Contains(string(t), "error") {
		return errTest
	}

	<-ctx.Done()

	return ctx.Err()
}

func TestGroup(t *testing.T) {
	cases := []struct {
		name string
		opts []Service
		nums int
	}{
		{
			name: "group-of-services(test-enabled)",
			nums: 1,
			opts: []Service{
				nil,
				testService("test-enabled"),
			},
		},
		{
			name: "group-of-services(test-enabled-1,test-enabled-2)",
			nums: 2,
			opts: []Service{
				testService("test-enabled-1"),
				testService("test-enabled-2"),
				testService("test-disabled"),
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc, ok := NewGroup(tt.opts...).(group)
			require.True(t, ok)
			require.Equal(t, tt.name, svc.Name())
			require.Len(t, svc, tt.nums)
			require.Panics(t, func() {
				assert.NoError(t, svc.Start(context.TODO()))
			}, "should do nothing")
			require.Panics(t, func() {
				svc.Stop(context.TODO())
			}, "should do nothing")
		})
	}
}
