package service

import (
	"errors"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

// Option represents a functional option for configuring the service settings.
type Option func(*settings)

// WithShutdownTimeout sets the timeout for graceful shutdown.
// If the provided value is zero, it is ignored.
func WithShutdownTimeout(v time.Duration) Option {
	return func(g *settings) {
		if v == 0 {
			return
		}

		g.shutdown = v
	}
}

// WithLoggerPingPong enables a periodic ping-pong log message.
// If the interval is zero or negative, it is ignored.
func WithLoggerPingPong(v time.Duration) Option {
	return func(g *settings) {
		if v <= 0 {
			return
		}

		g.logger.Info("ping pong service added")
		g.append(newPingPong(g.logger, v))
	}
}

// WithIgnoreError adds an error to the list of ignored errors.
// If the provided error is nil, it is ignored.
func WithIgnoreError(v error) Option {
	return func(g *settings) {
		if v == nil {
			return
		}

		g.ignore = errors.Join(g.ignore, v)
	}
}

// append adds a service to the list of managed services,
// ensuring that disabled services (implementing Enabler) are skipped.
func (g *settings) append(v Service) {
	if v == nil {
		return
	}

	if svc, ok := v.(Enabler); ok && !svc.Enabled() {
		g.logger.Warn("service disabled", logger.String("service", v.Name()))

		return
	}

	g.handle = append(g.handle, v)
}

// WithService adds one or more services to the runner.
// If a Group is provided, all its services are added.
func WithService(v ...Service) Option {
	return func(g *settings) {
		if v == nil {
			return
		}

		for _, service := range v {
			if svc, ok := service.(group); ok {
				g.logger.Info("adding group services", logger.String("group", svc.Name()))
				for _, item := range svc {
					g.append(item)
				}

				return
			}

			g.append(service)
		}
	}
}
