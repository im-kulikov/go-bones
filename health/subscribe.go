package health

import (
	"sync"

	"github.com/im-kulikov/go-bones/logger"
)

const subscriberBuffer = 64

// subscriber delivers events to one callback from its own goroutine, so a slow
// subscriber never blocks checks or other subscribers.
type subscriber struct {
	fn    func(Event)
	mu    sync.Mutex
	queue []Event
	wake  chan struct{}
	done  chan struct{}
	once  sync.Once
}

// Subscribe registers fn to be called on every transition. Events are only a
// hint that something changed: re-read Snapshot in fn. If fn is slower than
// the event rate, the oldest queued events are dropped (up to 64 are kept).
// A panic in fn is recovered and logged. Call unsubscribe to release the
// delivery goroutine.
func (m *Monitor) Subscribe(fn func(Event)) (unsubscribe func()) {
	if fn == nil {
		return func() {}
	}

	sub := &subscriber{
		fn:   fn,
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}

	m.subsMu.Lock()
	id := m.nextSub
	m.nextSub++
	m.subs[id] = sub
	m.subsMu.Unlock()

	go m.deliver(sub)

	return func() {
		sub.once.Do(func() {
			m.subsMu.Lock()
			delete(m.subs, id)
			m.subsMu.Unlock()
			close(sub.done)
		})
	}
}

// emit queues ev for every subscriber without blocking.
func (m *Monitor) emit(ev Event) {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()

	for _, sub := range m.subs {
		if sub.push(ev) {
			m.metrics.dropped()
			m.log.Warn("health subscriber is too slow, dropping the oldest event",
				logger.String("kind", ev.Kind.String()),
				logger.String("check", ev.Name))
		}
	}
}

// push appends ev and reports whether the oldest event had to be dropped.
func (s *subscriber) push(ev Event) (dropped bool) {
	s.mu.Lock()
	if len(s.queue) >= subscriberBuffer {
		s.queue = s.queue[1:]
		dropped = true
	}

	s.queue = append(s.queue, ev)
	s.mu.Unlock()

	select {
	case s.wake <- struct{}{}:
	default:
	}

	return dropped
}

func (s *subscriber) pop() (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.queue) == 0 {
		return Event{}, false
	}

	ev := s.queue[0]
	s.queue = s.queue[1:]

	return ev, true
}

func (m *Monitor) deliver(sub *subscriber) {
	for {
		select {
		case <-sub.done:
			return
		case <-sub.wake:
		}

		for ev, ok := sub.pop(); ok; ev, ok = sub.pop() {
			select {
			case <-sub.done:
				return
			default:
			}

			m.call1(sub.fn, ev)
		}
	}
}

// call1 invokes a subscriber callback with a panic guard.
func (m *Monitor) call1(fn func(Event), ev Event) {
	defer func() {
		if rec := recover(); rec != nil {
			m.log.Error("health subscriber panicked",
				logger.String("kind", ev.Kind.String()),
				logger.String("check", ev.Name),
				logger.Any("panic", rec))
		}
	}()

	fn(ev)
}
