package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

func TestNewTicker_RunsOnStartAndEveryInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var runs atomic.Int32

		svc := NewTicker("sweep", time.Minute, func(context.Context) error {
			runs.Add(1)

			return nil
		}, nil)
		require.Equal(t, "sweep", svc.Name())

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- svc.Start(ctx) }()

		synctest.Wait()
		require.Equal(t, int32(1), runs.Load(), "runs right after start")

		time.Sleep(time.Minute)
		synctest.Wait()
		require.Equal(t, int32(2), runs.Load())

		time.Sleep(3 * time.Minute)
		synctest.Wait()
		require.Equal(t, int32(5), runs.Load())

		cancel()
		require.NoError(t, <-done)
	})
}

func TestNewTicker_TimeoutAndErrors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := logger.NewSyncBuffer()
		release := make(chan struct{})

		svc := NewTicker("slow", time.Minute, func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-release: // shutdown path: the error below must not be logged
				return errors.New("interrupted")
			}
		}, WithTickerTimeout(time.Second), WithTickerLogger(logger.ForTests(logger.TestLoggerWriter(buf))))

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- svc.Start(ctx) }()

		time.Sleep(time.Second)
		synctest.Wait()
		require.Contains(t, buf.String(), "[slow] periodic task failed")
		require.Contains(t, buf.String(), "context deadline exceeded")

		time.Sleep(time.Minute) // second run is in flight
		synctest.Wait()
		cancel()
		close(release)
		require.NoError(t, <-done)
		require.NotContains(t, buf.String(), "interrupted", "errors after stop are not logged")
	})
}

func TestTicker_Options(t *testing.T) {
	tk := &ticker{every: time.Minute}

	WithTickerJitter(-1)(tk)
	require.Zero(t, tk.jitter)
	require.Equal(t, time.Minute, tk.next(), "no jitter")

	WithTickerJitter(5)(tk)
	require.InDelta(t, 1.0, tk.jitter, 0)

	WithTickerJitter(0.1)(tk)
	for range 100 {
		next := tk.next()
		require.GreaterOrEqual(t, next, 54*time.Second)
		require.LessOrEqual(t, next, 66*time.Second)
	}

	WithTickerTimeout(-time.Second)(tk)
	require.Zero(t, tk.timeout)

	WithTickerLogger(nil)(tk)
	require.Nil(t, tk.log)
}

func TestNewTicker_NonPositiveInterval(t *testing.T) {
	require.PanicsWithValue(t, "service: non-positive interval for NewTicker", func() {
		NewTicker("bad", 0, func(context.Context) error { return nil })
	})
}
