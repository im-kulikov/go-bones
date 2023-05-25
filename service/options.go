package service

import (
	"time"
)

// Option allows customizing service module.
type Option func(*group)

// WithShutdownTimeout allows set shutdown timeout.
func WithShutdownTimeout(v time.Duration) Option {
	return func(g *group) {
		if v == 0 {
			return
		}

		g.shutdown = v
	}
}

// WithLoggerPingPong allows to set ping-pong timer for logger.
func WithLoggerPingPong(v time.Duration) Option {
	return func(g *group) {
		if v <= 0 {
			return
		}

		g.pingPongTimeout = v
		g.pingPongDisable = false
	}
}

// WithIgnoreError allows set ignored errors.
func WithIgnoreError(v error) Option {
	return func(g *group) {
		if v == nil {
			return
		}

		g.ignore = append(g.ignore, v)
	}
}

// WithService allows set Service into Group.
func WithService(v Service) Option {
	return func(g *group) {
		if v == nil {
			return
		}

		if svc, ok := v.(Enabler); ok && !svc.Enabled() {
			g.logger.Warnw("service disabled", "service", v.Name())

			return
		}

		g.services = append(g.services, v)
	}
}
