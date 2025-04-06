package service

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSignalContext(t *testing.T) {
	top, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx, stop := SignalContext(top, syscall.SIGUSR1)
	defer stop()

	// Simulate sending a signal
	go func() {
		time.Sleep(100 * time.Millisecond)
		process, err := os.FindProcess(os.Getpid())
		assert.NoError(t, err)
		assert.NoError(t, process.Signal(syscall.SIGUSR1))
	}()

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), ErrOsSignal)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for signal context cancellation")
	}
}

func TestSignalContext_Cancel(t *testing.T) {
	top, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx, stop := SignalContext(top, syscall.SIGUSR1)
	stop()

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), ErrCancelCalled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for manual cancellation")
	}
}

func TestSignalContext_TopContextCancel(t *testing.T) {
	top, cancel := context.WithCancel(context.Background())
	ctx, stop := SignalContext(top, syscall.SIGUSR1)
	defer stop()

	cancel()

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), context.Canceled)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for top context cancellation")
	}
}
