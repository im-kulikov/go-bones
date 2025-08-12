package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
)

type stubTB struct {
	testing.TB
	mock.Mock
}

func (t *stubTB) Log(args ...any) { t.Called(args...) }

func (t *stubTB) Context() context.Context {
	if t.TB == nil {
		return nil
	}

	ctx, cancel := context.WithCancel(t.TB.Context())
	defer cancel()

	return ctx
}

func Test_ForTest(t *testing.T) {
	tb := &stubTB{TB: t}
	tb.Test(t)

	tb.On("Log",
		`level=INFO msg="hello world" app.name=test-app-name app.version=test-app-version`).Once()

	log := ForTests(TestLoggerWriteToTB(tb))
	log.Info("hello world")
}
