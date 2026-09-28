package service

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/internal"
	"github.com/im-kulikov/go-bones/logger"
)

type settings struct {
	ignore    error
	handle    []Service
	signal    []os.Signal
	logger    *logger.Logger
	shutdown  time.Duration
	newSignal func(context.Context, ...os.Signal) (context.Context, context.CancelCauseFunc, handler)

	// health integration, see WithHealth, WithDrainDelay and WithShutdownLast
	health     *health.Monitor
	drainDelay time.Duration
	last       []Service
	phased     bool
	notify     func(...os.Signal) (<-chan os.Signal, func())
	errs       []error
}

// Service represents a long-running component managed by Run or RunContext.
// Start reports terminal runtime errors to the runner. Stop performs best-effort
// cleanup and is responsible for reporting its own shutdown failures.
type Service interface {
	Name() string
	Start(context.Context) error
	Stop(context.Context)
}

// Enabler allows optional services to declare whether they should run.
type Enabler interface {
	Enabled() bool
}

var (
	//nolint:gochecknoglobals
	defaultIgnoredErrors = []error{
		ErrOsSignal,
		ErrCancelCalled,
		context.Canceled,
		context.DeadlineExceeded,
	}

	//nolint:gochecknoglobals
	defaultSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM}
)

// containsError reports whether err matches any of errs (via errors.Is), or
// matches something wrapped by a composite errs entry such as an
// errors.Join result - hence the recursion into Unwrap() []error.
//
// This used to also start with `if errors.Is(errors.Join(errs...), err) {
// return true }`. That check asks whether err itself is found within the
// errs values' own chain (errs wraps err) - the opposite direction from
// what every call site actually needs (err wraps/matches one of errs) - so
// it was genuinely dead: every case it caught, the loop below already
// caught via errors.Is(err, e) (which is also true whenever e == err).
// Confirmed by re-running Test_defaultErrorsIgnore, Test_groupErrors, and
// TestRunContext_* with it removed before deleting it.
func containsError(err error, errs ...error) bool {
	for _, e := range errs {
		if errors.Is(err, e) {
			return true
		}

		if v, ok := e.(interface{ Unwrap() []error }); !ok {
			continue
		} else if containsError(err, v.Unwrap()...) {
			return true
		}
	}

	return false
}

// Run starts multiple goroutines and ensures their graceful shutdown.
//
// It initializes and manages the lifecycle of multiple concurrent tasks,
// handling their execution and termination in a controlled manner.
//
// This function is a wrapper around RunContext with a default background context.
//
// Parameters:
//   - log: Logger instance used for logging events and errors.
//   - options: Optional configuration parameters.
//
// Returns:
//   - `error`: The combined (via errors.Join) non-ignored errors returned by every
//     managed service's Start method, or nil if none failed.
func Run(log *logger.Logger, options ...Option) error {
	return RunContext(context.Background(), log, options...)
}

// RunContext starts multiple goroutines and ensures their graceful shutdown,
// using the provided context to control their lifecycle.
//
// Unlike Run, this function allows passing a specific context,
// which can be used to manage cancellation and timeouts.
//
// Parameters:
//   - top: The parent context that controls the execution of the goroutines.
//   - log: Logger instance used for logging events and errors.
//   - options: Optional configuration parameters.
//
// With WithHealth, WithDrainDelay or WithShutdownLast the shutdown is phased:
// the health monitor is drained first, then (on SIGINT/SIGTERM) Run waits
// DrainDelay, then regular services are canceled and stopped, and only then the
// "last" services (the monitor and anything passed to WithShutdownLast).
// Health registration errors are returned before any service is started.
//
// Returns:
//   - error: The combined (via errors.Join) non-ignored errors returned by every
//     managed service's Start method, or nil if none failed. context.CancelCauseFunc
//     only remembers the first cause it's given, so this is tracked separately from
//     ctx's cancellation cause: if two services fail around the same time, both of
//     their errors are reported, not just whichever one's cancel call won the race.
func RunContext(top context.Context, log *logger.Logger, options ...Option) error {
	cfg := newSettings(log, options...)
	cfg.prepareHealth()

	if err := errors.Join(cfg.errs...); err != nil {
		return err
	}

	if len(cfg.handle) == 0 {
		return nil
	}

	if cfg.phased {
		return cfg.phasedRun(top)
	}

	return cfg.run(top)
}

// run is the classic flow: services receive the signal context directly and
// are stopped concurrently once it is canceled.
func (g *settings) run(top context.Context) error {
	l := g.logger
	ctx, cancel, handleSignals := g.newSignal(top, g.signal...)

	errs := make([]error, len(g.handle))

	var wg sync.WaitGroup
	for i, service := range g.handle {
		wg.Go(func() {
			defer cancel(nil)

			l.Info("starting service", logger.String("service", service.Name()))
			started := time.Now()
			err := service.Start(ctx)
			if err == nil || containsError(err, g.ignore) {
				g.logStopped(service, started)
			} else {
				errs[i] = err
				cancel(err)

				l.Error("could not start service",
					logger.String("service", service.Name()),
					logger.NamedError("cause", context.Cause(ctx)),
					logger.Err(err))
			}
		})
	}

	wg.Go(handleSignals)
	defer internal.LazyGracefulShutdown(ctx, g.shutdown, func(grace context.Context) {
		if err := context.Cause(ctx); err != nil && errors.Is(err, ErrOsSignal) {
			l.InfoContext(ctx, err.Error())
		}

		l.InfoContext(grace, "shutting down services")

		var shutdown sync.WaitGroup
		for _, service := range g.handle {
			shutdown.Go(func() {
				l.InfoContext(grace, "shutting down service",
					logger.String("service", service.Name()))

				service.Stop(grace)
			})
		}

		shutdown.Wait()
	})()

	wg.Wait()
	cancel(context.Canceled)

	return g.result(top, errs)
}

// logStopped reports a service that returned from Start without a failure,
// so every "starting service" line has a matching end.
func (g *settings) logStopped(svc Service, started time.Time) {
	g.logger.Info("service stopped",
		logger.String("service", svc.Name()),
		logger.Duration("uptime", time.Since(started)))
}

// result joins the service errors with the cancellation cause of top, unless
// that cause is ignored or already reported by a service: a caller that cancels
// RunContext with its own cause gets it back.
func (g *settings) result(top context.Context, errs []error) error {
	err := errors.Join(errs...)
	if cause := context.Cause(top); cause != nil &&
		!containsError(cause, g.ignore) && !errors.Is(err, cause) {
		err = errors.Join(err, cause)
	}

	return err
}

func newSettings(log *logger.Logger, options ...Option) settings {
	cfg := settings{
		logger:    logger.Named(log, "go-bones", "service"),
		signal:    defaultSignals,
		ignore:    errors.Join(defaultIgnoredErrors...),
		newSignal: signalContextRoutine,
		notify:    notifySignals,
	}
	for _, option := range options {
		option(&cfg)
	}

	return cfg
}
