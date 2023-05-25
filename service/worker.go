package service

import (
	"context"
	"errors"
	"sync"
)

type worker struct {
	name string
	call Launcher
	done chan struct{}

	sync.Mutex
	once *sync.Once
	stop context.CancelFunc
}

// Launcher function that will be called on start.
type Launcher func(context.Context) error

var _ Service = (*worker)(nil)

var errEmptyLauncher = errors.New("empty launcher function")

// NewWorker creates Service implementation for worker.
func NewWorker(name string, call Launcher) Service {
	return &worker{
		name: name,
		call: call,
		once: new(sync.Once),
		done: make(chan struct{}),
	}
}

// Name for worker.
func (w *worker) Name() string { return w.name }

// Start starts worker and waits until it will be stopped.
func (w *worker) Start(ctx context.Context) error {
	if w.call == nil {
		return errEmptyLauncher
	}

	var grace context.Context

	w.Lock()
	grace, w.stop = context.WithCancel(ctx)
	w.Unlock()

	defer w.once.Do(func() { close(w.done) })

	return w.call(grace)
}

// Stop stops worker gracefully.
func (w *worker) Stop(ctx context.Context) {
	w.Lock()
	if w.stop == nil {
		w.Unlock()

		return
	}

	w.stop()
	w.Unlock()

	select { // we should wait until worker stopped or context.Deadlined
	case <-w.done:
	case <-ctx.Done():
	}
}
