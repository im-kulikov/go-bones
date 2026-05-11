package service

import (
	"context"
	"runtime"
	"slices"
	"sync/atomic"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/logger"
)

var (
	// ErrEmptyLauncher reports that NewLauncher received a nil Launcher callback.
	ErrEmptyLauncher = bones.Error("empty launcher")
	// ErrStopsLauncher reports that Start was called after Stop had already begun.
	// Launcher is intentionally single-run, so a stopped instance is terminal.
	ErrStopsLauncher = bones.Error("start stopped launcher")
)

// Launcher is the user-supplied function executed by launcher.Start.
// It receives a child context canceled from Stop or by the parent context.
type Launcher func(context.Context) error

// The launcher adapts a single long-running function to the Service interface.
//
// The type intentionally models a one-shot lifecycle:
//   - Start may execute the callback only once.
//   - Stop is best-effort and idempotent.
//   - The callback result is stored and returned to every waiter after completion.
//
// This contract reflects the current service orchestration model in this repository:
// services are constructed once, started once, and then shut down permanently.
type launcher struct {
	name string
	call Launcher
	done chan struct{}
	logs *logger.Logger
	hook []func(context.Context)

	init atomic.Bool
	halt atomic.Bool

	errors atomic.Pointer[error]
	cancel atomic.Pointer[context.CancelFunc]
}

// LauncherOption configures launcher runtime behaviour.
type LauncherOption func(*launcher)

// WithLauncherLogger overrides the logger used for launcher lifecycle messages.
// Nil is ignored so callers can pass optional logger dependencies safely.
func WithLauncherLogger(log *logger.Logger) LauncherOption {
	return func(l *launcher) {
		if log != nil {
			l.logs = log
		}
	}
}

// WithLauncherShutdownHooks registers callbacks that run after Stop finishes waiting.
// Nil callbacks are discarded to keep shutdown paths panic-free for optional hooks.
func WithLauncherShutdownHooks(hook ...func(context.Context)) LauncherOption {
	return func(l *launcher) {
		l.hook = append(l.hook, slices.DeleteFunc(hook, func(h func(context.Context)) bool {
			return h == nil
		})...)
	}
}

func (l *launcher) apply(options ...LauncherOption) *launcher {
	for _, option := range options {
		option(l)
	}

	return l
}

// NewLauncher creates a Service wrapper for a single background callback.
//
// The returned service has a strict lifecycle:
//   - Start runs the callback once and remembers its result.
//   - Stop cancels the callback context, waits for completion or shutdown timeout,
//     and then executes shutdown hooks once.
//   - Further Start calls fail once the instance has already started or begun stopping.
func NewLauncher(name string, call Launcher, options ...LauncherOption) Service {
	return (&launcher{
		name: name,
		call: call,
		logs: logger.Default(),
		done: make(chan struct{}),
	}).apply(options...)
}

// Name returns the service name used in orchestration and lifecycle logs.
func (l *launcher) Name() string {
	return l.name
}

// Start executes the launcher callback exactly once.
//
// Start first validates the callback and parent context, then atomically claims the
// launcher run. The callback receives a derived cancelable context, and its returned
// error is stored, so callers waiting on the same run observe a consistent result.
// If Stop has already begun, Start returns ErrStopsLauncher immediately because the
// instance has already entered its terminal shutdown state.
//
// Why it works this way:
//   - The launcher used to have more ambiguous restart semantics.
//   - The current implementation makes the one-shot contract explicit.
//   - Persisting the callback result prevents races where later waiters would lose
//     the original startup/shutdown error.
func (l *launcher) Start(top context.Context) error {
	if l.call == nil {
		return ErrEmptyLauncher
	}

	if err := top.Err(); err != nil {
		return err
	}

	if called := l.halt.Load(); called {
		return ErrStopsLauncher
	}

	if !l.init.Swap(true) {
		ctx, cancel := context.WithCancel(top)
		l.cancel.Store(&cancel)
		l.errors.Store(new(l.call(ctx)))
		close(l.done)
	}

	<-l.done
	err := l.errors.Load()

	return *err
}

// Stop begins graceful shutdown for a started launcher.
//
// Stop is intentionally a no-op before Start, which avoids waiting on a launcher
// that never claimed resources. Once shutdown begins, Stop cancels the callback
// context, waits for the callback to exit or for ctx to expire, and finally runs
// registered shutdown hooks once. The `halt` flag also blocks any later Start call,
// making Stop the terminal transition for the launcher lifecycle.
func (l *launcher) Stop(ctx context.Context) {
	if called := l.init.Load(); !called {
		return
	}

	if !l.halt.Swap(true) {
		var cancel *context.CancelFunc
		for cancel = l.cancel.Load(); cancel == nil; cancel = l.cancel.Load() {
			if ctx.Err() != nil {
				return
			}

			runtime.Gosched()
		}

		(*cancel)()

		select {
		case <-l.done:
		case <-ctx.Done():
		}

		for _, h := range l.hook {
			h(ctx)
		}
	}
}
