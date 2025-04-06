package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

type workers struct {
	launchers []*launcher
}

func newWorkers(l *logger.Logger, shutdown ...func(context.Context)) *workers {
	var launchers []*launcher

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
		}, shutdown...)

		launchers = append(launchers, wrk.(*launcher))
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
			NewLauncher("simple", nil).Start(context.TODO()),
			ErrEmptyLauncher)
	})

	t.Run("should not run launcher on cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.TODO())
		cancel()

		log := logger.ForTests()
		wrk := NewLauncher("simple", func(top context.Context) error {
			<-top.Done()

			return context.Cause(top)
		})

		require.NoError(t, RunContext(ctx, log, WithService(wrk)))
	})

	t.Run("should not be blocked", func(t *testing.T) {
		wrk := NewLauncher("test", func(ctx context.Context) error {
			<-ctx.Done()

			time.Sleep(time.Second)

			return nil
		})

		{ // when we stop without start, we should wait
			ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond)
			defer cancel()

			now := time.Now()
			require.NotPanics(t, func() { wrk.Stop(ctx) })

			// should exit from launcher.Stop on context.DeadlineExceeded
			require.Greater(t, time.Since(now), time.Millisecond)
		}

		{ // when start and stop
			now := time.Now()

			ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*10)
			defer cancel()

			go func() { assert.NoError(t, wrk.Start(ctx)) }()

			<-time.After(time.Millisecond * 5)

			require.NotPanics(t, func() { wrk.Stop(ctx) })
			require.InDelta(
				t,
				time.Since(now),
				time.Millisecond*12,
				float64(time.Millisecond*5),
			) // 5ms lags
		}
	})

	t.Run("should run multiple workers and stop all", func(t *testing.T) {
		log := logger.ForTests()
		wrk := newWorkers(log)

		log.Info("test")

		ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*200)
		defer cancel()

		var wg sync.WaitGroup
		done := make(chan struct{})

		wg.Add(1)
		go func() {
			defer wg.Done()

			close(done)
			assert.NoError(t, RunContext(ctx, log, wrk.Options()...))
		}()

		<-done
		<-ctx.Done()

		wg.Wait()
	})
}

func Test_onShutdown(t *testing.T) {
	var inc atomic.Int32

	fun := func(context.Context) {
		t.Helper()
		assert.NotEmpty(t, inc.Add(1))
	}

	log := logger.ForTests()
	wrk := newWorkers(log, fun)
	ctx, cancel := context.WithTimeout(context.TODO(), time.Millisecond*100)
	defer cancel()

	options := wrk.Options()
	require.NoError(t, RunContext(ctx, log, options...))
	require.Equal(t, int32(len(wrk.launchers)), inc.Load())
}
