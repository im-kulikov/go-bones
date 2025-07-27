package logger

import (
	"io"
	"log/slog"
	"os"
)

type options struct {
	levels *Level
	output io.Writer

	handler      Handler
	transformers []slogTransformer
}

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

func (o *options) setDefaults() {
	if o.output == nil {
		o.output = os.Stdout
	}

	if o.levels == nil {
		tmp := slog.LevelInfo
		o.levels = &tmp
	}

	if o.handler == nil {
		o.handler = slog.NewTextHandler(o.output, &slog.HandlerOptions{
			AddSource: false,
			Level:     o.levels,
		})
	}
}
