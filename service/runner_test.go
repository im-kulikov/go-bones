package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/im-kulikov/go-bones/logger"
)

type (
	testService  string
	mockService  string
	stuckService time.Duration
)

type fakeSink struct{ io.Writer }

var errTest = errors.New("test")

var (
	_ Service = testService("one")
	_ Service = mockService("two")
	_ Service = stuckService(1)
)

func (f *fakeSink) Close() error { return nil }

func (f *fakeSink) Sync() error { return nil }

func (stuckService) Name() string { return "stuck-service" }

func (s stuckService) Start(context.Context) error {
	time.Sleep(time.Duration(s))

	return nil
}

func (s stuckService) Stop(context.Context) { time.Sleep(time.Duration(s)) }

func (mockService) Name() string { return "mocked" }

func (mockService) Start(context.Context) error {
	time.Sleep(time.Millisecond * 2)

	return nil
}

func (mockService) Stop(context.Context) {}

func (ts testService) Name() string { return string(ts) }

func (ts testService) Enabled() bool { return strings.Contains(string(ts), "enabled") }

func (testService) Stop(context.Context) {}

func (ts testService) Start(ctx context.Context) error {
	if strings.Contains(string(ts), "error") {
		return errTest
	}

	<-ctx.Done()

	return ctx.Err()
}

func newTestService(name string) Service { return testService(name) }

func TestNew(t *testing.T) {
	svc1 := newTestService("service-enabled")
	svc2 := newTestService("service-disable")

	grp := New(logger.ForTests(t),
		WithService(NewGroup("multiple", svc1, svc2, nil)),
		WithShutdownTimeout(time.Millisecond))

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*10)
	defer cancel()

	require.NoError(t, grp.Run(ctx))
}

func TestGraceStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buf := new(bytes.Buffer)
	run := make(chan struct{})
	end := make(chan struct{})
	done := make(chan struct{})

	log, err := logger.New(logger.Config{
		EncodingConsole: true,
		Level:           zapcore.InfoLevel.String(),
		Trace:           zapcore.FatalLevel.String(),
	}, logger.WithCustomOutput("custom-output-name", &fakeSink{Writer: buf}))

	require.NoError(t, err)

	grp := New(log,
		WithShutdownTimeout(time.Millisecond),
		WithService(NewWorker("test-worker", func(ctx context.Context) error {
			close(done)

			<-ctx.Done()

			time.Sleep(time.Millisecond * 100)

			close(end)

			return ctx.Err()
		})))

	go func() {
		assert.NoError(t, grp.Run(ctx))

		close(run)
	}()

	<-done   // started
	cancel() // should be graceful shutdown
	<-end    // should be closed
	<-run    // wait for run

	require.Contains(t, buf.String(), "gracefully shutdown")
}

func TestRunner(t *testing.T) {
	t.Run("should do nothing on empty services", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		grp := New(logger.ForTests(t),

			// should ignore empty shutdown timeout
			WithShutdownTimeout(0),
			// should ignore ping-pong
			WithLoggerPingPong(0),
			// should add to ignore
			WithIgnoreError(context.Canceled),
			// should ignore nil errors
			WithIgnoreError(nil))

		require.NoError(t, grp.Run(ctx))
	})

	t.Run("should fail in service", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		svc := newTestService("service-with-enabled-error")

		grp := New(logger.ForTests(t),
			WithService(svc),
			WithLoggerPingPong(time.Millisecond),
			WithShutdownTimeout(time.Millisecond))

		require.EqualError(t, errors.Unwrap(grp.Run(ctx)), errTest.Error())
	})

	t.Run("should run ping-pong service multiple times", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		svc := newTestService("test-service-enabled")

		grp := New(logger.ForTests(t),
			WithService(svc),
			WithLoggerPingPong(time.Millisecond*25),
			WithShutdownTimeout(time.Millisecond))

		require.NoError(t, grp.Run(ctx))
	})

	t.Run("should not stuck when service unexpectedly stopped", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		now := time.Now()
		one := mockService("two")
		two := newTestService("test-service-enabled")

		grp := New(logger.ForTests(t),
			WithService(one),
			WithService(two),
			WithLoggerPingPong(time.Millisecond*25),
			WithShutdownTimeout(time.Millisecond*25))

		require.NoError(t, grp.Run(ctx))
		require.InDelta(t, time.Since(now), time.Millisecond*2, float64(time.Millisecond*20)) // possible 20ms lags
	})

	t.Run("should stop on graceful context deadlined", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		now := time.Now()
		one := stuckService(time.Millisecond * 2)
		two := newTestService("test-service-enabled")

		grp := New(logger.ForTests(t),
			WithService(one),
			WithService(two),
			WithLoggerPingPong(time.Millisecond*25),
			WithShutdownTimeout(time.Nanosecond))

		require.NoError(t, grp.Run(ctx))
		require.InDelta(t, time.Since(now), time.Millisecond*5, float64(time.Millisecond*5)) // 5ms lags
	})

	t.Run("should ignore empty or disabled services", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		now := time.Now()
		one := (Service)(nil)
		two := newTestService("two")
		thr := newTestService("three-disabled")

		grp := New(logger.ForTests(t),
			WithService(one),
			WithService(two),
			WithService(thr),
			WithLoggerPingPong(time.Millisecond*25),
			WithShutdownTimeout(time.Nanosecond))

		require.NoError(t, grp.Run(ctx))
		require.InDelta(t, time.Since(now), time.Millisecond*5, float64(time.Millisecond*5)) // 5ms lags
	})
}
