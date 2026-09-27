package service

import (
	"context"
	"fmt"
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
	// ErrLauncherPanicked reports that the callback passed to NewLauncher panicked.
	// Start recovers the panic instead of letting it crash the process, so one
	// misbehaving service doesn't take down every other service being orchestrated
	// alongside it and still gets a chance at an orderly stop/shutdown-hook run.
	ErrLauncherPanicked = bones.Error("launcher callback panicked")
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
	// started is closed once cancel is published, so Stop can wait for that
	// publication without busy-polling.
	started chan struct{}
	logs    *logger.Logger
	hook    []func(context.Context)
	check   func(context.Context) error

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

// WithLauncherShutdownHooks registers callbacks that run exactly once, immediately
// after the callback passed to NewLauncher returns - whether that happens because
// Stop canceled it or because it exited on its own. Hooks run synchronously in the
// same goroutine as the callback, strictly after it returns, so they can never
// observe or race with a still-executing callback.
// Nil callbacks are discarded to keep shutdown paths panic-free for optional hooks.
func WithLauncherShutdownHooks(hook ...func(context.Context)) LauncherOption {
	return func(l *launcher) {
		// Clone before DeleteFunc: hook aliases the caller's backing array when
		// called as WithLauncherShutdownHooks(existingSlice...), and DeleteFunc
		// mutates its argument in place.
		l.hook = append(l.hook, slices.DeleteFunc(slices.Clone(hook), func(h func(context.Context)) bool {
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
//   - Start runs the callback once, remembers its result, and then runs shutdown
//     hooks once the callback has returned.
//   - Stop cancels the callback context and waits for completion, up to ctx's
//     deadline; it never runs shutdown hooks itself.
//   - Further Start calls fail once the instance has already started or begun stopping.
//
// With WithLauncherHealthCheck the returned service also implements HealthChecker.
func NewLauncher(name string, call Launcher, options ...LauncherOption) Service {
	l := (&launcher{
		name:    name,
		call:    call,
		logs:    logger.Default(),
		done:    make(chan struct{}),
		started: make(chan struct{}),
	}).apply(options...)

	if l.check != nil {
		return checkedLauncher{l}
	}

	return l
}

// Name returns the service name used in orchestration and lifecycle logs.
func (l *launcher) Name() string {
	return l.name
}

// invoke runs the callback with a panic guard, converting a panic into
// ErrLauncherPanicked instead of letting it crash the process.
func (l *launcher) invoke(ctx context.Context) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrLauncherPanicked, r)

			l.logs.ErrorContext(ctx, "launcher callback panicked",
				logger.String("service", l.name),
				logger.Any("panic", r))
		}
	}()

	return l.call(ctx)
}

// Start executes the launcher callback exactly once.
//
// Start first validates the callback and parent context, then atomically claims the
// launcher run. The callback receives a derived cancelable context, and its returned
// error is stored, so callers waiting on the same run observe a consistent result.
// If Stop has already begun, Start returns ErrStopsLauncher immediately because the
// instance has already entered its terminal shutdown state.
//
// Once the callback returns, Start runs the registered shutdown hooks itself, in the
// same goroutine, before closing l.done. This is the only place hooks run: doing it
// here - strictly after the callback has returned and before anything is signaled as
// complete - is what guarantees hooks can never run concurrently with the callback,
// regardless of whether Stop was ever called or how long its ctx allowed it to wait.
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
		close(l.started)

		l.errors.Store(new(l.invoke(ctx)))

		hookCtx := context.WithoutCancel(ctx)
		for _, h := range l.hook {
			h(hookCtx)
		}

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
// context and waits for the callback to exit, up to ctx's deadline. The `halt`
// flag also blocks any later Start call, making Stop the terminal transition for
// the launcher lifecycle.
//
// Stop does not run shutdown hooks itself - see Start, which runs them exactly
// once, strictly after the callback returns. If ctx expires first, Stop simply
// returns without waiting further; the callback keeps running in the background
// (in whatever goroutine called Start) and the hooks still run automatically,
// from that goroutine, once it eventually does return.
func (l *launcher) Stop(ctx context.Context) {
	if called := l.init.Load(); !called {
		return
	}

	if !l.halt.Swap(true) {
		// Check l.started non-blocking first: a plain `select { case
		// <-l.started: case <-ctx.Done(): }` races the two fairly, so with an
		// already-canceled ctx (a legitimate "fire and forget" caller
		// pattern - see network/grpc's shutdown-callback integration test),
		// Go could pick ctx.Done() over an already-published l.started
		// roughly half the time, skipping cancel entirely and leaving the
		// callback to run forever with nothing left to stop it.
		select {
		case <-l.started:
		default:
			select {
			case <-l.started:
			case <-ctx.Done():
				return
			}
		}

		cancel := l.cancel.Load()
		(*cancel)()

		select {
		case <-l.done:
		case <-ctx.Done():
			l.logs.WarnContext(ctx, "shutdown context expired before the launcher callback "+
				"returned; it keeps running in the background and shutdown hooks will run once "+
				"it exits", logger.String("service", l.name))
		}
	}
}
