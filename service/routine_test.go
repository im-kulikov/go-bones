package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
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

type stopCtxProbe struct {
	errCh chan error
}

func (s *stopCtxProbe) Name() string { return "test-service" }

func (s *stopCtxProbe) Start(ctx context.Context) error {
	<-ctx.Done()

	return nil
}

func (s *stopCtxProbe) Stop(ctx context.Context) {
	s.errCh <- ctx.Err()
}

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

func Test_groupErrors(t *testing.T) {
	cases := []struct {
		name string
		pass error
		errs error
	}{
		{name: "should ignore single", pass: context.Canceled, errs: errors.Join(context.Canceled)},
		{name: "should ignore multiple", pass: io.EOF, errs: errors.Join(
			context.Canceled, context.DeadlineExceeded, io.EOF)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, errors.Is(tc.errs, tc.pass))
		})
	}
}

func TestRun_Success(t *testing.T) {
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
			cfg.shutdown = time.Millisecond * 100
			cfg.signal = append(cfg.signal, syscall.SIGUSR1)
		},
		WithService(NewLauncher("test", func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		})),
	}

	errChan := make(chan error, 1)
	go func() { errChan <- Run(log, options...) }()

	time.Sleep(time.Millisecond * 100)
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGUSR1))

	select {
	case err := <-errChan:
		require.NoErrorf(t, err, "%v", err)
		require.Contains(t, buf.String(), syscall.SIGUSR1.String())
		mockSvc.AssertExpectations(t)
	case <-time.After(time.Hour):
		t.Fatal("signal not received")
	}
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
		errChan <- RunContext(t.Context(), log, options...)
	}()

	assert.ErrorIs(t, <-errChan, errStart)
	mockSvc.AssertExpectations(t)
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

func TestShutdownServices(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logger.ForTests()
		top, cancel := context.WithTimeout(t.Context(), time.Millisecond*100)
		defer cancel()

		mockSvc := new(mockService)
		mockSvc.name = "testService"
		mockSvc.On("Stop", mock.Anything).Return().Once()

		cfg := settings{
			handle:   []Service{mockSvc},
			shutdown: time.Millisecond * 100,
		}

		var wg sync.WaitGroup
		wg.Go(func() { shutdownServices(top, log, cfg) })

		time.Sleep(10 * time.Millisecond)
		cancel()
		wg.Wait()

		mockSvc.AssertExpectations(t)
	})
}

func TestShutdownServices_DefaultTimeoutIsNotAlreadyExpired(t *testing.T) {
	log := logger.ForTests()
	top, cancel := context.WithCancel(t.Context())

	svc := &stopCtxProbe{errCh: make(chan error, 1)}

	cfg := settings{
		handle: []Service{svc},
		// zero shutdown timeout should still allow graceful stop semantics
	}

	cancel()

	shutdownServices(top, log, cfg)

	select {
	case err := <-svc.errCh:
		assert.NoError(t, err, "shutdown context should not be expired before Stop runs")
	case <-time.After(time.Second):
		t.Fatal("service Stop was not called")
	}
}

func TestSignalHandling(t *testing.T) {
	ctx, stop := SignalContext(t.Context(), syscall.SIGUSR1)
	defer stop()

	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Go(func() {
		time.Sleep(time.Millisecond * 50)
		process, err := os.FindProcess(os.Getpid())
		assert.NoError(t, err)
		assert.NoError(t, process.Signal(syscall.SIGUSR1))
	})

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), ErrOsSignal)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for signal context to cancel")
	}
}
