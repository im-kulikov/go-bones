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

const defaultShutdownTimeout = time.Second * 15

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
	cfg := settings{signal: defaultSignals, ignore: errors.Join(defaultIgnoredErrors...)}
	for _, option := range options {
		option(&cfg)
	}

	l := logger.Named(log, "go-bones", "service")
	ctx, cancel, handleSignals := signalContextRoutine(top, cfg.signal...)

	var wg sync.WaitGroup
	for _, service := range cfg.handle {
		wg.Go(func() {
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
	defer shutdownServices(ctx, l, cfg)

	wg.Wait()
	cancel(context.Canceled)

	if err := context.Cause(ctx); err != nil && !containsError(err, cfg.ignore) {
		return err
	}

	return nil
}

// shutdownServices gracefully shutdown all registered services
// when the provided context is canceled.
//
// It listens for the cancellation signal from the top context, then
// initiates a shutdown sequence for all services defined in `cfg.handle`.
// Each service is stopped concurrently while ensuring proper synchronization.
//
// Parameters:
//   - top: The parent context that triggers the shutdown process.
//   - log: Logger instance used for logging shutdown events.
//   - cfg: Configuration containing shutdown timeout and service list.
func shutdownServices(top context.Context, log *logger.Logger, cfg settings) {
	<-top.Done()

	if err := context.Cause(top); err != nil && errors.Is(err, ErrOsSignal) {
		log.InfoContext(top, err.Error())
	}

	timeout := cfg.shutdown
	if timeout <= 0 {
		timeout = defaultShutdownTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.InfoContext(ctx, "shutting down services")

	var wg sync.WaitGroup
	for _, service := range cfg.handle {
		wg.Go(func() {
			log.InfoContext(ctx, "shutting down service",
				logger.String("service", service.Name()))
			service.Stop(ctx)
		})
	}

	wg.Wait()
}
