package health

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
)

var errDown = errors.New("postgres://user:secret@db:5432 connection refused")

func testConfig() config.Health {
	return config.Health{
		Interval:        10 * time.Second,
		InitialInterval: time.Second,
		Timeout:         2 * time.Second,
		MinInterval:     time.Second,
	}
}

func newTestMonitor(t *testing.T, cfg config.Health) *Monitor {
	t.Helper()

	m := New(cfg, logger.ForTests())
	m.jitter = func(d time.Duration) time.Duration { return d }

	return m
}

// start runs m in the background and returns a stop function that waits for Start.
func start(t *testing.T, m *Monitor) func() {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)

	go func() { done <- m.Start(ctx) }()

	synctest.Wait()

	return func() {
		cancel()
		require.NoError(t, <-done)
	}
}

// counter is a checker returning the configured error and counting calls.
type counter struct {
	calls atomic.Int32
	err   atomic.Pointer[error]
}

func (c *counter) set(err error) { c.err.Store(&err) }

func (c *counter) Check(context.Context) error {
	c.calls.Add(1)

	if err := c.err.Load(); err != nil {
		return *err
	}

	return nil
}

func TestRegisterValidation(t *testing.T) {
	m := newTestMonitor(t, testConfig())
	ok := CheckerFunc(func(context.Context) error { return nil })

	cases := map[string]struct {
		name string
		c    Checker
		opts []Option
	}{
		"empty name":         {name: " ", c: ok},
		"nil checker":        {name: "a"},
		"timeout > interval": {name: "a", c: ok, opts: []Option{WithInterval(time.Second)}},
		"negative interval":  {name: "a", c: ok, opts: []Option{WithInterval(-time.Second)}},
		"negative timeout":   {name: "a", c: ok, opts: []Option{WithTimeout(-time.Second)}},
		"bad thresholds":     {name: "a", c: ok, opts: []Option{WithThresholds(0, 1)}},
		"negative ttl":       {name: "a", c: ok, opts: []Option{WithTTL(-1)}},
		"unknown impact":     {name: "a", c: ok, opts: []Option{WithImpact(Impact(42))}},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			require.ErrorIs(t, m.Register(tt.name, tt.c, tt.opts...), ErrInvalidRegistration)
		})
	}

	require.NoError(t, m.Register("db", ok, WithInterval(5*time.Second), WithTimeout(time.Second),
		WithInitialInterval(500*time.Millisecond), nil))
	require.ErrorIs(t, m.Register("db", ok), ErrDuplicateCheck)
	require.ErrorIs(t, bonesErr(m.Status("db")), ErrDuplicateCheck)
	require.ErrorIs(t, bonesErr(m.Heartbeat("hb", 0)), ErrInvalidRegistration)
	require.ErrorIs(t, bonesErr(m.Heartbeat("db", time.Second)), ErrDuplicateCheck)
	require.ErrorIs(t, bonesErr(m.Status("")), ErrInvalidRegistration)

	snap := m.Snapshot()
	require.Equal(t, []string{"db"}, snap.Names())
	require.Equal(t, StatusUnknown, snap.Checks["db"].Status)
	require.False(t, snap.Live, "monitor is not started yet")
	require.Equal(t, ServiceName, m.Name())
	require.Equal(t, 10*time.Second, m.Config().Interval)
}

func bonesErr[T any](_ T, err error) error { return err }

func TestRegisterAfterStart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		stop := start(t, m)

		require.ErrorIs(
			t,
			m.Register("late", CheckerFunc(func(context.Context) error { return nil })),
			ErrMonitorStarted,
		)
		require.ErrorIs(t, m.Start(t.Context()), ErrMonitorStarted)

		stop()

		m.Stop(t.Context())
		require.ErrorIs(t, m.Start(t.Context()), ErrMonitorStopped)

		canceled, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, m.Start(canceled), context.Canceled,
			"Stop before Start during a shutdown is not a failure")
		require.ErrorIs(t, bonesErr(m.Status("late")), ErrMonitorStarted)
	})
}

func TestSchedule_InitialThenRegularInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		c := new(counter)
		c.set(errDown)
		require.NoError(t, m.Register("db", c))

		stop := start(t, m)
		defer stop()

		require.EqualValues(t, 1, c.calls.Load(), "first call happens right after Start")

		// failing: InitialInterval (1s) is used until the first success
		time.Sleep(3 * time.Second)
		synctest.Wait()
		require.EqualValues(t, 4, c.calls.Load())

		c.set(nil)
		time.Sleep(time.Second)
		synctest.Wait()
		require.EqualValues(t, 5, c.calls.Load())
		require.True(t, m.Snapshot().Ready)

		// passing: Interval (10s) from now on
		time.Sleep(9 * time.Second)
		synctest.Wait()
		require.EqualValues(t, 5, c.calls.Load())

		time.Sleep(time.Second)
		synctest.Wait()
		require.EqualValues(t, 6, c.calls.Load())
	})
}

func TestHungChecker(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())

		var calls atomic.Int32

		release := make(chan struct{})
		require.NoError(t, m.Register("hung", CheckerFunc(func(context.Context) error {
			calls.Add(1)
			<-release // ignores ctx on purpose

			return nil
		})))

		stop := start(t, m)
		defer stop()

		// readers never wait for the checker
		begin := time.Now()
		snap := m.Snapshot()
		require.Zero(t, time.Since(begin))
		require.Equal(t, StatusUnknown, snap.Checks["hung"].Status)
		require.False(t, snap.Ready)

		time.Sleep(2 * time.Second) // Timeout
		synctest.Wait()

		res := m.Snapshot().Checks["hung"]
		require.Equal(t, StatusFailing, res.Status)
		require.ErrorIs(t, res.Err, ErrTimeout)
		require.Equal(t, "timeout", PublicMessage(res.Err))

		// many ticks later there is still only one call in flight
		time.Sleep(30 * time.Second)
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load())
		require.True(
			t,
			m.Snapshot().Checks["hung"].Stale,
			"no fresh result within 2*interval+timeout",
		)

		close(release)
		time.Sleep(time.Second)
		synctest.Wait()
		require.EqualValues(t, 2, calls.Load())
		require.True(t, m.Snapshot().Ready)
	})
}

func TestReadiness_Thresholds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		c := new(counter)
		require.NoError(t, m.Register("db", c, WithThresholds(2, 2)))

		require.False(t, m.Snapshot().Ready, "unknown is not ready")

		stop := start(t, m)
		defer stop()

		require.True(t, m.Snapshot().Ready, "first success decides immediately")

		c.set(errDown)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		require.True(t, m.Snapshot().Ready, "one failure is below the threshold")
		require.Equal(t, 1, m.Snapshot().Checks["db"].ConsecutiveFailures)

		time.Sleep(10 * time.Second)
		synctest.Wait()

		snap := m.Snapshot()
		require.False(t, snap.Ready)
		require.True(t, snap.Live, "readiness failures never affect liveness")
		require.Equal(t, OverallFailing, snap.Overall)

		c.set(nil)
		time.Sleep(10 * time.Second)
		synctest.Wait()
		require.False(t, m.Snapshot().Ready, "one success is below the threshold")

		time.Sleep(10 * time.Second)
		synctest.Wait()
		require.True(t, m.Snapshot().Ready)
	})
}

func TestInformational_Degraded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		kafka, err := m.Status("kafka", WithImpact(Informational))
		require.NoError(t, err)
		require.Equal(t, "kafka", kafka.Name())

		stop := start(t, m)
		defer stop()

		snap := m.Snapshot()
		require.True(t, snap.Ready, "unknown informational check does not affect readiness")
		require.Equal(t, OverallOK, snap.Overall)

		kafka.Set(errDown)

		snap = m.Snapshot()
		require.True(t, snap.Ready)
		require.True(t, snap.Live)
		require.Equal(t, OverallDegraded, snap.Overall)
		require.Equal(t, "error", PublicMessage(snap.Checks["kafka"].Err))

		kafka.Set(nil)
		require.Equal(t, OverallOK, m.Snapshot().Overall)

		var nilHandle *StatusHandle
		require.NotPanics(t, func() { nilHandle.Set(nil) })
	})
}

func TestHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		hb, err := m.Heartbeat("worker", 5*time.Second)
		require.NoError(t, err)
		require.Equal(t, "worker", hb.Name())

		stop := start(t, m)
		defer stop()

		require.True(t, m.Snapshot().Live, "unknown liveness check is ok")

		time.Sleep(5 * time.Second)
		synctest.Wait()

		snap := m.Snapshot()
		require.False(t, snap.Live, "never beaten heartbeat goes stale after max age")
		require.False(t, snap.Ready)
		require.ErrorIs(t, snap.Checks["worker"].Err, ErrStale)
		require.ErrorIs(t, hb.Check(t.Context()), ErrStale)

		hb.Beat()
		require.True(t, m.Snapshot().Live)
		require.NoError(t, hb.Check(t.Context()))

		for range 10 {
			time.Sleep(4 * time.Second)
			hb.Beat()
		}

		synctest.Wait()
		require.True(t, m.Snapshot().Live, "regular beats keep it alive")
		require.Equal(t, time.Now(), m.Snapshot().Checks["worker"].CheckedAt)

		time.Sleep(6 * time.Second)
		synctest.Wait()
		require.False(t, m.Snapshot().Live)

		var nilBeat *Heartbeat
		require.NotPanics(t, nilBeat.Beat)
	})
}

func TestPushTTL(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		h, err := m.Status("cache", WithTTL(3*time.Second))
		require.NoError(t, err)

		never, err := m.Status("forever")
		require.NoError(t, err)

		stop := start(t, m)
		defer stop()

		h.Set(nil)
		never.Set(nil)

		time.Sleep(3 * time.Second)
		synctest.Wait()

		snap := m.Snapshot()
		require.True(t, snap.Checks["cache"].Stale)
		require.False(t, snap.Ready)
		require.False(t, snap.Checks["forever"].Stale, "push status without TTL never goes stale")
	})
}

func TestTrigger(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		c := new(counter)
		require.NoError(t, m.Register("db", c))
		push, err := m.Status("push")
		require.NoError(t, err)

		stop := start(t, m)
		defer stop()

		require.EqualValues(t, 1, c.calls.Load())

		// within MinInterval: coalesced and postponed
		for range 10 {
			m.Trigger("db")
		}

		m.Trigger("unknown")
		m.Trigger(push.Name())
		synctest.Wait()
		require.EqualValues(t, 1, c.calls.Load())

		time.Sleep(time.Second)
		synctest.Wait()
		require.EqualValues(t, 2, c.calls.Load(), "postponed run happens after MinInterval")

		time.Sleep(5 * time.Second)
		synctest.Wait()
		require.EqualValues(t, 2, c.calls.Load())

		c.set(errDown)
		m.Trigger("db")
		synctest.Wait()
		require.EqualValues(
			t,
			3,
			c.calls.Load(),
			"trigger runs immediately without waiting for a tick",
		)
		require.False(t, m.Snapshot().Ready)
	})
}

func TestPanics(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		require.NoError(
			t,
			m.Register("boom", CheckerFunc(func(context.Context) error { panic("boom") })),
		)

		other := new(counter)
		require.NoError(t, m.Register("other", other))

		unsubscribe := m.Subscribe(func(Event) { panic("subscriber") })
		defer unsubscribe()

		stop := start(t, m)
		defer stop()

		time.Sleep(3 * time.Second)
		synctest.Wait()

		snap := m.Snapshot()
		require.ErrorIs(t, snap.Checks["boom"].Err, ErrPanic)
		require.Equal(t, "panic", PublicMessage(snap.Checks["boom"].Err))
		require.Equal(t, StatusPassing, snap.Checks["other"].Status)
		require.EqualValues(t, 1, other.calls.Load())
	})
}

func TestSubscribe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		h, err := m.Status("db")
		require.NoError(t, err)

		var (
			mu     sync.Mutex
			events []Event
		)

		unsubscribe := m.Subscribe(func(ev Event) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		})
		noop := m.Subscribe(nil)
		require.NotNil(t, noop)
		require.NotPanics(t, noop)

		stop := start(t, m)
		defer stop()

		h.Set(nil)
		h.Set(errDown)
		m.Drain()
		m.Drain()
		synctest.Wait()

		mu.Lock()
		kinds := make([]string, 0, len(events))
		for _, ev := range events {
			kinds = append(kinds, ev.Kind.String()+":"+string(ev.To))
		}
		mu.Unlock()

		require.Equal(t, []string{
			"live:", "check:passing", "ready:", "check:failing", "not_ready:", "draining:",
		}, kinds)

		unsubscribe()
		unsubscribe()
		h.Set(nil)
		synctest.Wait()

		mu.Lock()
		require.Len(t, events, 6, "no events after unsubscribe")
		mu.Unlock()

		snap := m.Snapshot()
		require.True(t, snap.Draining)
		require.False(t, snap.Ready, "draining is permanent")
	})
}

func TestSubscribe_SlowSubscriberDoesNotBlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		h, err := m.Status("db")
		require.NoError(t, err)

		release := make(chan struct{})

		var got atomic.Int32

		unsubscribe := m.Subscribe(func(Event) {
			<-release
			got.Add(1)
		})
		defer unsubscribe()

		for i := range 200 {
			if i%2 == 0 {
				h.Set(nil)
			} else {
				h.Set(errDown)
			}
		}

		close(release)
		synctest.Wait()

		require.LessOrEqual(t, got.Load(), int32(subscriberBuffer+1))
		require.Positive(t, testCounterValue(t, m, "go_bones_health_events_dropped_total"))
	})
}

func TestStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())

		release := make(chan struct{})
		require.NoError(t, m.Register("hung", CheckerFunc(func(context.Context) error {
			<-release

			return nil
		})))

		done := make(chan error, 1)
		go func() { done <- m.Start(t.Context()) }()

		synctest.Wait()
		require.True(t, m.Snapshot().Live)

		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()

		begin := time.Now()
		m.Stop(ctx)
		require.Equal(
			t,
			time.Second,
			time.Since(begin),
			"Stop waits for in-flight checks up to the deadline",
		)
		require.NoError(t, <-done)

		snap := m.Snapshot()
		require.False(t, snap.Live)
		require.False(t, snap.Ready)
		require.True(t, snap.Draining)

		close(release)
		synctest.Wait()
	})
}

func TestStopBeforeStart(t *testing.T) {
	m := newTestMonitor(t, testConfig())
	m.Stop(t.Context())
	require.True(t, m.Snapshot().Draining)
}

func TestLogRepeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := logger.NewSyncBuffer()
		cfg := testConfig()
		cfg.LogRepeatInterval = 15 * time.Second
		m := New(cfg, logger.ForTests(logger.TestLoggerWriter(buf)))
		m.jitter = func(d time.Duration) time.Duration { return d }

		c := new(counter)
		c.set(errDown)
		require.NoError(t, m.Register("db", c))

		stop := start(t, m)
		defer stop()

		time.Sleep(16 * time.Second)
		synctest.Wait()

		out := buf.String()
		require.Contains(t, out, "health check failing")
		require.Contains(t, out, "health check still failing")

		c.set(nil)
		time.Sleep(time.Second)
		synctest.Wait()
		require.Contains(t, buf.String(), "health check passing")
		require.Contains(t, buf.String(), "downtime")
	})
}

// A check failing while its service is still starting stays unknown (not
// ready, no WARN) until StartPeriod is over.
func TestStartPeriod(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := logger.NewSyncBuffer()
		cfg := testConfig()
		cfg.StartPeriod = 5 * time.Second
		m := New(cfg, logger.ForTests(logger.TestLoggerWriter(buf)))
		m.jitter = func(d time.Duration) time.Duration { return d }

		c := new(counter)
		c.set(errDown)
		require.NoError(t, m.Register("db", c))

		stop := start(t, m)
		defer stop()

		time.Sleep(4 * time.Second)
		synctest.Wait()

		res := m.Snapshot().Checks["db"]
		require.Equal(t, StatusUnknown, res.Status)
		require.ErrorIs(t, res.Err, errDown)
		require.False(t, m.Snapshot().Ready)
		require.NotContains(t, buf.String(), "health check failing")

		time.Sleep(2 * time.Second)
		synctest.Wait()
		require.Equal(t, StatusFailing, m.Snapshot().Checks["db"].Status)
		require.Contains(t, buf.String(), "health check failing")
	})
}

func TestDefaultJitter(t *testing.T) {
	for range 100 {
		v := defaultJitter(10 * time.Second)
		assert.GreaterOrEqual(t, v, 9*time.Second)
		assert.LessOrEqual(t, v, 11*time.Second)
	}

	require.Equal(t, time.Duration(5), defaultJitter(5))
}

func TestSubscribe_UnsubscribeStopsQueuedDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		h, err := m.Status("db")
		require.NoError(t, err)

		entered := make(chan struct{}, 1)
		release := make(chan struct{})

		var got atomic.Int32

		unsubscribe := m.Subscribe(func(Event) {
			got.Add(1)
			entered <- struct{}{}
			<-release
		})

		h.Set(nil)
		<-entered
		h.Set(errDown)
		h.Set(nil)

		unsubscribe()
		close(release)
		synctest.Wait()

		require.EqualValues(t, 1, got.Load(), "queued events are not delivered after unsubscribe")
	})
}

// TestStartStopRace runs Stop concurrently with Start. Stop must not return
// before the check loops started by Start have exited; with -race it also
// catches WaitGroup.Add racing with Wait.
func TestStartStopRace(t *testing.T) {
	for range 200 {
		m := New(config.Health{}, logger.ForTests())

		var stopped atomic.Bool
		lateCalls := new(atomic.Int64)
		require.NoError(t, m.Register("db", CheckerFunc(func(context.Context) error {
			if stopped.Load() {
				lateCalls.Add(1)
			}

			return nil
		})))

		done := make(chan error, 1)
		go func() { done <- m.Start(t.Context()) }()

		m.Stop(t.Context())
		stopped.Store(true)
		<-done

		require.Zero(t, lateCalls.Load(), "check ran after Stop returned")
	}
}

func TestHeartbeat_LastSuccessFollowsBeats(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, testConfig())
		hb, err := m.Heartbeat("worker", 5*time.Second)
		require.NoError(t, err)

		stop := start(t, m)
		defer stop()

		hb.Beat()
		first := m.Snapshot().Checks["worker"].LastSuccess

		time.Sleep(time.Second)
		hb.Beat() // fast path: the check is already passing

		res := m.Snapshot().Checks["worker"]
		require.Equal(t, first.Add(time.Second), res.LastSuccess)
		require.Equal(t, res.CheckedAt, res.LastSuccess)
	})
}
