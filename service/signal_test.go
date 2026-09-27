package service

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalContext(t *testing.T) {
	ctx, stop := SignalContext(t.Context(), syscall.SIGUSR1)
	defer stop()

	// SignalContext registers the signal handler before returning, so no delay
	// is needed before exercising the actual process-signal path.
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, process.Signal(syscall.SIGUSR1))

	select {
	case <-ctx.Done():
		require.ErrorIs(t, context.Cause(ctx), ErrOsSignal)
	case <-time.After(1 * time.Second):
		assert.FailNow(t, "timeout waiting for signal context cancellation")
	}
}

func TestSignalContext_Cancel(t *testing.T) {
	ctx, cancel := SignalContext(t.Context(), syscall.SIGUSR1)
	cancel()

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), ErrCancelCalled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for manual cancellation")
	}
}

func TestSignalContext_TopContextCancel(t *testing.T) {
	top, stop := context.WithCancel(t.Context())
	stop()

	ctx, cancel := SignalContext(top, syscall.SIGUSR1)
	defer cancel()

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), context.Canceled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for top context cancellation")
	}
}
