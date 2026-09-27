package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones"
	"github.com/im-kulikov/go-bones/logger"
)

func Test_defaultErrorsIgnore(t *testing.T) {
	cases := []struct {
		name string
		errs error
	}{
		{name: "default", errs: ErrOsSignal},
		{name: "signals", errs: fmt.Errorf("%w: %v", ErrOsSignal, os.Interrupt)},
		{name: "manual cancel", errs: ErrCancelCalled},
		{name: "context cancel", errs: context.Canceled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, containsError(tc.errs, defaultIgnoredErrors...))
			require.True(t, containsError(tc.errs, errors.Join(defaultIgnoredErrors...)))
		})
	}
}

func TestRun_Success(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf := logger.NewSyncBuffer()
		log := logger.ForTests(
			logger.TestLoggerWriter(buf),
			logger.TestLoggerWriteToTB(t))

		mockSvc := new(mockService)
		mockSvc.name = "testService"
		mockSvc.On("Start", mock.Anything).
			Run(func(args mock.Arguments) {
				<-args.Get(0).(context.Context).Done()
			}).
			Return(nil).
			Once()
		mockSvc.On("Stop", mock.Anything).Return().Once()

		options := []Option{
			func(cfg *settings) {
				cfg.handle = append(cfg.handle, mockSvc)
				cfg.shutdown = time.Second
				cfg.signal = append(cfg.signal, syscall.SIGUSR1)
				cfg.newSignal = func(top context.Context, signal ...os.Signal) (context.Context, context.CancelCauseFunc, handler) {
					ctx, cancel := context.WithCancelCause(top)

					return ctx, cancel, func() {
						<-ctx.Done()
						cancel(ErrCancelCalled)
					}
				}
			},
			WithService(NewLauncher("test", func(ctx context.Context) error {
				<-ctx.Done()

				return ctx.Err()
			})),
		}

		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(ErrCancelCalled)

		require.NoError(t, Run(log)) // empty handlers

		go func() {
			// RunContext starts services asynchronously. This is virtual time inside
			// synctest, so it does not slow the suite down.
			time.Sleep(time.Millisecond * 100)
			cancel(ErrReceivedSignal(syscall.SIGUSR1))
		}()

		errChan := make(chan error, 1)
		defer close(errChan)
		errChan <- RunContext(ctx, log, options...)

		select {
		case err := <-errChan:
			require.NoErrorf(t, err, "%v", err)
			require.Contains(t, buf.String(), syscall.SIGUSR1.String())
			mockSvc.AssertExpectations(t)
		case <-time.After(time.Hour):
			require.FailNow(t, "signal not received")
		}
	})
}

func TestRunContext_Success(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logger.ForTests()
		top, cancel := context.WithTimeout(t.Context(), time.Millisecond*100)
		defer cancel()

		mockSvc := new(mockService)
		mockSvc.name = "testService"
		mockSvc.On("Start", mock.Anything).
			Run(func(args mock.Arguments) {
				<-args.Get(0).(context.Context).Done()
			}).
			Return(nil).
			Once()
		mockSvc.On("Stop", mock.Anything).Return().Once()

		options := []Option{
			func(cfg *settings) {
				cfg.handle = append(cfg.handle, mockSvc)
				cfg.shutdown = time.Millisecond * 100
			},
		}

		errChan := make(chan error, 1)
		go func() {
			defer close(errChan)
			errChan <- RunContext(top, log, options...)
		}()

		time.Sleep(10 * time.Millisecond)
		cancel()

		require.NoError(t, <-errChan)
		mockSvc.AssertExpectations(t)
	})
}

func TestRunContext_EmptyServices(t *testing.T) {
	log := logger.ForTests()

	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- RunContext(t.Context(), log)
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("RunContext blocked with no services registered")
	}
}

func TestRunContext_ServiceExitCancelsContext(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "clean exit", err: nil},
		{name: "ignored exit", err: context.Canceled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			log := logger.ForTests()
			top, cancel := context.WithCancel(t.Context())
			defer cancel()

			mockSvc := new(mockService)
			mockSvc.name = "testService"
			mockSvc.On("Start", mock.Anything).Return(tc.err).Once()
			mockSvc.On("Stop", mock.Anything).Return().Once()

			options := []Option{
				func(cfg *settings) {
					cfg.handle = append(cfg.handle, mockSvc)
					cfg.shutdown = time.Millisecond * 100
				},
			}

			done := make(chan error, 1)
			go func() {
				defer close(done)
				done <- RunContext(top, log, options...)
			}()

			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("RunContext blocked after service exit")
			}

			mockSvc.AssertExpectations(t)
		})
	}
}

func TestRunContext_Failure(t *testing.T) {
	log := logger.ForTests()

	errStart := bones.Error("start error")

	mockSvc := new(mockService)
	mockSvc.name = "testService"
	mockSvc.On("Start", mock.Anything).Return(errStart).Once()
	mockSvc.On("Stop", mock.Anything).Return().Once()

	options := []Option{
		func(cfg *settings) {
			cfg.handle = append(cfg.handle, mockSvc)
			cfg.shutdown = time.Millisecond * 100
		},
	}

	errChan := make(chan error, 1)
	go func() {
		defer close(errChan)
		errChan <- RunContext(t.Context(), log, options...)
	}()

	assert.ErrorIs(t, <-errChan, errStart)
	mockSvc.AssertExpectations(t)
}

func TestRunContext_ConcurrentFailuresAreAllReported(t *testing.T) {
	log := logger.ForTests()

	errA := bones.Error("service a failed")
	errB := bones.Error("service b failed")

	started := make(chan struct{}, 2)
	release := make(chan struct{})

	newFailingService := func(name string, failWith error) *mockService {
		svc := new(mockService)
		svc.name = name
		svc.On("Start", mock.Anything).
			Run(func(mock.Arguments) {
				started <- struct{}{}
				<-release
			}).
			Return(failWith).
			Once()
		svc.On("Stop", mock.Anything).Return().Once()

		return svc
	}

	svcA := newFailingService("svc-a", errA)
	svcB := newFailingService("svc-b", errB)

	options := []Option{
		func(cfg *settings) {
			cfg.handle = append(cfg.handle, svcA, svcB)
			cfg.shutdown = time.Millisecond * 100
		},
	}

	errChan := make(chan error, 1)
	go func() {
		defer close(errChan)
		errChan <- RunContext(t.Context(), log, options...)
	}()

	<-started
	<-started
	close(release) // both services fail at (as close as possible to) the same time

	err := <-errChan
	require.ErrorIs(t, err, errA, "RunContext must report every service that failed concurrently")
	require.ErrorIs(t, err, errB, "RunContext must report every service that failed concurrently")

	svcA.AssertExpectations(t)
	svcB.AssertExpectations(t)
}

func TestRunContext_SignalContextCancelIsIgnored(t *testing.T) {
	log := logger.ForTests()
	top, cancel := SignalContext(t.Context(), syscall.SIGUSR1)
	defer cancel()

	mockSvc := new(mockService)
	mockSvc.name = "testService"

	started := make(chan struct{}, 1)
	mockSvc.On("Start", mock.Anything).
		Run(func(args mock.Arguments) {
			started <- struct{}{}
			<-args.Get(0).(context.Context).Done()
		}).
		Return(nil).
		Once()
	mockSvc.On("Stop", mock.Anything).Return().Once()

	options := []Option{
		func(cfg *settings) {
			cfg.handle = append(cfg.handle, mockSvc)
			cfg.shutdown = time.Millisecond * 100
		},
	}

	done := make(chan error, 1)
	go func() {
		defer close(done)
		done <- RunContext(top, log, options...)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("RunContext did not start service")
	}

	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("RunContext did not stop after SignalContext cancel")
	}

	mockSvc.AssertExpectations(t)
}

func TestSignalHandling(t *testing.T) {
	ctx, stop := SignalContext(t.Context(), syscall.SIGUSR1)
	defer stop()

	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, process.Signal(syscall.SIGUSR1))

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), ErrOsSignal)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for signal context to cancel")
	}
}
