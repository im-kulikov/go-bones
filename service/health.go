package service

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/internal"
	"github.com/im-kulikov/go-bones/logger"
)

// HealthChecker is an optional interface for services managed by Run.
// Services registered with WithService that implement it are registered in the
// health monitor passed to WithHealth under their Name, with Impact=Readiness.
// Implement health.Configurer (HealthOptions) to change the defaults.
//
// It is an alias, so service.HealthChecker and health.Checker are the same type.
type HealthChecker = health.Checker

// WithHealth wires a health monitor into Run:
//   - the monitor is started with the other services (passing it to WithService
//     as well is harmless) and is stopped last (see WithShutdownLast);
//   - every service implementing HealthChecker is registered in the monitor;
//   - on shutdown the monitor is drained (readiness off) before services stop,
//     see WithDrainDelay.
//
// A nil monitor is ignored.
func WithHealth(m *health.Monitor) Option {
	return func(g *settings) {
		if m == nil {
			return
		}

		g.health = m
		g.append(m)
		g.last = append(g.last, m)
	}
}

// WithDrainDelay sets the pause between draining (readiness off) and stopping
// services when Run receives SIGINT/SIGTERM. During the pause servers still
// accept traffic while load balancers and kubelet observe /readyz 503 and gRPC
// NOT_SERVING. A second signal interrupts the pause. There is no pause when a
// service fails or the parent context is canceled.
//
// The shutdown budget is DrainDelay + shutdown timeout, so Kubernetes
// terminationGracePeriodSeconds must be larger than that sum. Do not combine it
// with a preStop sleep hook, the delays add up.
func WithDrainDelay(d time.Duration) Option {
	return func(g *settings) {
		if d < 0 {
			return
		}

		g.drainDelay = d
		g.phased = true
	}
}

// WithShutdownLast makes the given services stop only after all other services
// have stopped. Their context is canceled in the second phase too, so an OPS
// server keeps answering /readyz with 503 (instead of connection refused) while
// the rest of the application shuts down. WithHealth applies it to the monitor.
func WithShutdownLast(v ...Service) Option {
	return func(g *settings) {
		for _, svc := range v {
			if svc != nil {
				g.last = append(g.last, svc)
			}
		}
	}
}

// WithLauncherHealthCheck makes the launcher implement HealthChecker, so it is
// auto-registered by WithHealth under the launcher name. NewLauncher returns an
// unexported type, which is why the check is passed as an option. opts
// configure the registration, e.g. health.WithImpact(health.Informational);
// the default is a Readiness check with the monitor's intervals.
func WithLauncherHealthCheck(fn func(context.Context) error, opts ...health.Option) LauncherOption {
	return func(l *launcher) {
		l.check = fn
		l.checkOpts = slices.Clone(opts) // the caller may reuse its slice
	}
}

// checkedLauncher is a launcher with a health check.
type checkedLauncher struct{ *launcher }

// Check implements HealthChecker.
func (l checkedLauncher) Check(ctx context.Context) error { return l.check(ctx) }

// HealthOptions implements health.Configurer with the options passed to
// WithLauncherHealthCheck.
func (l checkedLauncher) HealthOptions() []health.Option { return slices.Clone(l.checkOpts) }

// sameService compares services without panicking on non-comparable types.
func sameService(a, b Service) bool {
	if a == nil || b == nil {
		return a == b
	}

	if !reflect.TypeOf(a).Comparable() || !reflect.TypeOf(b).Comparable() {
		return false
	}

	return a == b
}

// prepareHealth deduplicates the monitor and registers health checkers.
// Errors are accumulated in g.errs so Run fails before starting anything.
func (g *settings) prepareHealth() {
	if len(g.last) > 0 {
		g.phased = true
	}

	if g.health == nil {
		return
	}

	g.phased = true
	seen := false
	g.handle = slices.DeleteFunc(g.handle, func(svc Service) bool {
		if !sameService(svc, g.health) {
			return false
		}

		dup := seen
		seen = true

		return dup
	})

	for _, svc := range g.handle {
		checker, ok := svc.(HealthChecker)
		if !ok || sameService(svc, g.health) {
			continue
		}

		var opts []health.Option
		if c, ok := svc.(health.Configurer); ok {
			opts = c.HealthOptions()
		}

		if err := g.health.Register(svc.Name(), checker, opts...); err != nil {
			g.errs = append(g.errs, err)
		}
	}
}

func (g *settings) isLast(svc Service) bool {
	return slices.ContainsFunc(g.last, func(item Service) bool { return sameService(item, svc) })
}

// notifySignals subscribes to the configured signals for the drain phase.
func notifySignals(signals ...os.Signal) (<-chan os.Signal, func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)

	return ch, func() { signal.Stop(ch) }
}

// phasedRun is the shutdown flow used with WithHealth, WithDrainDelay or
// WithShutdownLast. It separates "shutdown requested" (trigger) from "cancel
// services" (serve), so readiness can be withdrawn while servers still work:
//
//	trigger → Drain() → [DrainDelay on signal] → cancel+Stop services → cancel+Stop last
func (g *settings) phasedRun(top context.Context) error {
	l := g.logger
	trigger, stop, handleSignals := g.newSignal(top, g.signal...)

	serve, cancelServe := context.WithCancelCause(context.WithoutCancel(top))
	last, cancelLast := context.WithCancelCause(context.WithoutCancel(top))

	// serve and last are detached from top, so an already done top would
	// otherwise start services with live contexts until the watcher below
	// cancels them. Cancel them up front, as the classic path does.
	if top.Err() != nil {
		cancelServe(context.Cause(top))
		cancelLast(context.Cause(top))
	}

	errs := make([]error, len(g.handle))

	var wg sync.WaitGroup
	for i, svc := range g.handle {
		ctx := serve
		if g.isLast(svc) {
			ctx = last
		}

		wg.Go(func() {
			defer stop(nil)

			errs[i] = g.startService(ctx, svc, stop)
		})
	}

	wg.Go(handleSignals)

	done := make(chan struct{})

	go func() {
		defer close(done)

		<-trigger.Done()
		g.drain(trigger)

		cause := context.Cause(trigger)
		internal.GracefulShutdown(trigger, g.shutdown, func(grace context.Context) {
			if errors.Is(cause, ErrOsSignal) {
				l.InfoContext(trigger, cause.Error())
			}

			cancelServe(cause)
			g.stopServices(grace, false)
			cancelLast(cause)
			g.stopServices(grace, true)
		})
	}()

	wg.Wait()
	stop(context.Canceled)
	<-done

	return g.result(top, errs)
}

// startService runs one service and reports a non-ignored error through stop.
func (g *settings) startService(
	ctx context.Context,
	svc Service,
	stop context.CancelCauseFunc,
) error {
	g.logger.Info("starting service", logger.String("service", svc.Name()))

	started := time.Now()
	err := svc.Start(ctx)
	if err == nil || containsError(err, g.ignore) {
		g.logStopped(svc, started)

		return nil
	}

	stop(err)
	g.logger.Error("could not start service",
		logger.String("service", svc.Name()),
		logger.Err(err))

	return err
}

// drain withdraws readiness and, when shutdown was requested by an OS signal,
// waits DrainDelay (interrupted by a second signal).
func (g *settings) drain(trigger context.Context) {
	if g.health != nil {
		g.health.Drain()
	}

	if g.drainDelay <= 0 || !errors.Is(context.Cause(trigger), ErrOsSignal) {
		return
	}

	signals, release := g.notify(g.signal...)
	defer release()

	g.logger.Info("draining before shutdown", logger.Duration("drain_delay", g.drainDelay))

	timer := time.NewTimer(g.drainDelay)
	defer timer.Stop()

	select {
	case <-timer.C:
	case sig := <-signals:
		g.logger.Warn("second signal received, skipping drain delay", logger.Any("signal", sig))
	}
}

// stopServices stops either the regular or the "last" group concurrently.
func (g *settings) stopServices(grace context.Context, lastGroup bool) {
	var wg sync.WaitGroup

	for _, svc := range g.handle {
		if g.isLast(svc) != lastGroup {
			continue
		}

		wg.Go(func() {
			g.logger.InfoContext(
				grace,
				"shutting down service",
				logger.String("service", svc.Name()),
			)
			svc.Stop(grace)
		})
	}

	wg.Wait()
}
