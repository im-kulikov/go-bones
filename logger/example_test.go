package logger_test

import (
	"io"
	"log/slog"
	"os"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
)

// A service adds its own format in main, before app.Init or logger.Init; then
// LOGGER_FORMAT picks it like a built-in one.
func ExampleRegisterFormat() {
	logger.RegisterFormat("short", func(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewTextHandler(w, &slog.HandlerOptions{
			Level: opts.Level, // keeps LOGGER_LEVEL working
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if len(groups) == 0 && a.Key == slog.TimeKey {
					return slog.Attr{} // the collector stamps the time
				}

				return a
			},
		})
	})

	log := logger.Init(config.Logger{Format: "short", Level: "info", Secrets: []string{"password"}},
		logger.WithOutput(os.Stdout))

	log.Debug("not printed")
	log.Info("login", "user", "ivanov", "password", "hunter2")
	// Output: level=INFO msg=login user=ivanov password=REDACTED
}
