package logger

import "github.com/im-kulikov/go-bones/config"

// prepareTransformers initializes a set of record transformers for the logger.
// It appends transformers for secrets masking and context handling, as well as
// optional tracing transformations, based on the logger configuration.
//
// Parameters:
//   - cfg: Logger configuration containing options for secrets and tracing.
//   - transformers: Additional custom slogTransformer implementations.
//
// Returns:
//   - A slice of transformers that will process log records before handling.
func prepareTransformers(cfg config.Logger, transformers ...slogTransformer) []slogTransformer {
	var out []slogTransformer

	// Add a transformer for secret masking if secrets are configured.
	if len(cfg.Secrets) > 0 {
		secrets := new(secretTransformer)
		secrets.apply(cfg.Secrets)

		out = append(out, secrets)
	}

	// Add a transformer to include context metadata in log records.
	out = append(out, slogTransformerFunc(contextTransformer))

	// Add an OpenTracing transformer if tracing is enabled.
	if cfg.OpenTracingEnabled {
		out = append(out, slogTransformerFunc(openTracingTransform))
	}

	// Add any additional custom transformers provided by the caller.
	out = append(out, transformers...)

	return out
}

// applyHandler decorates the provided `handler` with application metadata.
// It adds attributes like the application name and version from the logger configuration.
//
// Parameters:
//   - cfg: Logger configuration containing application metadata.
//   - handler: The underlying slog.Handler to be modified.
//
// Returns:
//   - A new handler with embedded application metadata attributes.
func applyHandler(cfg config.Logger, handler Handler) Handler {
	values := make([]any, 0, 2)

	// Add the application name to the handler attributes, if provided.
	if cfg.AppName() != "" {
		values = append(values, String("name", cfg.AppName()))
	}

	// Add an application version to the handler attributes, if provided.
	if cfg.AppVersion() != "" {
		values = append(values, String("version", cfg.AppVersion()))
	}

	// If there are attributes to add, create a new handler with them; otherwise, return the original handler.
	if len(values) > 0 {
		return handler.WithAttrs([]Attr{Group("app", values...)})
	}

	return handler
}

// New creates a new logger instance with the configured handler and transformers.
// It wraps the provided handler with application metadata and prepares the transformers to process records.
//
// Parameters:
//   - cfg: Logger configuration containing attributes and options for logging behavior.
//   - handler: A slog.Handler instance for processing log records.
//   - transformers: Optional custom record transformers to apply.
//
// Returns:
//   - A pointer to the configured Logger instance.
func New(cfg config.Logger, handler Handler, transformers ...slogTransformer) *Logger {
	return newLogger(&wrappedHandler{
		conf: cfg,
		next: applyHandler(cfg, handler),
		list: prepareTransformers(cfg, transformers...),
	})
}
