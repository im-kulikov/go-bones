package service

import (
	"time"
)

// Option allows customizing service module.
type Option func(*runner)

// WithShutdownTimeout allows set shutdown timeout.
func WithShutdownTimeout(v time.Duration) Option {
	return func(g *runner) {
		if v == 0 {
			return
		}

		g.shutdown = v
	}
}

// WithLoggerPingPong allows to set ping-pong timer for logger.
func WithLoggerPingPong(v time.Duration) Option {
	return func(g *runner) {
		if v <= 0 {
			return
		}

		g.pingPongTimeout = v
		g.pingPongDisable = false
	}
}

// WithIgnoreError allows set ignored errors.
func WithIgnoreError(v error) Option {
	return func(g *runner) {
		if v == nil {
			return
		}

		g.ignore = append(g.ignore, v)
	}
}

func (g *runner) append(v Service) {
	if svc, ok := v.(Enabler); ok && !svc.Enabled() {
		g.logger.Warnw("service disabled", "service", v.Name())

		return
	}

	g.services = append(g.services, v)
}

// WithService allows set Service into Runner.
func WithService(v Service) Option {
	return func(g *runner) {
		if v == nil {
			return
		}

		if group, ok := v.(*Group); ok {
			g.logger.Info("try to add group services")

			for _, svc := range group.Services() {
				g.append(svc)
			}

			return
		}

		g.append(v)
	}
}
