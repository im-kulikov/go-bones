package internal

import (
	"context"
	"sync"
	"time"
)

// GracefulShutdown performs a shutdown operation with a context-derived timeout,
// ensuring cleanup within the specified duration.
func GracefulShutdown(top context.Context, timeout time.Duration, shutdown func(context.Context)) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(top), FallbackTimeout(timeout))
	defer cancel()

	shutdown(ctx)
}

// LazyGracefulShutdown starts a goroutine to delay graceful shutdown until the
// parent context is canceled.
// Returns a function to wait for shutdown completion.
// Parameters:
//   - `top`: Parent context controlling the operation's lifecycle.
//   - `timeout`: Duration for a graceful shutdown timeout.
//   - `shutdown`: Function that performs the actual shutdown logic.
func LazyGracefulShutdown(
	top context.Context,
	timeout time.Duration,
	shutdown func(context.Context),
) func() {
	var wg sync.WaitGroup
	wg.Go(func() {
		<-top.Done()

		GracefulShutdown(top, timeout, shutdown)
	})

	return wg.Wait
}
