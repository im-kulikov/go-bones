package logger

import (
	"fmt"
	"net/url"

	"go.uber.org/zap"
)

// Option allows to set custom logger settings.
type Option func(*logger)

// WithAppName allows to set application name to logger fields.
func WithAppName(v string) Option {
	return func(l *logger) { l.appName = v }
}

// WithAppVersion allows to set application version to logger fields.
func WithAppVersion(v string) Option {
	return func(l *logger) { l.appVersion = v }
}

// WithConsoleColored allows to set colored console output.
func WithConsoleColored() Option {
	return func(l *logger) { l.colored = true }
}

// WithSentry allows to integrate sentry.
func WithSentry(v Sentry) Option {
	return func(l *logger) { l.sentry = &v }
}

// WithTimeKey allows to set TimeKey for zapcore.EncoderConfig.
func WithTimeKey(v string) Option {
	return func(l *logger) { l.config.EncoderConfig.TimeKey = v }
}

// WithZapOption allows to set zap.Option.
func WithZapOption(v zap.Option) Option {
	return func(l *logger) { l.options = append(l.options, v) }
}

// WithCustomLevel allows to pass custom level, that can be changed at any time.
// For example take a look https://github.com/uber-go/zap/blob/master/http_handler.go#L33.
func WithCustomLevel(v zap.AtomicLevel) Option {
	return func(l *logger) { l.config.Level = v }
}

// WithCustomOutput allows to set custom output for Logger.
func WithCustomOutput(name string, v Sink) Option {
	return func(l *logger) {
		scheme := name + "-writer"

		if err := zap.RegisterSink(scheme, func(_ *url.URL) (Sink, error) { return v, nil }); err != nil {
			Default().Panicf("could not register custom logger.Sink: %s", err)
		}

		scheme = fmt.Sprintf("%s:whatever", scheme)

		l.config.OutputPaths = []string{scheme}
	}
}
