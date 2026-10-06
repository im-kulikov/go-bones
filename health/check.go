package health

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

// Run outcomes, used as the "result" label of go_bones_health_check_runs_total.
const (
	resultSuccess = "success"
	resultError   = "error"
	resultTimeout = "timeout"
	resultPanic   = "panic"

	skipInFlight    = "in_flight"
	skipRateLimited = "rate_limited"
)

func (r *registration) lastBeatTime() time.Time {
	if v := r.lastBeat.Load(); v != 0 {
		return time.Unix(0, v)
	}

	return time.Time{}
}

func latest(times ...time.Time) time.Time {
	var out time.Time

	for _, t := range times {
		if t.After(out) {
			out = t
		}
	}

	return out
}

// loop owns one registration: polled checks are scheduled, push checks with a
// TTL are watched for staleness.
func (m *Monitor) loop(ctx context.Context, r *registration) {
	if r.push {
		m.watch(ctx, r)

		return
	}

	m.schedule(ctx, r)
}

// watch marks a push check stale when it was not updated within its TTL.
func (m *Monitor) watch(ctx context.Context, r *registration) {
	window := r.staleAfter(m.cfg.StaleAfter)
	if window <= 0 {
		return
	}

	timer := time.NewTimer(window)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-timer.C:
			timer.Reset(m.sweep(r, now))
		}
	}
}

// sweep marks r stale if its window has elapsed and returns the time left
// until it may become stale again.
func (m *Monitor) sweep(r *registration, now time.Time) time.Duration {
	window := r.staleAfter(m.cfg.StaleAfter)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear up before reading lastBeat: a Heartbeat.Beat that stores its
	// timestamp after that read then sees up=false and records a success
	// (after this sweep releases m.mu) instead of skipping it on the fast path
	// and leaving a fresh heartbeat marked stale.
	wasUp := r.up.Swap(false)

	res := r.result
	if res.Stale || !m.isStale(r, res, m.startedAt, now) {
		r.up.Store(wasUp)

		ref := latest(res.CheckedAt, r.lastBeatTime(), m.startedAt)

		return max(window-now.Sub(ref), window/10, time.Millisecond)
	}

	next := res
	next.Stale = true
	next.Err = ErrStale
	next.ConsecutiveFailures++
	next.ConsecutiveSuccesses = 0
	next.Status = StatusFailing

	m.applyLocked(r, next, now)

	return window
}

// schedule runs a polled check immediately, then with InitialInterval until the
// first success and with Interval afterwards. Trigger requests are coalesced
// and delayed to respect MinInterval.
func (m *Monitor) schedule(ctx context.Context, r *registration) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	nextAt := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-timer.C:
			m.sweep(r, now)
		case <-r.trigger:
			if wait := m.cfg.MinInterval - time.Since(m.lastStart(r)); wait > 0 {
				m.metrics.skipped(r.name, skipRateLimited)

				if at := time.Now().Add(wait); at.Before(nextAt) {
					nextAt = at
					timer.Reset(wait)
				}

				continue
			}
		}

		m.run(ctx, r)

		next := m.nextInterval(r)
		nextAt = time.Now().Add(next)
		timer.Reset(next)
	}
}

func (m *Monitor) lastStart(r *registration) time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()

	return r.lastStart
}

func (m *Monitor) nextInterval(r *registration) time.Duration {
	m.mu.Lock()
	passed := !r.result.LastSuccess.IsZero()
	m.mu.Unlock()

	if passed {
		return m.jitter(r.interval)
	}

	return m.jitter(r.initial)
}

// run executes one Check with a timeout. At most one call per registration is
// in flight: if the previous call ignored its context and is still running, the
// run is skipped. The result is recorded as soon as the deadline expires,
// without waiting for such a checker to return.
func (m *Monitor) run(ctx context.Context, r *registration) {
	m.mu.Lock()
	if r.inFlight {
		m.mu.Unlock()
		m.metrics.skipped(r.name, skipInFlight)
		m.log.WarnContext(ctx, "health check is still running, skipping run",
			logger.String("check", r.name))

		return
	}

	r.inFlight = true
	r.lastStart = time.Now()
	m.mu.Unlock()

	cctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	done := make(chan error, 1)
	started := time.Now()

	m.checks.Go(func() {
		defer m.clearInFlight(r)

		done <- m.call(cctx, r)
	})

	var err error

	select {
	case err = <-done:
	case <-cctx.Done():
		err = cctx.Err()
	}

	if ctx.Err() != nil {
		return // shutting down: do not record cancellations as failures
	}

	kind, err := classify(err, r.timeout)
	m.record(r, err, kind, time.Since(started))
}

func (m *Monitor) clearInFlight(r *registration) {
	m.mu.Lock()
	r.inFlight = false
	m.mu.Unlock()
}

// call invokes the checker, converting a panic into ErrPanic.
func (m *Monitor) call(ctx context.Context, r *registration) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("%w: %v", ErrPanic, rec)
			m.log.ErrorContext(ctx, "health check panicked",
				logger.String("check", r.name),
				logger.Any("panic", rec),
				logger.String("stack", string(debug.Stack())))
		}
	}()

	return r.checker.Check(ctx)
}

// classify maps an error to the run outcome.
func classify(err error, timeout time.Duration) (string, error) {
	switch {
	case err == nil:
		return resultSuccess, nil
	case errors.Is(err, ErrPanic):
		return resultPanic, err
	case errors.Is(err, ErrTimeout):
		return resultTimeout, err
	case errors.Is(err, context.DeadlineExceeded):
		return resultTimeout, fmt.Errorf("%w after %s: %w", ErrTimeout, timeout, err)
	default:
		return resultError, err
	}
}

// record applies a new outcome to r using its thresholds. A negative took marks
// a push update, which is not observed in the duration histogram. The first outcome
// decides the status immediately (except a failure within StartPeriod, which
// keeps it unknown), afterwards FailureThreshold consecutive failures (or
// SuccessThreshold successes) are needed to flip it.
func (m *Monitor) record(r *registration, err error, kind string, took time.Duration) {
	m.metrics.observe(r.name, kind, took)

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	prev := r.result
	next := prev
	next.CheckedAt = now
	next.Duration = max(took, 0)
	next.Stale = false

	if err == nil {
		next.Err = nil
		next.LastSuccess = now
		next.ConsecutiveSuccesses++
		next.ConsecutiveFailures = 0

		if prev.Status != StatusPassing &&
			(prev.Status == StatusUnknown || next.ConsecutiveSuccesses >= r.success) {
			next.Status = StatusPassing
		}
	} else {
		next.Err = err
		next.ConsecutiveFailures++
		next.ConsecutiveSuccesses = 0

		if prev.Status != StatusFailing && !m.starting(prev, now) &&
			(prev.Status == StatusUnknown || next.ConsecutiveFailures >= r.failure) {
			next.Status = StatusFailing
		}
	}

	m.applyLocked(r, next, now)
}

// starting reports whether a check without a status yet is still within
// StartPeriod: a failure then keeps it unknown, its service may not be started.
// Must be called with m.mu held.
func (m *Monitor) starting(prev Result, now time.Time) bool {
	return prev.Status == StatusUnknown && now.Sub(m.startedAt) < m.cfg.StartPeriod
}

// applyLocked stores next as the result of r, emits/logs a transition and
// publishes a new snapshot. Must be called with m.mu held.
func (m *Monitor) applyLocked(r *registration, next Result, now time.Time) {
	prev := r.result
	r.result = next
	r.up.Store(next.Up())

	if prev.Status == next.Status {
		if next.Status == StatusFailing && now.Sub(r.lastLogged) >= m.cfg.LogRepeatInterval {
			r.lastLogged = now
			m.log.LogAttrs(context.Background(), slog.LevelWarn, "health check still failing",
				m.checkAttrs(r, prev, next)...)
		}

		m.publishLocked(now)

		return
	}

	if next.Status == StatusFailing {
		r.failedAt = now
	}

	r.lastLogged = now
	m.metrics.transition(r.name, next.Status)
	m.logTransition(r, prev, next, now)
	m.publishLocked(now, Event{
		Kind: EventCheck, Name: r.name, Impact: r.impact,
		From: prev.Status, To: next.Status, Err: next.Err, At: now,
	})
}

func (m *Monitor) checkAttrs(r *registration, prev, next Result) []logger.Attr {
	attrs := []logger.Attr{
		logger.String("check", r.name),
		logger.String("impact", r.impact.String()),
		logger.String("from", string(prev.Status)),
		logger.String("to", string(next.Status)),
		logger.Duration("duration", next.Duration),
		logger.Int("consecutive_failures", next.ConsecutiveFailures),
	}

	if next.Err != nil {
		attrs = append(attrs, logger.Err(next.Err))
	}

	return attrs
}

func (m *Monitor) logTransition(r *registration, prev, next Result, now time.Time) {
	attrs := m.checkAttrs(r, prev, next)
	level, msg := slog.LevelWarn, "health check failing"

	switch {
	case next.Status == StatusPassing:
		level, msg = slog.LevelInfo, "health check passing"

		if prev.Status == StatusFailing {
			attrs = append(attrs, logger.Duration("downtime", now.Sub(r.failedAt)))
		}
	case r.impact == Liveness:
		level = slog.LevelError
	}

	m.log.LogAttrs(context.Background(), level, msg, attrs...)
}
