package service

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"time"

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
}

// Service represents a long-running component managed by Run or RunContext.
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

func containsError(err error, errs ...error) bool {
	if errors.Is(errors.Join(errs...), err) {
		return true
	}

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
//   - `error`: An error if any of the managed goroutines fail to start or stop properly.
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
// Returns:
//   - error: An error if any of the managed goroutines fail to start or stop properly.
func RunContext(top context.Context, log *logger.Logger, options ...Option) error {
	cfg := newSettings(log, options...)
	l := cfg.logger

	if len(cfg.handle) == 0 {
		return nil
	}

	ctx, cancel, handleSignals := cfg.newSignal(top, cfg.signal...)

	var wg sync.WaitGroup
	for _, service := range cfg.handle {
		wg.Go(func() {
			defer cancel(nil)

			l.Info("starting service", logger.String("service", service.Name()))
			err := service.Start(ctx)
			if err != nil && !containsError(err, cfg.ignore) {
				cancel(err)

				l.Error("could not start service",
					logger.String("service", service.Name()),
					logger.NamedError("cause", context.Cause(ctx)),
					logger.Err(err))
			}
		})
	}

	wg.Go(handleSignals)
	defer internal.LazyGracefulShutdown(ctx, cfg.shutdown, func(grace context.Context) {
		if err := context.Cause(ctx); err != nil && errors.Is(err, ErrOsSignal) {
			l.InfoContext(ctx, err.Error())
		}

		l.InfoContext(grace, "shutting down services")

		var shutdown sync.WaitGroup
		for _, service := range cfg.handle {
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

	return runnableError(ctx, cfg.ignore)
}

func newSettings(log *logger.Logger, options ...Option) settings {
	cfg := settings{
		logger:    logger.Named(log, "go-bones", "service"),
		signal:    defaultSignals,
		ignore:    errors.Join(defaultIgnoredErrors...),
		newSignal: signalContextRoutine,
	}
	for _, option := range options {
		option(&cfg)
	}

	return cfg
}

func runnableError(ctx context.Context, ignored error) error {
	if err := context.Cause(ctx); err != nil && !containsError(err, ignored) {
		return err
	}

	return nil
}
