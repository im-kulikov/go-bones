package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

type workers struct {
	launchers []*worker
}

func newWorkers(log logger.Logger) *workers {
	var launchers []*worker

	for i := 0; i < 10; i++ {
		num := fmt.Sprintf("worker_%02d", i)

		wrk := NewWorker(num, func(ctx context.Context) error {
			tick := time.NewTicker(time.Millisecond * 25)
			defer tick.Stop()

			cnt := 1

			for {
				select {
				case <-ctx.Done():
					return nil
				case <-tick.C:
					log.Infof("tick name:%s counter:%d", num, cnt)

					cnt++
				}
			}
		})

		launchers = append(launchers, wrk.(*worker))
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
		require.EqualError(t,
			NewWorker("simple", nil).Start(context.Background()),
			errEmptyLauncher.Error())
	})

	t.Run("should not be blocked", func(t *testing.T) {
		wrk := NewWorker("test", func(ctx context.Context) error {
			<-ctx.Done()

			time.Sleep(time.Second)

			return nil
		})

		{ // when we stop without start, we should not wait
			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
			defer cancel()

			now := time.Now()
			require.NotPanics(t, func() { wrk.Stop(ctx) })

			// should exit from worker.Stop immediately
			require.Less(t, time.Since(now), time.Millisecond/2)
		}

		{ // when start and stop
			now := time.Now()

			ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*10)
			defer cancel()

			go func() { assert.NoError(t, wrk.Start(ctx)) }()

			<-time.After(time.Millisecond * 5)

			require.NotPanics(t, func() { wrk.Stop(ctx) })
			require.InDelta(t, time.Since(now), time.Millisecond*12, float64(time.Millisecond*5)) // 5ms lags
		}
	})

	t.Run("should run multiple workers and stop all", func(t *testing.T) {
		log := logger.ForTests(t)
		wrk := newWorkers(log)
		grp := New(log, wrk.Options()...)

		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		require.NoError(t, grp.Run(ctx))

		<-ctx.Done()
	})
}
