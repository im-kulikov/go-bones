package internal

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestGracefulShutdown(t *testing.T) { //nolint:cyclop
	tests := []struct {
		name                    string
		timeout                 time.Duration
		topCtxTimeout           time.Duration
		cancelTopBeforeShutdown bool
		check                   func(t *testing.T, ctx context.Context)
	}{
		{
			name:    "successful shutdown within timeout",
			timeout: 100 * time.Millisecond,
			check: func(t *testing.T, ctx context.Context) {
				select {
				case <-ctx.Done():
				case <-time.After(50 * time.Millisecond):
				}
			},
			topCtxTimeout: 1 * time.Second,
		},
		{
			name:    "context times out before shutdown completes",
			timeout: 50 * time.Millisecond,
			check: func(t *testing.T, ctx context.Context) {
				select {
				case <-ctx.Done():
				case <-time.After(100 * time.Millisecond):
				}
			},
			topCtxTimeout: 1 * time.Second,
		},
		{
			name:    "top-level context cancels shutdown",
			timeout: 500 * time.Millisecond,
			check: func(t *testing.T, ctx context.Context) {
				select {
				case <-ctx.Done():
				case <-time.After(200 * time.Millisecond):
				}
			},
			topCtxTimeout: 100 * time.Millisecond,
		},
		{
			name:    "immediate shutdown",
			timeout: 100 * time.Millisecond,
			check: func(t *testing.T, ctx context.Context) {
				select {
				case <-ctx.Done():
				default: // No action needed; shutdown finishes immediately.
				}
			},
			topCtxTimeout: 1 * time.Second,
		},
		{
			name:    "default timeout ignores canceled parent",
			timeout: 0,
			check: func(t *testing.T, ctx context.Context) {
				if err := ctx.Err(); err != nil {
					t.Fatalf("shutdown context should not be canceled at start: %v", err)
				}

				select {
				case <-ctx.Done():
					t.Fatal("shutdown context was canceled before shutdown returned")
				default:
				}
			},
			cancelTopBeforeShutdown: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var (
					top    context.Context
					cancel context.CancelFunc
				)

				if tt.topCtxTimeout > 0 {
					top, cancel = context.WithTimeout(context.Background(), tt.topCtxTimeout)
				} else {
					top, cancel = context.WithCancel(context.Background())
				}
				defer cancel()

				if tt.cancelTopBeforeShutdown {
					cancel()
				}

				completed := make(chan struct{})
				shutdownFn := func(ctx context.Context) {
					if tt.check != nil {
						tt.check(t, ctx)
					}
					close(completed)
				}

				GracefulShutdown(top, tt.timeout, shutdownFn)

				select {
				case <-completed:
				case <-time.After(200 * time.Millisecond):
					t.Fatalf("shutdown did not complete as expected for test case: %s", tt.name)
				}
			})
		})
	}
}

func TestLazyGracefulShutdown(t *testing.T) { //nolint:cyclop
	tests := []struct {
		name      string
		preCancel bool
		timeout   time.Duration
	}{
		{
			name:    "runs after parent cancellation",
			timeout: 100 * time.Millisecond,
		},
		{
			name:      "runs when parent is already canceled",
			timeout:   0,
			preCancel: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				top, cancel := context.WithCancel(context.Background())
				defer cancel()

				if tt.preCancel {
					cancel()
				}

				completed := make(chan struct{})
				LazyGracefulShutdown(top, tt.timeout, func(ctx context.Context) {
					if err := ctx.Err(); err != nil {
						t.Fatalf("shutdown context should not be canceled at start: %v", err)
					}
					close(completed)
				})

				select {
				case <-completed:
					if !tt.preCancel {
						t.Fatal("lazy shutdown ran before parent cancellation")
					}
				default:
					if tt.preCancel {
						select {
						case <-completed:
						case <-time.After(200 * time.Millisecond):
							t.Fatal("lazy shutdown did not run for already canceled parent")
						}
					}
				}

				if tt.preCancel {
					return
				}

				cancel()

				select {
				case <-completed:
				case <-time.After(200 * time.Millisecond):
					t.Fatal("lazy shutdown did not run after parent cancellation")
				}
			})
		})
	}
}
