package service

import (
	"context"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

const pingPongServiceName = "ping-pong"

// newPingPong returns an instance of Service that wraps the launcher function named "ping-pong" with the logger.
func newPingPong(l *logger.Logger, timeout time.Duration) Service {
	return NewLauncher(pingPongServiceName, func(ctx context.Context) error {
		log := logger.Named(l, pingPongServiceName)

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		log.Info("would be run with next settings", logger.Duration("interval", timeout))

		for {
			select {
			case <-ctx.Done():
				return context.Cause(ctx)
			case <-timer.C:
				log.Info(pingPongServiceName)
				timer.Reset(timeout)
			}
		}
	})
}
