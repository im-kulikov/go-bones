package service

import (
	"context"
	"errors"
	"math"
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
	require.InDelta(t, maxJitter, tk.jitter, 0)

	for range 100 {
		require.GreaterOrEqual(t, tk.next(), 30*time.Second, "at least half an interval")
	}

	// NaN would make every interval negative and spin the timer.
	WithTickerJitter(math.NaN())(tk)
	require.Zero(t, tk.jitter)
	require.Equal(t, time.Minute, tk.next(), "NaN is no jitter")

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

// lateCancel passes the launcher's own Err check once and is canceled after
// that, so the first tick and ctx.Done are both ready in the ticker's select.
type lateCancel struct {
	context.Context

	checked atomic.Bool
}

func (c *lateCancel) Err() error {
	if c.checked.CompareAndSwap(false, true) {
		return nil
	}

	return c.Context.Err()
}

func TestNewTicker_NoRunAfterCancel(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	var runs atomic.Int32

	for range 100 {
		svc := NewTicker("late", time.Hour, func(context.Context) error {
			runs.Add(1)

			return nil
		})
		require.NoError(t, svc.Start(&lateCancel{Context: canceled}))
	}

	require.Zero(t, runs.Load())
}

func TestTicker_JitterNeverOverflows(t *testing.T) {
	huge := &ticker{every: math.MaxInt64 - time.Second, jitter: 1}

	for range 1000 {
		require.Positive(t, huge.next())
	}
}
