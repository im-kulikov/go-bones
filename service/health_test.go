package service

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/logger"
)

// journal records lifecycle events in order.
type journal struct {
	mu     sync.Mutex
	events []string
}

func (j *journal) add(event string) {
	j.mu.Lock()
	j.events = append(j.events, event)
	j.mu.Unlock()
}

func (j *journal) list() []string {
	j.mu.Lock()
	defer j.mu.Unlock()

	return append([]string(nil), j.events...)
}

// fakeService blocks in Start until ctx is canceled and records Start/Stop.
// Like real services, Stop waits for Start to return.
type fakeService struct {
	name string
	log  *journal
	err  error
	done chan struct{}
	once sync.Once
}

func (f *fakeService) Name() string { return f.name }

func (f *fakeService) exited() chan struct{} {
	f.once.Do(func() { f.done = make(chan struct{}) })

	return f.done
}

func (f *fakeService) Start(ctx context.Context) error {
	defer close(f.exited())

	f.log.add("start:" + f.name)

	if f.err != nil {
		return f.err
	}

	<-ctx.Done()
	f.log.add("canceled:" + f.name)

	return nil
}

func (f *fakeService) Stop(ctx context.Context) {
	select {
	case <-f.exited():
	case <-ctx.Done():
	}

	f.log.add("stop:" + f.name)
}

// checkedService is a fakeService that implements HealthChecker and Configurer.
type checkedService struct {
	fakeService

	opts []health.Option
}

func (c *checkedService) Check(context.Context) error { return nil }

func (c *checkedService) HealthOptions() []health.Option { return c.opts }

type disabledChecked struct{ checkedService }

func (*disabledChecked) Enabled() bool { return false }

// fakeSignals makes RunContext controllable: the returned function emulates an
// OS signal (the first call triggers shutdown, next calls are "second signals").
func fakeSignals() (Option, func(os.Signal)) {
	var (
		mu      sync.Mutex
		trigger context.CancelCauseFunc
		second  = make(chan os.Signal, 1)
	)

	opt := func(g *settings) {
		g.newSignal = func(top context.Context, _ ...os.Signal) (context.Context, context.CancelCauseFunc, handler) {
			ctx, cancel := context.WithCancelCause(top)

			mu.Lock()
			trigger = cancel
			mu.Unlock()

			return ctx, cancel, func() { <-ctx.Done() }
		}
		g.notify = func(...os.Signal) (<-chan os.Signal, func()) { return second, func() {} }
	}

	sent := 0
	send := func(sig os.Signal) {
		mu.Lock()
		defer mu.Unlock()

		sent++
		if sent == 1 {
			trigger(ErrReceivedSignal(sig))

			return
		}

		second <- sig
	}

	return opt, send
}

// requireStoppedBefore checks that first was canceled and stopped before last
// was canceled (cancel and Stop of one service may interleave).
func requireStoppedBefore(t *testing.T, events []string, first, last string) {
	t.Helper()

	idx := func(event string) int {
		i := slices.Index(events, event)
		require.NotEqual(t, -1, i, event)

		return i
	}

	firstDone := max(idx("canceled:"+first), idx("stop:"+first))
	lastBegin := min(idx("canceled:"+last), idx("stop:"+last))
	require.Less(t, firstDone, lastBegin, "%v", events)
}

func newMonitor() *health.Monitor {
	return health.New(config.Health{}, logger.ForTests())
}

func TestWithHealth_AutoRegistration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		j := new(journal)
		hc := newMonitor()

		db := &checkedService{fakeService: fakeService{name: "db", log: j}}
		cache := &checkedService{
			fakeService: fakeService{name: "cache", log: j},
			opts:        []health.Option{health.WithImpact(health.Informational)},
		}
		off := &disabledChecked{checkedService{fakeService: fakeService{name: "off", log: j}}}
		worker := NewLauncher("worker", func(ctx context.Context) error {
			<-ctx.Done()

			return nil
		}, WithLauncherHealthCheck(func(context.Context) error { return nil }))
		upstream := NewLauncher("upstream", func(ctx context.Context) error {
			<-ctx.Done()

			return nil
		}, WithLauncherHealthCheck(func(context.Context) error { return nil },
			health.WithImpact(health.Informational)))
		plain := &fakeService{name: "plain", log: j}

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)

		go func() {
			signals, _ := fakeSignals()
			done <- RunContext(ctx, logger.ForTests(), signals,
				WithHealth(hc),
				WithHealth(nil),
				WithService(Compose(db, cache, off), worker, upstream, plain, hc))
		}()

		synctest.Wait()

		snap := hc.Snapshot()
		require.Equal(t, []string{"cache", "db", "upstream", "worker"}, snap.Names())
		require.Equal(
			t,
			health.Informational,
			snap.Checks["upstream"].Impact,
			"launcher check options",
		)
		require.Equal(t, health.Readiness, snap.Checks["worker"].Impact)
		require.Equal(t, health.Informational, snap.Checks["cache"].Impact)
		require.Equal(t, health.Readiness, snap.Checks["db"].Impact)
		require.True(t, snap.Ready)

		_, ok := worker.(HealthChecker)
		require.True(t, ok)

		cancel()
		require.NoError(t, <-done, "monitor passed twice is started once")
		require.True(t, hc.Snapshot().Draining)
	})
}

func TestWithHealth_RegistrationErrorStopsBeforeStart(t *testing.T) {
	j := new(journal)
	a := &checkedService{fakeService: fakeService{name: "dup", log: j}}
	b := &checkedService{fakeService: fakeService{name: "dup", log: j}}

	err := RunContext(t.Context(), logger.ForTests(), WithHealth(newMonitor()), WithService(a, b))
	require.ErrorIs(t, err, health.ErrDuplicateCheck)
	require.Empty(t, j.list(), "no service is started")
}

func TestWithHealth_DrainOnSignal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		j := new(journal)
		hc := newMonitor()
		api := &fakeService{name: "api", log: j}
		ops := &fakeService{name: "ops", log: j}
		signals, send := fakeSignals()

		done := make(chan error, 1)

		go func() {
			done <- RunContext(t.Context(), logger.ForTests(), signals,
				WithHealth(hc),
				WithDrainDelay(5*time.Second),
				WithDrainDelay(-1),
				WithShutdownLast(ops, nil),
				WithShutdownTimeout(time.Second),
				WithService(api, ops))
		}()

		synctest.Wait()
		require.True(t, hc.Snapshot().Ready)

		begin := time.Now()
		send(syscall.SIGTERM)
		synctest.Wait()

		snap := hc.Snapshot()
		require.False(t, snap.Ready, "readiness is withdrawn immediately")
		require.True(t, snap.Draining)
		require.NotContains(t, j.list(), "canceled:api", "servers keep serving during drain delay")

		require.NoError(t, <-done)
		require.Equal(t, 5*time.Second, time.Since(begin))

		requireStoppedBefore(t, j.list(), "api", "ops")
	})
}

func TestWithHealth_SecondSignalSkipsDrainDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		j := new(journal)
		signals, send := fakeSignals()
		done := make(chan error, 1)

		go func() {
			done <- RunContext(t.Context(), logger.ForTests(), signals,
				WithHealth(newMonitor()),
				WithDrainDelay(time.Minute),
				WithService(&fakeService{name: "api", log: j}))
		}()

		synctest.Wait()

		begin := time.Now()
		send(syscall.SIGTERM)
		synctest.Wait()

		time.Sleep(time.Second)
		send(syscall.SIGTERM)

		require.NoError(t, <-done)
		require.Equal(t, time.Second, time.Since(begin))
	})
}

func TestWithHealth_FailureSkipsDrainDelay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		j := new(journal)
		errBoom := errors.New("boom")
		hc := newMonitor()

		signals, _ := fakeSignals()
		begin := time.Now()
		err := RunContext(
			t.Context(),
			logger.ForTests(),
			signals,
			WithHealth(hc),
			WithDrainDelay(time.Minute),
			WithService(
				&fakeService{name: "api", log: j},
				&fakeService{name: "bad", log: j, err: errBoom},
			),
		)

		require.ErrorIs(t, err, errBoom)
		require.Zero(t, time.Since(begin))
		require.True(t, hc.Snapshot().Draining)
	})
}

func TestWithShutdownLast_WithoutHealth(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		j := new(journal)
		ctx, cancel := context.WithCancel(t.Context())
		first := &fakeService{name: "first", log: j}
		last := &fakeService{name: "last", log: j}

		go func() {
			time.Sleep(time.Second)
			cancel()
		}()

		signals, _ := fakeSignals()
		require.NoError(t, RunContext(ctx, logger.ForTests(), signals,
			WithShutdownLast(last),
			WithService(last, first)))

		requireStoppedBefore(t, j.list(), "first", "last")
	})
}

func TestSameService(t *testing.T) {
	a := &fakeService{name: "a"}
	group := Compose(a)

	require.True(t, sameService(a, a))
	require.False(t, sameService(a, &fakeService{name: "a"}))
	require.False(t, sameService(group, group), "non-comparable services are never equal")
	require.True(t, sameService(nil, nil))
	require.False(t, sameService(a, nil))
}

func TestNotifySignals(t *testing.T) {
	ch, release := notifySignals(syscall.SIGUSR2)
	defer release()

	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, process.Signal(syscall.SIGUSR2))

	select {
	case sig := <-ch:
		require.Equal(t, syscall.SIGUSR2, sig)
	case <-time.After(time.Second):
		t.Fatal("signal not delivered")
	}
}

// ctxProbe records whether its context was already done when Start ran.
type ctxProbe struct {
	name       string
	doneAtBoot bool
}

func (p *ctxProbe) Name() string { return p.name }

func (p *ctxProbe) Start(ctx context.Context) error {
	p.doneAtBoot = ctx.Err() != nil
	<-ctx.Done()

	return nil
}

func (p *ctxProbe) Stop(context.Context) {}

func TestRunContext_CanceledParent(t *testing.T) {
	phased := []struct {
		name string
		opts []Option
	}{
		{name: "classic"},
		{name: "phased", opts: []Option{WithDrainDelay(time.Second)}},
	}

	for _, tc := range phased {
		t.Run(tc.name+": services start with a canceled context", func(t *testing.T) {
			// Real scheduler on purpose: the phased race depended on which
			// goroutine ran first, so repeat it instead of using synctest.
			for range 100 {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()

				probes := []*ctxProbe{{name: "a"}, {name: "b"}, {name: "c"}}
				signals, _ := fakeSignals()
				opts := append([]Option{
					signals,
					WithService(probes[0], probes[1], probes[2]),
				}, tc.opts...)

				require.NoError(t, RunContext(ctx, logger.ForTests(), opts...))

				for _, p := range probes {
					require.True(t, p.doneAtBoot, "%s: Start must not see a live context", p.name)
				}
			}
		})

		t.Run(tc.name+": a non-ignored parent cause is returned", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				errAborted := errors.New("startup aborted")
				ctx, cancel := context.WithCancelCause(t.Context())

				signals, _ := fakeSignals()
				opts := append([]Option{signals, WithService(&ctxProbe{name: "api"})}, tc.opts...)

				done := make(chan error, 1)
				go func() { done <- RunContext(ctx, logger.ForTests(), opts...) }()

				synctest.Wait()
				cancel(errAborted)

				require.ErrorIs(t, <-done, errAborted)
			})
		})
	}
}

func TestWithLauncherHealthCheck_ClonesOptions(t *testing.T) {
	opts := []health.Option{health.WithImpact(health.Informational)}
	svc := NewLauncher("worker", func(context.Context) error { return nil },
		WithLauncherHealthCheck(func(context.Context) error { return nil }, opts...))

	opts[0] = nil // the caller reuses its slice
	got := svc.(health.Configurer).HealthOptions()
	require.NotNil(t, got[0], "options are copied when the launcher is built")

	got[0] = nil
	require.NotNil(t, svc.(health.Configurer).HealthOptions()[0], "and when they are handed out")
}
