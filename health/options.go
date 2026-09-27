package health

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// Option configures a single check registration.
type Option func(*registration)

// WithImpact sets what a failure of the check affects (default Readiness).
func WithImpact(v Impact) Option {
	return func(r *registration) { r.impact = v }
}

// WithInterval sets the polling interval of the check; 0 keeps the config default.
func WithInterval(v time.Duration) Option {
	return func(r *registration) {
		if v != 0 {
			r.interval = v
		}
	}
}

// WithInitialInterval sets the polling interval used until the first success;
// 0 keeps the config default.
func WithInitialInterval(v time.Duration) Option {
	return func(r *registration) {
		if v != 0 {
			r.initial = v
		}
	}
}

// WithTimeout sets the deadline of a single Check call; 0 keeps the config default.
func WithTimeout(v time.Duration) Option {
	return func(r *registration) {
		if v != 0 {
			r.timeout = v
		}
	}
}

// WithThresholds sets how many consecutive failures turn a passing check into
// failing and how many consecutive successes turn it back (default 1/1).
func WithThresholds(failure, success int) Option {
	return func(r *registration) {
		r.failure = failure
		r.success = success
	}
}

// WithTTL makes a push status stale when it was not updated for longer than v.
// For polled checks it overrides the stale window (default 2*Interval + Timeout).
func WithTTL(v time.Duration) Option {
	return func(r *registration) { r.ttl = v }
}

type registration struct {
	name     string
	impact   Impact
	checker  Checker
	push     bool
	interval time.Duration
	initial  time.Duration
	timeout  time.Duration
	ttl      time.Duration
	failure  int
	success  int

	// runtime state, guarded by Monitor.mu unless stated otherwise
	result     Result
	inFlight   bool
	lastStart  time.Time
	lastLogged time.Time
	failedAt   time.Time
	trigger    chan struct{} // cap 1, coalesces Trigger calls

	up       atomic.Bool  // result.Up(), read lock-free by Heartbeat.Beat
	lastBeat atomic.Int64 // unix nanos of the last Heartbeat.Beat
}

// validate checks a registration after defaults and options have been applied.
func (r *registration) validate() error {
	var problems []string

	if strings.TrimSpace(r.name) == "" {
		problems = append(problems, "empty name")
	}

	if r.impact > Liveness {
		problems = append(problems, "unknown impact")
	}

	if r.failure < 1 || r.success < 1 {
		problems = append(problems, "thresholds must be >= 1")
	}

	if r.ttl < 0 {
		problems = append(problems, "ttl must be >= 0")
	}

	if !r.push {
		problems = append(problems, r.validatePolled()...)
	}

	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("%w %q: %s", ErrInvalidRegistration, r.name, strings.Join(problems, ", "))
}

func (r *registration) validatePolled() []string {
	var problems []string

	if r.checker == nil {
		problems = append(problems, "nil checker")
	}

	if r.interval <= 0 || r.initial <= 0 {
		problems = append(problems, "interval must be > 0")
	}

	if r.timeout <= 0 {
		problems = append(problems, "timeout must be > 0")
	}

	if r.timeout >= r.interval {
		problems = append(problems, "timeout must be < interval")
	}

	return problems
}

// staleAfter returns the stale window, 0 means "never stale".
func (r *registration) staleAfter(defaultStale time.Duration) time.Duration {
	switch {
	case r.ttl > 0:
		return r.ttl
	case r.push:
		return 0
	case defaultStale > 0:
		return defaultStale
	default:
		return 2*r.interval + r.timeout
	}
}
