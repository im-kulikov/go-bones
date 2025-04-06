package logger

import (
	"testing"

	"github.com/stretchr/testify/mock"
)

type stubTB struct {
	testing.TB
	mock.Mock
}

func (t *stubTB) Log(args ...any) { t.Called(args...) }

func Test_ForTest(t *testing.T) {
	tb := &stubTB{TB: t}
	tb.Test(t)

	tb.On("Log",
		`level=INFO msg="hello world" app.name=test-app-name app.version=test-app-version`).Once()

	log := ForTests(TestLoggerWriteToTB(tb))
	log.Info("hello world")
}
