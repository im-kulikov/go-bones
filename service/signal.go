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

// ErrCancelCalled fires when context.CancelCauseFunc is called.
const ErrCancelCalled = bones.Error("cancel called")

// ErrOsSignal fires when os.Signal received.
const ErrOsSignal = bones.Error("received signal")

// SignalContext creates a context that is canceled when one of the specified signals is received.
// This function serves as an alternative to signal.NotifyContext but provides better introspection.
func SignalContext(
	top context.Context,
	signals ...os.Signal,
) (context.Context, context.CancelFunc) {
	ctx, cancel, handle := signalContextRoutine(top, signals...)
	go handle()
	return ctx, func() { cancel(ErrCancelCalled) }
}

// ErrReceivedSignal is a custom error for received OS signals.
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
		if ctx.Err() != nil {
			return
		}

		var err error
		select {
		case sig := <-out:
			err = ErrReceivedSignal(sig)
		case <-ctx.Done():
			err = errors.Join(context.Canceled, context.Cause(ctx))
		}

		cancel(err)
		signal.Stop(out)
	}
}
