package service

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/im-kulikov/go-bones"
)

// launcher is a background process that runs a given Launcher function
// and supports graceful shutdown.
type launcher struct {
	name string
	call Launcher
	done chan struct{} // Signals when the launcher has fully stopped.
	wait chan struct{} // Ensures the launcher has started before stopping.
	once *sync.Once
	stop context.CancelCauseFunc

	onShutdown []func(context.Context)
}

// Launcher defines a function executed by the launcher.
type Launcher func(context.Context) error

// ErrEmptyLauncher fires when launcher function is empty.
const ErrEmptyLauncher bones.Error = "empty launcher function"

// NewLauncher creates a new launcher instance implementing the Service interface.
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

// Name returns the launcher's name.
func (w *launcher) Name() string { return w.name }

// Start runs the launcher and waits for its termination signal.
// If the provided function is nil, it returns an error.
// The launcher stops when the context is canceled.
func (w *launcher) Start(ctx context.Context) error {
	defer w.once.Do(func() { close(w.done) })

	if w.call == nil {
		close(w.wait) // Signal that the launcher has started.

		return ErrEmptyLauncher
	}

	var grace context.Context
	grace, w.stop = context.WithCancelCause(ctx)

	if grace.Err() != nil {
		close(w.wait) // Signal that the launcher has started.

		return context.Cause(grace)
	}

	close(w.wait) // Signal that the launcher has started.

	return w.call(grace)
}

// Stop gracefully shuts down the launcher, waiting for it to exit.
// If the launcher hasn't started yet, it waits until it does.
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
