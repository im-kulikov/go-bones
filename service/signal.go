package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/im-kulikov/go-bones"
)

type handler func()

// ErrCancelCalled is returned when context.CancelCauseFunc is explicitly called.
const ErrCancelCalled = bones.Error("cancel called")

// ErrOsSignal is returned when an OS signal is received by the signal handler.
const ErrOsSignal = bones.Error("received signal")

// SignalContext creates a context which canceled when one of the specified signals is received.
// This function serves as an alternative to signal.NotifyContext but provides better introspection.
func SignalContext(
	top context.Context,
	signals ...os.Signal,
) (context.Context, context.CancelFunc) {
	ctx, cancel, handle := signalContextRoutine(top, signals...)
	go handle()
	return ctx, func() { cancel(ErrCancelCalled) }
}

// ErrReceivedSignal wraps ErrOsSignal with the specific signal that was received.
func ErrReceivedSignal(sig os.Signal) error {
	return fmt.Errorf("%w: %v", ErrOsSignal, sig)
}

// signalContextRoutine creates a signal-aware context and returns a handler function
// that listens for termination signals and cancels the context accordingly.
func signalContextRoutine(
	top context.Context,
	signals ...os.Signal,
) (context.Context, context.CancelCauseFunc, handler) {
	ctx, cancel := context.WithCancelCause(top)
	out := make(chan os.Signal, 1)
	signal.Notify(out, signals...)

	return ctx, cancel, func() {
		defer signal.Stop(out)

		var err error
		defer func() { cancel(err) }()

		if err = ctx.Err(); err != nil {
			return
		}

		select {
		case sig := <-out:
			err = ErrReceivedSignal(sig)
		case <-ctx.Done():
			err = errors.Join(context.Canceled, context.Cause(ctx))
		}
	}
}
