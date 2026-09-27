package health

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/im-kulikov/go-bones"
)

// Checker is the only mandatory contract of a health check.
//
// Check must respect ctx, must not leave background goroutines behind, must be
// cheap (a ping, not a business query) and must not check other services
// transitively. The dependency client keeps its own timeouts; the deadline set
// by the Monitor is only a safety net.
type Checker interface {
	Check(ctx context.Context) error
}

// CheckerFunc adapts a plain function to Checker.
type CheckerFunc func(context.Context) error

// Check calls f(ctx).
func (f CheckerFunc) Check(ctx context.Context) error { return f(ctx) }

// Configurer may be implemented by an auto-registered service to change the
// defaults applied to its check (for example, its Impact).
type Configurer interface {
	HealthOptions() []Option
}

// Reader is the read side of a Monitor used by OPS HTTP and gRPC adapters.
type Reader interface {
	Snapshot() Snapshot
	Subscribe(fn func(Event)) (unsubscribe func())
}

// Impact describes what a failing check affects.
type Impact uint8

const (
	// Readiness is the default: a failure turns /readyz into 503.
	Readiness Impact = iota
	// Informational failures only mark /healthz as degraded; traffic is not removed.
	Informational
	// Liveness failures turn /livez into 503 (restart). Use only for internal checks.
	Liveness
)

// String returns a lowercase name of the impact.
func (i Impact) String() string {
	switch i {
	case Readiness:
		return "readiness"
	case Informational:
		return "informational"
	case Liveness:
		return "liveness"
	default:
		return "unknown"
	}
}

// MarshalText implements encoding.TextMarshaler.
func (i Impact) MarshalText() ([]byte, error) { return []byte(i.String()), nil }

// Status is the thresholded state of a single check.
type Status string

// Possible check statuses.
const (
	StatusUnknown Status = "unknown"
	StatusPassing Status = "passing"
	StatusFailing Status = "failing"
)

// Overall is the aggregated state reported by /healthz.
type Overall string

// Possible aggregated states.
const (
	OverallOK       Overall = "ok"
	OverallDegraded Overall = "degraded"
	OverallFailing  Overall = "failing"
)

// Errors reported by the Monitor.
const (
	// ErrTimeout is recorded when Check did not return before its deadline.
	ErrTimeout bones.Error = "health check timeout"
	// ErrPanic is recorded when Check panicked.
	ErrPanic bones.Error = "health check panicked"
	// ErrStale is recorded when a result was not refreshed in time.
	ErrStale bones.Error = "health check result is stale"
	// ErrMonitorStarted is returned by registration methods after Start.
	ErrMonitorStarted bones.Error = "health monitor already started"
	// ErrInvalidRegistration is returned for an invalid name, checker or options.
	ErrInvalidRegistration bones.Error = "invalid health check registration"
	// ErrDuplicateCheck is returned when a name is registered twice.
	ErrDuplicateCheck bones.Error = "duplicate health check name"
)

// Result is the last known state of a single check.
type Result struct {
	Name   string
	Impact Impact
	Status Status
	// Err is the raw error of the last failure. It is meant for logs only,
	// use PublicMessage to expose it over the network.
	Err                  error
	CheckedAt            time.Time
	Duration             time.Duration
	LastSuccess          time.Time
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	Stale                bool
}

// Up reports whether the check is passing and fresh.
func (r Result) Up() bool { return r.Status == StatusPassing && !r.Stale }

// Snapshot is an immutable view of the Monitor state. Readers must not modify Checks.
type Snapshot struct {
	// Running is true between Monitor.Start and the end of Monitor.Stop.
	Running  bool
	Live     bool
	Ready    bool
	Draining bool
	Overall  Overall
	Checks   map[string]Result
}

// Names returns check names in a deterministic (sorted) order.
func (s Snapshot) Names() []string {
	names := make([]string, 0, len(s.Checks))
	for name := range s.Checks {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// Without returns a copy of the snapshot that ignores the given checks while
// computing Live, Ready and Overall. Draining is never ignored.
func (s Snapshot) Without(names ...string) Snapshot {
	if len(names) == 0 {
		return s
	}

	checks := make(map[string]Result, len(s.Checks))
	for name, res := range s.Checks {
		if !slices.Contains(names, name) {
			checks[name] = res
		}
	}

	return aggregate(s.Running, s.Draining, checks)
}

// aggregate computes Live, Ready and Overall from check results.
func aggregate(running, draining bool, checks map[string]Result) Snapshot {
	out := Snapshot{
		Running:  running,
		Live:     running,
		Ready:    running && !draining,
		Draining: draining,
		Checks:   checks,
	}

	degraded := false

	for _, res := range checks {
		switch {
		case res.Impact == Liveness && res.down():
			out.Live = false
		case res.Impact == Readiness && !res.Up():
			out.Ready = false
		case res.Impact == Informational && res.down():
			degraded = true
		}
	}

	out.Ready = out.Ready && out.Live
	out.Overall = overall(out.Ready, degraded)

	return out
}

// down reports whether a check is failing or stale; unknown is not down.
func (r Result) down() bool { return r.Status == StatusFailing || r.Stale }

func overall(ready, degraded bool) Overall {
	switch {
	case !ready:
		return OverallFailing
	case degraded:
		return OverallDegraded
	default:
		return OverallOK
	}
}

// EventKind distinguishes check transitions from aggregate transitions.
type EventKind uint8

const (
	// EventCheck is a status transition of a single check.
	EventCheck EventKind = iota
	// EventReady is emitted when the aggregate becomes ready.
	EventReady
	// EventNotReady is emitted when the aggregate stops being ready.
	EventNotReady
	// EventDraining is emitted once, when Drain is called.
	EventDraining
	// EventLive is emitted when the aggregate becomes live (for example, on Start).
	EventLive
	// EventNotLive is emitted when the aggregate stops being live.
	EventNotLive
)

// String returns a lowercase name of the event kind.
func (k EventKind) String() string {
	switch k {
	case EventCheck:
		return "check"
	case EventReady:
		return "ready"
	case EventNotReady:
		return "not_ready"
	case EventDraining:
		return "draining"
	case EventLive:
		return "live"
	case EventNotLive:
		return "not_live"
	default:
		return "unknown"
	}
}

// Event is a hint that the state changed. Consumers should always re-read
// Snapshot: events may be dropped for slow subscribers.
type Event struct {
	Kind   EventKind
	Name   string
	Impact Impact
	From   Status
	To     Status
	Err    error
	At     time.Time
}

// publicError carries a message that is safe to expose over HTTP/gRPC.
type publicError struct {
	msg   string
	cause error
}

func (e *publicError) Error() string {
	if e.cause == nil {
		return e.msg
	}

	return e.msg + ": " + e.cause.Error()
}

func (e *publicError) Unwrap() error { return e.cause }

// PublicError wraps cause with a message that is safe to show in HTTP
// responses. The full text (msg + cause) is still written to logs.
func PublicError(msg string, cause error) error {
	return &publicError{msg: msg, cause: cause}
}

// PublicMessage classifies err into a message that is safe to expose:
// the PublicError message when present, otherwise one of "timeout", "panic",
// "stale", "canceled" or "error". An empty string is returned for nil.
func PublicMessage(err error) string {
	if err == nil {
		return ""
	}

	if pub, ok := errors.AsType[*publicError](err); ok {
		return pub.msg
	}

	switch {
	case errors.Is(err, ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, ErrPanic):
		return "panic"
	case errors.Is(err, ErrStale):
		return "stale"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}
