package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type (
	testService  string
	plainService string
)

var errTest = errors.New("test")

func (t testService) Name() string  { return string(t) }
func (p plainService) Name() string { return string(p) }

func (t testService) Stop(context.Context)  {}
func (p plainService) Stop(context.Context) {}

func (t testService) Enabled() bool { return strings.Contains(string(t), "enabled") }

func (t testService) Start(ctx context.Context) error {
	if strings.Contains(string(t), "error") {
		return errTest
	}

	<-ctx.Done()

	return ctx.Err()
}

func (p plainService) Start(ctx context.Context) error {
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
			name: "composed-services(test-enabled)",
			nums: 1,
			opts: []Service{
				nil,
				testService("test-enabled"),
			},
		},
		{
			name: "composed-services(test-enabled-1,test-enabled-2)",
			nums: 2,
			opts: []Service{
				testService("test-enabled-1"),
				testService("test-enabled-2"),
				testService("test-disabled"),
			},
		},
		{
			name: "composed-services(test-enabled,plain-service)",
			nums: 2,
			opts: []Service{
				testService("test-enabled"),
				plainService("plain-service"),
				testService("test-disabled"),
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc, ok := Compose(tt.opts...).(composed)
			require.True(t, ok)
			require.Equal(t, tt.name, svc.Name())
			require.Len(t, svc, tt.nums)
			assert.ErrorIs(t, svc.Start(t.Context()), ErrComposedServiceNotRunnable)
			require.NotPanics(t, func() {
				svc.Stop(t.Context())
			}, "stop should be a no-op")
		})
	}
}
