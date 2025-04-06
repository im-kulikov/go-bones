package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// worker is a background process that runs a given Launcher function
// and supports graceful shutdown.
type worker struct {
	name string
	call Launcher
	done chan struct{} // Signals when the worker has fully stopped.
	wait chan struct{} // Ensures the worker has started before stopping.

	sync.Mutex
	once *sync.Once
	stop context.CancelCauseFunc
}

// Launcher defines a function executed by the worker.
type Launcher func(context.Context) error

var _ Service = (*worker)(nil)

var errEmptyLauncher = errors.New("empty launcher function")

// NewWorker creates a new worker instance implementing the Service interface.
func NewWorker(name string, call Launcher) Service {
	return &worker{
		name: name,
		call: call,
		stop: func(error) {},
		once: new(sync.Once),
		done: make(chan struct{}),
		wait: make(chan struct{}),
	}
}

// Name returns the worker's name.
func (w *worker) Name() string { return w.name }

// Start runs the worker and waits for its termination signal.
// If the provided function is nil, it returns an error.
// The worker stops when the context is canceled.
func (w *worker) Start(ctx context.Context) error {
	if w.call == nil {
		close(w.wait) // Signal that the worker has started.

		return errEmptyLauncher
	}

	var grace context.Context
	grace, w.stop = context.WithCancelCause(ctx)

	defer w.once.Do(func() { close(w.done) })

	if grace.Err() != nil {
		close(w.wait) // Signal that the worker has started.

		return context.Cause(grace)
	}

	close(w.wait) // Signal that the worker has started.
	return w.call(grace)
}

// Stop gracefully shuts down the worker, waiting for it to exit.
// If the worker hasn't started yet, it waits until it does.
func (w *worker) Stop(ctx context.Context) {
	select {
	case <-ctx.Done():
		w.stop(fmt.Errorf("worker %q stopped: %w", w.name, context.Cause(ctx)))
	case <-w.wait: // Ensure the worker has started before stopping.
	}

	select { // Wait until the worker stops or the context deadline expires.
	case <-w.done:
	case <-ctx.Done():
	}
}
