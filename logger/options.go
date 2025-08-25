package logger

import (
	"io"
	"log/slog"
	"os"
)

// options contain configuration settings for creating and customizing a logger.
// It defines the log source flag, log level, output writer, output format,
// handler, and optional log transformers.
//
// It is typically configured via Option functions such as WithSource,
// WithLevel, or via optionsFromConfig, which builds it from a config.Logger.
//
// Fields:
//   - source: whether to include source file and line in logs.
//   - levels: the log level threshold.
//   - output: destination for log output, defaults to os.Stdout.
//   - format: a HandlerConstructor function to build the slog.Handler.
//   - handler: the actual slog.Handler instance used for logging.
//   - transformers: functions applied to log records before they are handled.
type options struct {
	source bool
	levels *Level
	output io.Writer
	format HandlerConstructor

	handler      Handler
	transformers []slogTransformer
}

// HandlerConstructor is a factory function that creates a new Handler
// using the given io.Writer and HandlerOptions.
type HandlerConstructor = func(io.Writer, *HandlerOptions) Handler

// Option defines a function type used to modify the configuration of options.
// It allows customization by applying specific settings to the "options" struct.
type Option func(*options)

// WithLevel returns an Option that sets the logging level in the "options" configuration.
func WithLevel(l Level) Option {
	return func(o *options) { o.levels = &l }
}

// WithOutput sets the output destination for log records. It writes logs to the specified io.Writer.
func WithOutput(w io.Writer) Option {
	return func(o *options) { o.output = w }
}

// WithHandler sets the log Handler for processing log records and returns an Option to configure logging options.
func WithHandler(handler Handler) Option {
	return func(o *options) { o.handler = handler }
}

// WithTransformers adds one or more slogTransformer instances to modify log records before they are processed.
func WithTransformers(transformers ...slogTransformer) Option {
	return func(o *options) { o.transformers = append(o.transformers, transformers...) }
}

// WithSource returns an Option that enables or disables the inclusion
// of source file and line information in log entries.
func WithSource(source bool) Option {
	return func(o *options) { o.source = source }
}

// setDefaults populates missing option values with their defaults:
//   - output is set to os.Stdout if not specified.
//   - levels defaults to slog.LevelInfo.
//   - handler is built using format if a format is provided.
func (o *options) setDefaults() {
	if o.output == nil {
		o.output = os.Stdout
	}

	if o.levels == nil {
		tmp := slog.LevelInfo
		o.levels = &tmp
	}

	if o.handler == nil && o.format != nil {
		o.handler = o.format(o.output, &slog.HandlerOptions{
			AddSource: o.source,
			Level:     o.levels,
		})
	}
}
