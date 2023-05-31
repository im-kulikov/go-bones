package service

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

type (
	runner struct {
		ignore   []error
		services []Service
		logger   logger.Logger
		shutdown time.Duration

		pingPongDisable bool
		pingPongTimeout time.Duration
	}

	// Service interface for component that should be run as goroutine.
	Service interface {
		Name() string
		Start(context.Context) error
		Stop(context.Context)
	}

	// Enabler allows check that service enabled.
	Enabler interface {
		Enabled() bool
	}

	// Runner collects services and runs them concurrently.
	// - when any service returns, all services will be stopped.
	// - when context canceled or deadlined all services will be stopped.
	Runner interface {
		Run(context.Context) error
	}

	stopper struct {
		name string
		err  error
	}
)

const defaultShutdownTimeout = time.Second * 5

var (
	_ Runner = (*runner)(nil)

	// nolint:gochecknoglobals
	defaultIgnoredErrors = []error{
		context.Canceled,
		context.DeadlineExceeded,
	}
)

// New creates and configures Runner by passed Option's.
func New(log logger.Logger, options ...Option) Runner {
	run := &runner{
		logger: log,
		ignore: defaultIgnoredErrors,

		pingPongDisable: true,
	}

	for _, o := range options {
		o(run)
	}

	if run.shutdown <= 0 {
		run.shutdown = defaultShutdownTimeout
	}

	return run
}

func (g *runner) checkAndIgnore(err error) error {
	for i := range g.ignore {
		if errors.Is(err, g.ignore[i]) {
			return nil
		}
	}

	return err
}

// Run allows to run all services (launcher function).
// - method blocks until all services will be stopped.
// - when context will be canceled or deadline exceeded we call shutdown for services.
// - when the first service (launcher function) returns, all other services will be notified to stop.
func (g *runner) Run(parent context.Context) error {
	if len(g.services) == 0 {
		return nil
	}

	ctx, cancel := signal.NotifyContext(parent, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var (
		err error
		top context.Context
		res = make(chan stopper, len(g.services))
	)

	// add ping-pong logger service
	if !g.pingPongDisable {
		g.services = append(g.services, newPingPong(g.logger, g.pingPongTimeout))
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(g.services))

	// run all services
	for i := range g.services {
		go func(svc Service) {
			defer wg.Done()

			g.logger.Infow("running service", "service", svc.Name())

			var errRun error
			if errRun = svc.Start(ctx); errRun != nil {
				errRun = fmt.Errorf("problems with starting %s: %w", svc.Name(), errRun)
			}

			res <- stopper{name: svc.Name(), err: errRun}
		}(g.services[i])
	}

	// wait for context.Done() or error will be received:
	select {
	case state := <-res:
		err = state.err
		top = context.Background()

		g.logger.Errorw("received an error", "service", state.name, "error", state.err)

	case <-ctx.Done():
		err = ctx.Err()
		top = context.Background()

		g.logger.Infow("wait before stop", "timeout", g.shutdown)

		// Kubernetes (rolling update) doesn't wait until a pod is out of rotation before sending SIGTERM,
		// and external LB could still route traffic to a non-existing pod resulting in a surge of 50x API errors.
		// It's recommended to wait for 5 seconds before terminating the program; see references
		// https://github.com/kubernetes-retired/contrib/issues/1140, https://youtu.be/me5iyiheOC8?t=1797.
		time.Sleep(g.shutdown)
	}

	cancel()
	g.stopServices(top, err, res)

	wg.Wait()

	g.logger.Infow("done")

	// return only first error except ignored errors
	return g.checkAndIgnore(err)
}

func (g *runner) stopServices(ctx context.Context, cause error, output <-chan stopper) {
	// prepare graceful context to stop
	grace, stop := context.WithTimeout(ctx, g.shutdown)
	defer stop()

	// we should wait until all services will gracefully stopped
	wg := new(sync.WaitGroup)
	defer wg.Wait()

	wg.Add(len(g.services))

	// notify all services to stop
	for i := range g.services {
		go func(svc Service) {
			defer wg.Done()

			if cause != nil {
				g.logger.Warnw("service will be stopped",
					"service", svc.Name(),
					"cause", cause)
			}

			g.logger.Infow("stopping service", "service", svc.Name())

			svc.Stop(ctx)
		}(g.services[i])
	}

	var (
		stopped int
		length  = len(output)
	)

	// wait when all services will stop
	for {
		select {
		case <-grace.Done():
			return
		case <-output:
			stopped++

			if stopped == length {
				return
			}
		}
	}
}
