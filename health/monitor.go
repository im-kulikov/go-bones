package health

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
)

const (
	// ServiceName is the name returned by Monitor.Name.
	ServiceName = "health"

	// ErrMonitorStopped is returned by Start after Stop.
	ErrMonitorStopped bones.Error = "health monitor stopped"

	jitterPercent = 10
)

// state is the immutable data behind Snapshot, swapped atomically on every change.
type state struct {
	running   bool
	draining  bool
	startedAt time.Time
	regs      []*registration
	results   map[string]Result
}

// Monitor runs health checks, keeps the last result of each one and hands it
// out to readers (HTTP, gRPC, Prometheus, subscribers). Readers never run checks:
// every I/O happens in goroutines owned by the Monitor.
//
// Monitor structurally implements service.Service (Name/Start/Stop) and
// prometheus.Collector. Registration is allowed only before Start.
type Monitor struct {
	cfg     config.Health
	log     *logger.Logger
	jitter  func(time.Duration) time.Duration
	metrics *metrics

	mu        sync.Mutex
	regs      map[string]*registration
	order     []*registration
	started   bool
	stopped   bool
	running   bool
	draining  bool
	live      bool
	ready     bool
	startedAt time.Time
	cancel    context.CancelFunc

	current atomic.Pointer[state]

	loops  sync.WaitGroup // scheduler and watchdog goroutines
	checks sync.WaitGroup // in-flight Check calls

	subsMu  sync.Mutex
	subs    map[uint64]*subscriber
	nextSub uint64
}

// New creates a Monitor. Zero values of cfg are replaced by defaults, and a nil
// logger falls back to logger.Default.
func New(cfg config.Health, log *logger.Logger) *Monitor {
	m := &Monitor{
		cfg:     cfg.WithDefaults(),
		log:     logger.Named(log, "go-bones", "health"),
		jitter:  defaultJitter,
		metrics: newMetrics(),
		regs:    make(map[string]*registration),
		subs:    make(map[uint64]*subscriber),
	}
	m.publishLocked(time.Now())

	return m
}

// defaultJitter spreads the interval by ±10% so replicas do not hit a shared
// dependency in lockstep.
func defaultJitter(d time.Duration) time.Duration {
	spread := int64(d) * jitterPercent / 100
	if spread <= 0 {
		return d
	}

	//nolint:gosec // jitter does not need a cryptographic source.
	return d + time.Duration(rand.Int64N(2*spread+1)-spread)
}

// Config returns the effective configuration (with defaults applied).
func (m *Monitor) Config() config.Health { return m.cfg }

func (m *Monitor) newRegistration(name string, push bool, opts []Option) *registration {
	r := &registration{
		name:     name,
		push:     push,
		impact:   Readiness,
		interval: m.cfg.Interval,
		initial:  m.cfg.InitialInterval,
		timeout:  m.cfg.Timeout,
		failure:  m.cfg.FailureThreshold,
		success:  m.cfg.SuccessThreshold,
		trigger:  make(chan struct{}, 1),
	}

	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}

	r.initial = min(r.initial, r.interval)

	return r
}

func (m *Monitor) add(r *registration) error {
	if err := r.validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started || m.stopped {
		return ErrMonitorStarted
	}

	if _, ok := m.regs[r.name]; ok {
		return fmt.Errorf("%w: %q", ErrDuplicateCheck, r.name)
	}

	r.result = Result{Name: r.name, Impact: r.impact, Status: StatusUnknown}
	m.regs[r.name] = r
	m.order = append(m.order, r)
	m.publishLocked(time.Now())

	return nil
}

// Register adds a polled check. It must be called before Start, otherwise
// ErrMonitorStarted is returned.
func (m *Monitor) Register(name string, c Checker, opts ...Option) error {
	r := m.newRegistration(name, false, opts)
	r.checker = c

	return m.add(r)
}

// Status registers a push check whose state is set by the returned handle,
// for example from a driver reconnect callback. It never goes stale unless
// WithTTL is given.
func (m *Monitor) Status(name string, opts ...Option) (*StatusHandle, error) {
	r := m.newRegistration(name, true, opts)
	if err := m.add(r); err != nil {
		return nil, err
	}

	return &StatusHandle{m: m, r: r}, nil
}

// Heartbeat registers a Liveness push check that fails when Beat was not called
// for longer than maxAge. It catches stuck loops and deadlocks without putting
// external dependencies into liveness.
func (m *Monitor) Heartbeat(name string, maxAge time.Duration) (*Heartbeat, error) {
	if maxAge <= 0 {
		return nil, fmt.Errorf("%w %q: max age must be > 0", ErrInvalidRegistration, name)
	}

	r := m.newRegistration(name, true, []Option{WithImpact(Liveness), WithTTL(maxAge)})
	if err := m.add(r); err != nil {
		return nil, err
	}

	return &Heartbeat{m: m, r: r}, nil
}

// Trigger requests an out-of-schedule run of a polled check (for example, from a
// reconnect callback). It never blocks, coalesces repeated calls and respects
// MinInterval. Unknown names and push checks are ignored.
func (m *Monitor) Trigger(name string) {
	m.mu.Lock()
	r, ok := m.regs[name]
	m.mu.Unlock()

	if !ok || r.push {
		return
	}

	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

// Name implements service.Service.
func (m *Monitor) Name() string { return ServiceName }

// Start runs the checks and blocks until ctx is canceled or Stop is called.
// A second Start returns ErrMonitorStarted.
func (m *Monitor) Start(top context.Context) error {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()

		return ErrMonitorStopped
	}

	if m.started {
		m.mu.Unlock()

		return ErrMonitorStarted
	}

	ctx, cancel := context.WithCancel(top)
	now := time.Now()
	m.started, m.running, m.startedAt, m.cancel = true, true, now, cancel
	regs := m.order
	m.publishLocked(now)
	// Add the loops before unlocking: a concurrent Stop takes m.mu next, so its
	// loops.Wait always sees them and cannot return before they exit.
	m.loops.Add(len(regs))
	m.mu.Unlock()

	for _, r := range regs {
		go func() {
			defer m.loops.Done()
			m.loop(ctx, r)
		}()
	}

	<-ctx.Done()
	m.loops.Wait()
	m.setRunning(false)

	return nil
}

// Stop drains the monitor (Ready becomes false forever), stops the scheduler
// and waits for in-flight checks until ctx expires. The snapshot stays readable.
func (m *Monitor) Stop(ctx context.Context) {
	m.Drain()

	m.mu.Lock()
	m.stopped = true
	cancel := m.cancel
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	done := make(chan struct{})
	go func() {
		m.loops.Wait()
		m.checks.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		m.log.WarnContext(ctx, "health checks did not finish before shutdown deadline")
	}

	m.setRunning(false)
}

// Drain permanently marks the monitor as not ready. It is idempotent.
func (m *Monitor) Drain() {
	m.mu.Lock()
	if m.draining {
		m.mu.Unlock()

		return
	}

	now := time.Now()
	m.draining = true
	m.publishLocked(now, Event{Kind: EventDraining, At: now})
	m.mu.Unlock()

	m.log.Info("health monitor draining: readiness disabled")
}

func (m *Monitor) setRunning(running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running == running {
		return
	}

	m.running = running
	m.publishLocked(time.Now())
}

// publishLocked stores a new immutable state and emits aggregate events.
// Must be called with m.mu held.
func (m *Monitor) publishLocked(now time.Time, events ...Event) {
	st := m.storeLocked()

	// Events are emitted only after the state is stored, so a subscriber that
	// re-reads Snapshot always sees the change it was notified about.
	for _, ev := range events {
		m.emit(ev)
	}

	snap := m.snapshotOf(st, now)
	if snap.Live != m.live {
		m.live = snap.Live
		m.emit(Event{Kind: pick(snap.Live, EventLive, EventNotLive), At: now})
	}

	if snap.Ready != m.ready {
		m.ready = snap.Ready
		m.emit(Event{Kind: pick(snap.Ready, EventReady, EventNotReady), At: now})
		m.logAggregate(snap.Ready)
	}
}

func pick[T any](cond bool, yes, no T) T {
	if cond {
		return yes
	}

	return no
}

// storeLocked builds and stores a new immutable state. Must be called with m.mu held.
func (m *Monitor) storeLocked() *state {
	results := make(map[string]Result, len(m.order))
	for _, r := range m.order {
		results[r.name] = r.result
	}

	st := &state{
		running:   m.running,
		draining:  m.draining,
		startedAt: m.startedAt,
		regs:      m.order,
		results:   results,
	}
	m.current.Store(st)

	return st
}

func (m *Monitor) logAggregate(ready bool) {
	if ready {
		m.log.Info("service is ready")

		return
	}

	st := m.current.Load()
	m.log.Warn("service is not ready",
		logger.Bool("running", st.running),
		logger.Bool("draining", st.draining))
}

// Snapshot returns the current state. It never performs I/O; staleness is
// evaluated at read time.
func (m *Monitor) Snapshot() Snapshot {
	return m.snapshotOf(m.current.Load(), time.Now())
}

func (m *Monitor) snapshotOf(st *state, now time.Time) Snapshot {
	checks := make(map[string]Result, len(st.results))

	for _, r := range st.regs {
		res := st.results[r.name]
		if st.running && !res.Stale {
			res.Stale = m.isStale(r, res, st.startedAt, now)
		}

		// Beat's fast path only stores lastBeat: overlay it, and count it as a
		// success while the check is up.
		if at := r.lastBeatTime(); at.After(res.CheckedAt) {
			res.CheckedAt = at

			if res.Up() {
				res.LastSuccess = at
			}
		}

		checks[r.name] = res
	}

	return aggregate(st.running, st.draining, checks)
}

// isStale reports whether res was not refreshed within the stale window.
// Before the first result the window is counted from Start.
func (m *Monitor) isStale(r *registration, res Result, startedAt, now time.Time) bool {
	window := r.staleAfter(m.cfg.StaleAfter)
	if window <= 0 {
		return false
	}

	ref := latest(startedAt, res.CheckedAt, r.lastBeatTime())

	return !ref.IsZero() && now.Sub(ref) >= window
}
