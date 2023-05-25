package service

import (
	"context"
	"sync"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

type pingPong struct {
	log     logger.Logger
	timeout time.Duration

	once sync.Once
	done chan struct{}
}

func newPingPong(log logger.Logger, timeout time.Duration) Service {
	log.Infow("ping-pong service", "timer", timeout)

	return &pingPong{log: log, timeout: timeout, done: make(chan struct{})}
}

// Name of the service.
func (p *pingPong) Name() string { return "ping-pong" }

// Start ping-pong service.
func (p *pingPong) Start(ctx context.Context) error {
	timer := time.NewTimer(p.timeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-p.done:
			return nil
		case <-timer.C:
			p.log.Info("ping-pong")
			timer.Reset(p.timeout)
		}
	}
}

// Stop ping-pong service.
func (p *pingPong) Stop(_ context.Context) {
	p.once.Do(func() {
		close(p.done)
	})
}
