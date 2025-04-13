package service

import (
	"context"
	"errors"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

type settings struct {
	ignore   error
	handle   []Service
	signal   []os.Signal
	logger   *logger.Logger
	shutdown time.Duration
}

// Service interface for component that should be run as goroutine.
type Service interface {
	Name() string
	Start(context.Context) error
	Stop(context.Context)
}

// Enabler allows check that service enabled.
type Enabler interface {
	Enabled() bool
}

var (
	_ = Run // prevent unused linter

	//nolint:gochecknoglobals
	defaultIgnoredErrors = []error{
		context.Canceled,
		context.DeadlineExceeded,
	}

	//nolint:gochecknoglobals
	defaultSignals = []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM}
)

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
//   - error: An error if any of the managed goroutines fail to start or stop properly.
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
	cfg := settings{signal: defaultSignals, ignore: errors.Join(defaultIgnoredErrors...)}
	for _, option := range options {
		option(&cfg)
	}

	l := logger.Named(log, "go-bones")
	ctx, cancel, handleSignals := signalContextRoutine(top, cfg.signal...)

	var wg sync.WaitGroup
	for _, service := range cfg.handle {
		wg.Add(1)

		go func() {
			defer wg.Done()

			l.Info("starting service", logger.String("service", service.Name()))
			if err := service.Start(ctx); err != nil && !errors.Is(cfg.ignore, err) {
				cancel(err)

				l.Error("could not start service",
					logger.String("service", service.Name()),
					logger.NamedError("cause", context.Cause(ctx)),
					logger.Err(err))
			}
		}()
	}

	go handleSignals()
	defer shutdownServices(ctx, l, cfg)

	wg.Wait()
	cancel(context.Canceled)

	if err := context.Cause(ctx); err != nil && !errors.Is(cfg.ignore, err) {
		return err
	}

	return nil
}

// shutdownServices gracefully shutdown all registered services
// when the provided context is canceled.
//
// It listens for the cancellation signal from `top` context, then
// initiates a shutdown sequence for all services defined in `cfg.handle`.
// Each service is stopped concurrently while ensuring proper synchronization.
//
// Parameters:
//   - top: The parent context that triggers the shutdown process.
//   - log: Logger instance used for logging shutdown events.
//   - cfg: Configuration containing shutdown timeout and service list.
func shutdownServices(top context.Context, log *logger.Logger, cfg settings) {
	<-top.Done()

	ctx, cancel := context.WithTimeout(top, cfg.shutdown)
	defer cancel()

	log.Info("shutting down services")

	var wg sync.WaitGroup
	for _, service := range cfg.handle {
		wg.Add(1)
		log.Info("shutting down service", logger.String("service", service.Name()))

		go func() {
			defer wg.Done()

			service.Stop(ctx)
		}()
	}

	wg.Wait()
}
