package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

type workers struct {
	launchers []*launcher
}

func newWorkers(l *logger.Logger, shutdown ...func(context.Context)) *workers {
	launchers := make([]*launcher, 10)

	for i := range 10 {
		num := fmt.Sprintf("worker_%02d", i)
		log := logger.Named(l, num)

		wrk := NewLauncher(num, func(ctx context.Context) error {
			tick := time.NewTicker(time.Millisecond * 25)
			defer tick.Stop()

			cnt := 1

			for {
				select {
				case <-ctx.Done():
					return nil
				case <-tick.C:
					log.InfoContext(ctx, "tick",
						logger.String("name", num),
						logger.Int("count", cnt))

					cnt++
				}
			}
		},
			WithLauncherLogger(log),
			WithLauncherShutdownHooks(shutdown...))

		launchers[i] = wrk.(*launcher)
	}

	return &workers{launchers: launchers}
}

func (w *workers) Options() []Option {
	var options []Option
	for _, wrk := range w.launchers {
		if wrk != nil {
			options = append(options, WithService(wrk))
		}
	}

	return append(options, WithShutdownTimeout(time.Millisecond*100))
}

func Test_Workers(t *testing.T) {
	t.Run("should fail on empty launcher", func(t *testing.T) {
		require.ErrorIs(t,
			NewLauncher("simple", nil).Start(t.Context()),
			ErrEmptyLauncher)
	})

	t.Run("should reject start after stop has begun", func(t *testing.T) {
		started := make(chan struct{})
		wrk := NewLauncher("simple", func(ctx context.Context) error {
			close(started)
			<-ctx.Done()

			return context.Cause(ctx)
		})

		runDone := make(chan error, 1)
		go func() {
			runDone <- wrk.Start(t.Context())
		}()

		<-started

		stopCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		wrk.Stop(stopCtx)

		require.ErrorIs(t, wrk.Start(t.Context()), ErrStopsLauncher)
		require.ErrorIs(t, <-runDone, context.Canceled)
	})

	t.Run("should cancel even if stop races with cancel publication", func(t *testing.T) {
		wrk := &launcher{
			name: "simple",
			done: make(chan struct{}),
			logs: logger.ForTests(),
		}
		wrk.init.Store(true)

		cancelled := make(chan struct{})
		var cancel context.CancelFunc = func() {
			close(cancelled)
			close(wrk.done)
		}

		go func() {
			time.Sleep(25 * time.Millisecond)
			wrk.cancel.Store(&cancel)
		}()

		stopCtx, stopCancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer stopCancel()

		stopped := make(chan struct{})
		go func() {
			wrk.Stop(stopCtx)
			close(stopped)
		}()

		select {
		case <-cancelled:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Stop missed cancel publication")
		}

		select {
		case <-stopped:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Stop did not return after cancel")
		}
	})

	t.Run(
		"should return when stop context is already done while cancel is unpublished",
		func(t *testing.T) {
			wrk := &launcher{
				name: "simple",
				done: make(chan struct{}),
				logs: logger.ForTests(),
			}
			wrk.init.Store(true)

			stopCtx, stopCancel := context.WithCancel(t.Context())
			stopCancel()

			now := time.Now()
			require.NotPanics(t, func() { wrk.Stop(stopCtx) })
			require.Less(t, time.Since(now), 10*time.Millisecond)
		})

	t.Run("should not run launcher on cancelled context", func(t *testing.T) {
		log := logger.ForTests(logger.TestLoggerWriteToTB(t))
		ctx, cancel := context.WithTimeout(t.Context(), time.Nanosecond)
		defer cancel()

		wrk := NewLauncher("simple",
			func(top context.Context) error {
				<-top.Done()

				return context.Cause(top)
			}, WithLauncherLogger(log))

		require.NoError(t, RunContext(ctx, log,
			WithShutdownTimeout(time.Nanosecond),
			WithService(wrk)))
	})

	t.Run("should not be blocked", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			wrk := NewLauncher("test", func(ctx context.Context) error {
				<-ctx.Done()

				return nil
			})

			{ // when we stop without a start, stop should return immediately
				ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
				defer cancel()

				now := time.Now()
				require.NotPanics(t, func() { wrk.Stop(ctx) })
				require.Zero(t, time.Since(now))
			}
		})

		synctest.Test(t, func(t *testing.T) {
			started := make(chan struct{})
			released := make(chan struct{})
			wrk := NewLauncher("test", func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				<-released

				return nil
			})

			runDone := make(chan error, 1)
			go func() { runDone <- wrk.Start(t.Context()) }()

			<-started
			synctest.Wait()

			stopCtx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			defer cancel()

			start := time.Now()
			require.NotPanics(t, func() { wrk.Stop(stopCtx) })
			require.Equal(t, 10*time.Millisecond, time.Since(start))

			close(released)
			synctest.Wait()
			require.NoError(t, <-runDone)
		})
	})

	t.Run("should run multiple workers and stop all", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			log := logger.ForTests()
			wrk := newWorkers(log)

			log.Info("test")

			ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*200)
			defer cancel()

			done := make(chan struct{})
			var wg sync.WaitGroup
			wg.Go(func() {
				close(done)
				assert.NoError(t, RunContext(ctx, log, wrk.Options()...))
			})

			<-done
			synctest.Wait()

			wg.Wait()
		})
	})
}

func Test_onShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var inc atomic.Int32

		fun := func(context.Context) {
			t.Helper()
			assert.NotEmpty(t, inc.Add(1))
		}

		log := logger.ForTests()
		wrk := newWorkers(log, fun)
		ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*100)
		defer cancel()

		options := wrk.Options()
		require.NoError(t, RunContext(ctx, log, options...))
		require.Equal(t, int32(len(wrk.launchers)), inc.Load())
	})
}
