package logger_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"slices"
	"sync/atomic"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
)

// late is a handler whose target is bound after the logger is built: a sink
// that is a component itself, such as a Kafka producer added with app.Add.
// Records logged before bind are dropped.
//
// It records WithAttrs and WithGroup and replays them on the target once it is
// bound: go-bones adds attributes while it builds the logger (the app group),
// and log.With may run before the component exists.
type late struct {
	target *atomic.Pointer[slog.Handler]
	ops    []func(slog.Handler) slog.Handler
	cache  *atomic.Pointer[derived]
}

// derived is the target with the ops of one late applied, built once per bind.
type derived struct {
	from *slog.Handler
	h    slog.Handler
}

func newLate() (handler *late, bind func(slog.Handler)) {
	target := new(atomic.Pointer[slog.Handler])

	return &late{target: target, cache: new(atomic.Pointer[derived])},
		func(h slog.Handler) { target.Store(&h) }
}

func (l *late) resolve() slog.Handler {
	base := l.target.Load()
	if base == nil {
		return nil
	}

	if d := l.cache.Load(); d != nil && d.from == base {
		return d.h
	}

	h := *base
	for _, op := range l.ops {
		h = op(h)
	}

	l.cache.Store(&derived{from: base, h: h})

	return h
}

func (l *late) Enabled(ctx context.Context, level slog.Level) bool {
	h := l.resolve()

	return h != nil && h.Enabled(ctx, level)
}

func (l *late) Handle(ctx context.Context, r slog.Record) error {
	if h := l.resolve(); h != nil {
		return h.Handle(ctx, r)
	}

	return nil
}

func (l *late) WithAttrs(attrs []slog.Attr) slog.Handler {
	return l.with(func(h slog.Handler) slog.Handler { return h.WithAttrs(attrs) })
}

func (l *late) WithGroup(name string) slog.Handler {
	if name == "" {
		return l
	}

	return l.with(func(h slog.Handler) slog.Handler { return h.WithGroup(name) })
}

func (l *late) with(op func(slog.Handler) slog.Handler) slog.Handler {
	return &late{
		target: l.target,
		ops:    append(slices.Clip(l.ops), op),
		cache:  new(atomic.Pointer[derived]),
	}
}

// noTime drops the time, to keep the example output stable.
func noTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey {
		return slog.Attr{}
	}

	return a
}

// A format that writes every record to stdout and to a second sink with
// slog.NewMultiHandler. The sink is a component built after app.Init, so it is
// bound late, and attributes added before that still reach it. Settings of
// such a format come from the environment: its constructor does not see the
// service config.
func ExampleRegisterFormat_multi() {
	sink, bind := newLate()

	logger.RegisterFormat("text+sink", func(w io.Writer, opts *slog.HandlerOptions) slog.Handler {
		return slog.NewMultiHandler(
			slog.NewTextHandler(w, &slog.HandlerOptions{Level: opts.Level, ReplaceAttr: noTime}),
			sink,
		)
	})

	cfg := config.Logger{Format: "text+sink", Level: "info", AddAppInfo: true}
	cfg.SetAppNameAndVersion("demo", "v1")

	log := logger.Init(cfg, logger.WithOutput(os.Stdout)) // app.Init in a service
	peer := log.With("peer", "10.0.0.1")
	peer.Info("before the sink")

	// In the Start of the component that owns the sink:
	bind(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{ReplaceAttr: noTime}))
	peer.Info("after the sink")

	// Output:
	// level=INFO msg="before the sink" app.name=demo app.version=v1 peer=10.0.0.1
	// level=INFO msg="after the sink" app.name=demo app.version=v1 peer=10.0.0.1
	// {"level":"INFO","msg":"after the sink","app":{"name":"demo","version":"v1"},"peer":"10.0.0.1"}
}
