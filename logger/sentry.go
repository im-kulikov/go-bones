package logger

import (
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Sentry allows to configure sentry client.
type Sentry struct {
	DSN string `env:"DSN" usage:"allows to set custom sentry DSN" example:"https://public@sentry.example.com/1"`
	Env string `env:"ENVIRONMENT" usage:"allows to set custom sentry environment" example:"production"`
}

// passSentryToLogger allows capture zap.Logger messages with >= error level with sentry client.
func passSentryToLogger(logger *zap.Logger, cfg Sentry, release string) (*zap.Logger, error) {
	if err := sentry.Init(sentry.ClientOptions{Dsn: cfg.DSN, Release: release, Environment: cfg.Env}); err != nil {
		return nil, fmt.Errorf("sentry config: %w", err)
	}

	logger = logger.WithOptions(zap.Hooks(func(entry zapcore.Entry) error {
		if entry.Level >= zapcore.ErrorLevel {
			defer sentry.Flush(2 * time.Second)
			sentry.CaptureMessage(fmt.Sprintf("%s, Line No: %d :: %s\n\nstack:\n%s",
				entry.Caller.File,
				entry.Caller.Line,
				entry.Message,
				entry.Stack))
		}

		return nil
	}))

	return logger, nil
}
