package service

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

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
	log := logger.ForTests()

	mockSvc := new(mockService)
	mockSvc.name = "testService"
	mockSvc.On("Start", mock.Anything).Return(nil).Once()
	mockSvc.On("Stop", mock.Anything).Return().Once()

	options := []Option{
		func(cfg *settings) {
			cfg.handle = append(cfg.handle, mockSvc)
			cfg.shutdown = time.Millisecond * 100
			cfg.signal = append(cfg.signal, syscall.SIGUSR1)
		},
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- Run(log, options...)
	}()

	time.Sleep(time.Millisecond * 10)
	process, err := os.FindProcess(os.Getpid())
	require.NoError(t, err)
	require.NoError(t, process.Signal(syscall.SIGUSR1))

	require.NoError(t, <-errChan)
	mockSvc.AssertExpectations(t)
}

func TestRunContext_Success(t *testing.T) {
	log := logger.ForTests()
	top, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockSvc := new(mockService)
	mockSvc.name = "testService"
	mockSvc.On("Start", mock.Anything).Return(nil).Once()
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

	time.Sleep(time.Millisecond * 10)
	cancel()

	require.NoError(t, <-errChan)
	mockSvc.AssertExpectations(t)
}

func TestRunContext_Failure(t *testing.T) {
	log := logger.ForTests()
	top, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockSvc := new(mockService)
	mockSvc.name = "testService"
	mockSvc.On("Start", mock.Anything).Return(errors.New("start error")).Once()
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

	assert.Error(t, <-errChan)
	mockSvc.AssertExpectations(t)
}

func TestShutdownServices(t *testing.T) {
	log := logger.ForTests()
	top, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockSvc := new(mockService)
	mockSvc.name = "testService"
	mockSvc.On("Stop", mock.Anything).Return().Once()

	cfg := settings{
		handle:   []Service{mockSvc},
		shutdown: time.Millisecond * 100,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		shutdownServices(top, log, cfg)
	}()

	time.Sleep(time.Millisecond * 10)
	cancel()
	wg.Wait()

	mockSvc.AssertExpectations(t)
}

func TestSignalHandling(t *testing.T) {
	top, cancel := context.WithCancel(context.Background())
	defer cancel()

	ctx, stop := SignalContext(top, syscall.SIGUSR1)
	defer stop()

	go func() {
		time.Sleep(time.Millisecond * 50)
		process, err := os.FindProcess(os.Getpid())
		assert.NoError(t, err)
		assert.NoError(t, process.Signal(syscall.SIGUSR1))
	}()

	select {
	case <-ctx.Done():
		assert.ErrorIs(t, context.Cause(ctx), ErrOsSignal)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for signal context to cancel")
	}
}
