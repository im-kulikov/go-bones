package service

import (
	"errors"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

// Option configures the service runner built by Run and RunContext.
type Option func(*settings)

// WithShutdownTimeout overrides the timeout used while stopping registered services.
func WithShutdownTimeout(v time.Duration) Option {
	return func(g *settings) {
		if v == 0 {
			return
		}

		g.shutdown = v
	}
}

// WithLoggerPingPong registers an internal heartbeat service that logs on the given interval.
func WithLoggerPingPong(v time.Duration) Option {
	return func(g *settings) {
		if v <= 0 {
			return
		}

		g.logger.Info("ping pong service added")
		g.append(newPingPong(g.logger, v))
	}
}

// WithIgnoreError adds an error to the ignore list checked by Run and RunContext.
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

// WithService registers one or more services with the runner.
// Composed services are unwrapped so their members are started individually.
func WithService(v ...Service) Option {
	return func(g *settings) {
		if v == nil {
			return
		}

		for _, service := range v {
			if svc, ok := service.(composed); ok {
				for _, item := range svc {
					g.append(item)
				}

				continue
			}

			g.append(service)
		}
	}
}
