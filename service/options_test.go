package service

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

// MockService is a mock implementation of the Service interface.
type mockService struct {
	mock.Mock

	name string

	enabled bool
}

func isMethodCalled(m *mock.Mock, methodName string) bool {
	return len(m.ExpectedCalls) > 0 &&
		slices.ContainsFunc(m.ExpectedCalls, func(call *mock.Call) bool {
			return call.Method == methodName
		})
}

func (m *mockService) Name() string {
	if isMethodCalled(&m.Mock, "Name") {
		return m.Called().String(0)
	}

	if m.name != "" {
		return m.name
	}

	return "mock"
}

func (m *mockService) Start(ctx context.Context) error {
	if isMethodCalled(&m.Mock, "Start") {
		return m.Called(ctx).Error(0)
	}

	return nil
}

func (m *mockService) Stop(ctx context.Context) {
	if isMethodCalled(&m.Mock, "Stop") {
		m.Called(ctx)
	}
}

func (m *mockService) Enabled() bool {
	if isMethodCalled(&m.Mock, "Enabled") {
		return m.Called().Bool(0)
	}

	return m.enabled
}

func TestWithShutdownTimeout(t *testing.T) {
	cfg := &settings{}
	opt := WithShutdownTimeout(5 * time.Second)
	opt(cfg)
	require.Equal(t, 5*time.Second, cfg.shutdown)
}

func TestWithShutdownTimeout_ZeroValue(t *testing.T) {
	cfg := &settings{shutdown: 10 * time.Second}
	opt := WithShutdownTimeout(0)
	opt(cfg)
	require.Equal(t, 10*time.Second, cfg.shutdown)
}

func TestWithLoggerPingPong(t *testing.T) {
	log := logger.ForTests()
	cfg := &settings{logger: log}
	opt := WithLoggerPingPong(time.Second)
	opt(cfg)

	require.Len(t, cfg.handle, 1)
}

func TestWithLoggerPingPong_NegativeValue(t *testing.T) {
	log := logger.ForTests()
	cfg := &settings{logger: log}
	opt := WithLoggerPingPong(-1 * time.Second)
	opt(cfg)

	require.Empty(t, cfg.handle)
}

func TestWithIgnoreError(t *testing.T) {
	cfg := &settings{}
	opt := WithIgnoreError(errors.New("test error"))
	opt(cfg)
	assert.Error(t, cfg.ignore)
}

func TestWithIgnoreError_NilValue(t *testing.T) {
	cfg := &settings{}
	opt := WithIgnoreError(nil)
	opt(cfg)
	assert.Nil(t, cfg.ignore)
}

func TestWithService(t *testing.T) {
	log := logger.ForTests()
	cfg := &settings{logger: log}

	service := &mockService{}
	service.On("Enabled").Return(true)

	opt := WithService(service, nil)
	opt(cfg)

	require.Len(t, cfg.handle, 1)
	assert.Equal(t, service, cfg.handle[0])
}

func TestWithService_nil(t *testing.T) {
	log := logger.ForTests()
	cfg := &settings{logger: log}

	service := &mockService{}
	service.On("Enabled").Return(true)

	opt := WithService()
	opt(cfg)

	require.Empty(t, cfg.handle)
}

func TestWithService_DisabledService(t *testing.T) {
	log := logger.ForTests()
	cfg := &settings{logger: log}

	service := &mockService{enabled: false}
	opt := WithService(service)
	opt(cfg)

	require.Empty(t, cfg.handle)
}

func TestWithService_Group(t *testing.T) {
	log := logger.ForTests()
	cfg := &settings{logger: log}

	service1 := &mockService{enabled: true}
	service2 := &mockService{enabled: true}
	service := NewGroup(service1, service2)

	opt := WithService(service)
	opt(cfg)

	require.Len(t, cfg.handle, 2)
	assert.Equal(t, service1, cfg.handle[0])
	assert.Equal(t, service2, cfg.handle[1])
}
