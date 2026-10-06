package service

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/im-kulikov/go-bones/logger"
)

// TickerOption configures a service created by NewTicker.
type TickerOption func(*ticker)

type ticker struct {
	every   time.Duration
	timeout time.Duration
	jitter  float64
	log     *logger.Logger
}

// WithTickerTimeout bounds each run of the task with a deadline. Zero, the
// default, means no deadline beyond the service context.
func WithTickerTimeout(d time.Duration) TickerOption {
	return func(t *ticker) { t.timeout = max(d, 0) }
}

// WithTickerJitter spreads runs by ±fraction of the interval (0.1 is ±10%), so
// replicas started together do not hit a dependency at the same moment. The
// fraction is clamped to [0, 1]; NaN means no jitter.
func WithTickerJitter(fraction float64) TickerOption {
	return func(t *ticker) {
		t.jitter = 0
		if fraction > 0 { // false for NaN, which max would pass through
			t.jitter = min(fraction, 1)
		}
	}
}

// WithTickerLogger sets the logger for failed runs; logger.Default() otherwise.
// Nil is ignored.
func WithTickerLogger(log *logger.Logger) TickerOption {
	return func(t *ticker) {
		if log != nil {
			t.log = log
		}
	}
}

// NewTicker returns a service that runs task right after start and then every
// interval, until the service is stopped. A run that returns an error is logged
// and the ticker keeps going: to stop the application on a failure, use
// NewLauncher. A panic in task stops the service like in any launcher.
//
// Runs never overlap: the next interval starts when a run returns. For a
// liveness signal, call a health.Heartbeat's Beat from task.
//
// It panics if every is not positive, like time.NewTicker.
func NewTicker(
	name string,
	every time.Duration,
	task func(context.Context) error,
	options ...TickerOption,
) Service {
	if every <= 0 {
		panic("service: non-positive interval for NewTicker")
	}

	t := &ticker{every: every, log: logger.Default()}
	for _, option := range options {
		if option != nil {
			option(t)
		}
	}

	log := logger.Named(t.log, name)

	return NewLauncher(name, func(ctx context.Context) error {
		timer := time.NewTimer(0)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-timer.C:
				if ctx.Err() != nil { // select picks at random when both are ready
					return nil
				}

				t.run(ctx, log, task)
				timer.Reset(t.next())
			}
		}
	}, WithLauncherLogger(t.log))
}

func (t *ticker) run(top context.Context, log *logger.Logger, task func(context.Context) error) {
	ctx, cancel := top, context.CancelFunc(func() {})
	if t.timeout > 0 {
		ctx, cancel = context.WithTimeout(top, t.timeout)
	}
	defer cancel()

	if err := task(ctx); err != nil && top.Err() == nil {
		log.ErrorContext(top, "periodic task failed", logger.Err(err))
	}
}

func (t *ticker) next() time.Duration {
	if t.jitter == 0 {
		return t.every
	}

	//nolint:gosec // jitter does not need a cryptographic source.
	spread := (rand.Float64()*2 - 1) * t.jitter * float64(t.every)

	return t.every + time.Duration(spread)
}
