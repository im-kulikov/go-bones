package service

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/im-kulikov/go-bones"
)

// launcher implements a background process that runs a specified Launcher function
// and supports graceful shutdown once started.
type launcher struct {
	name string
	call Launcher
	done chan struct{} // Signals when the launcher has fully stopped.
	wait chan struct{} // Ensures the launcher has started before stopping.
	once *sync.Once
	stop context.CancelCauseFunc

	onShutdown []func(context.Context)
}

// Launcher is a function executed by the launcher, usually containing the main logic
// to be run in the background until the context is canceled or the function returns.
type Launcher func(context.Context) error

// ErrEmptyLauncher is returned if the launcher function is nil or empty.
const ErrEmptyLauncher bones.Error = "empty launcher function"

// NewLauncher creates and returns a new Service that runs the provided Launcher function.
// If the function is nil, any subsequent call to Start returns ErrEmptyLauncher.
// The optional onShutdown callbacks are invoked when Stop completes.
func NewLauncher(name string, call Launcher, onShutdown ...func(context.Context)) Service {
	return &launcher{
		name: name,
		call: call,
		stop: func(error) {},
		once: new(sync.Once),
		done: make(chan struct{}),
		wait: make(chan struct{}),

		onShutdown: slices.DeleteFunc(onShutdown, func(handle func(context.Context)) bool {
			return handle == nil
		}),
	}
}

// Name returns the name of the launcher, implementing the Service interface.
func (w *launcher) Name() string {
	return w.name
}

// Start runs the launcher function in a separate goroutine. If the launcher function is nil,
// it returns ErrEmptyLauncher. Once the context is canceled, the launcher terminates.
func (w *launcher) Start(ctx context.Context) error {
	defer w.once.Do(func() { close(w.done) })

	if w.call == nil {
		close(w.wait)
		return ErrEmptyLauncher
	}

	var grace context.Context
	grace, w.stop = context.WithCancelCause(ctx)

	if grace.Err() != nil {
		close(w.wait)
		return context.Cause(grace)
	}

	close(w.wait)
	return w.call(grace)
}

// Stop gracefully shuts down the launcher, waiting for it to exit. If the launcher
// has not started yet, it blocks until Start is called before proceeding.
// After the launcher stops, any onShutdown callbacks are invoked.
func (w *launcher) Stop(ctx context.Context) {
	select {
	case <-ctx.Done():
		w.stop(fmt.Errorf("launcher %q stopped: %w", w.name, context.Cause(ctx)))
	case <-w.wait: // Ensure the launcher has started before stopping.
	}

	select { // Wait until the launcher stops or the context deadline expires.
	case <-w.done:
	case <-ctx.Done():
	}

	for _, call := range w.onShutdown {
		call(ctx)
	}
}
