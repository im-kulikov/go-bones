package health

import (
	"context"
	"time"
)

// StatusHandle sets the state of a push check registered with Monitor.Status.
type StatusHandle struct {
	m *Monitor
	r *registration
}

// Set records the current state synchronously and without I/O: nil means
// passing, any error means failing (subject to thresholds). It is safe for
// concurrent use, including from driver callbacks.
func (h *StatusHandle) Set(err error) {
	if h == nil {
		return
	}

	err, kind := classify(err, 0)
	h.m.record(h.r, err, kind, -1)
}

// Name returns the registered check name.
func (h *StatusHandle) Name() string { return h.r.name }

// Heartbeat is a Liveness push check that fails when Beat is not called in time.
//
// Call Beat on every iteration of the watched loop. A worker that may block
// while waiting for work must also wake up on a ticker, for example:
//
//	tick := time.NewTicker(maxAge / 3)
//	for {
//		hb.Beat()
//		select {
//		case <-ctx.Done():
//			return
//		case job := <-jobs:
//			handle(job)
//		case <-tick.C:
//		}
//	}
type Heartbeat struct {
	m *Monitor
	r *registration
}

// Beat reports that the watched loop is alive. The fast path is a single
// atomic store; the snapshot is rebuilt only when the check was not passing.
func (b *Heartbeat) Beat() {
	if b == nil {
		return
	}

	b.r.lastBeat.Store(time.Now().UnixNano())

	if !b.r.up.Load() {
		b.m.record(b.r, nil, resultSuccess, -1)
	}
}

// Name returns the registered check name.
func (b *Heartbeat) Name() string { return b.r.name }

// Check makes Heartbeat usable as a Checker in tests and adapters: it reports
// ErrStale when the last Beat is older than the max age.
func (b *Heartbeat) Check(context.Context) error {
	if b.m.Snapshot().Checks[b.r.name].Stale {
		return ErrStale
	}

	return nil
}
