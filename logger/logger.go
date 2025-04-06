package logger

import "github.com/im-kulikov/go-bones/config"

func prepareTransformers(cfg config.Logger, transformers ...slogTransformer) []slogTransformer {
	var out []slogTransformer //nolint:prealloc

	if len(cfg.Secrets) > 0 {
		secrets := new(secretTransformer)
		secrets.apply(cfg.Secrets)

		out = append(out, secrets)
	}

	out = append(out, slogTransformerFunc(contextTransformer))

	if cfg.OpenTracingEnabled { // add
		out = append(out, slogTransformerFunc(openTracingTransform))
	}

	out = append(out, transformers...)

	return out
}

func applyHandler(cfg config.Logger, handler Handler) Handler {
	values := make([]any, 0, 2)
	if cfg.AppName() != "" {
		values = append(values, String("name", cfg.AppName()))
	}

	if cfg.AppVersion() != "" {
		values = append(values, String("version", cfg.AppVersion()))
	}

	if len(values) > 0 {
		return handler.WithAttrs([]Attr{Group("app", values...)})
	}

	return handler
}

// New configures wrappedHandler and creates new slog.Logger.
func New(cfg config.Logger, handler Handler, transformers ...slogTransformer) *Logger {
	return newLogger(&wrappedHandler{
		conf: cfg,
		next: applyHandler(cfg, handler),
		list: prepareTransformers(cfg, transformers...),
	})
}
