package service

import (
	"errors"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

// Option provides a type interface with a method to create options for managing goroutines or services.
// An option can be created by calling WithService or WithShutdownTimeout,
// which add options to manage their respective resources.
// Options are used to conditionally run, stop, or modify the lifecycle of goroutines and services.
type Option func(*settings)

// WithShutdownTimeout adds an option to manage the shutdown of services.
// When called with a time.Duration, it shuts down any service specified.
// If no argument is provided, returns an Option that can be used with WithService.
// This function provides a way to conditionally manage service shutdowns cleanly via options.
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
