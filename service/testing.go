package service

import (
	"testing"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/logger"
)

// TestEnv returns an Env for testing a single component: the test context, a
// test logger writing to t, a fresh health monitor and values as the
// dependencies Get can find (usually fakes).
//
//	svc, err := resolver.New(cfg, service.TestEnv(t, fakeStore))
func TestEnv(t testing.TB, values ...any) Env {
	t.Helper()

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	return NewEnv(t.Context(), log, health.New(config.Health{}, log), values...)
}
