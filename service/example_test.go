package service_test

import (
	"context"
	"time"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

// A periodic task: runs on start, then every minute, each run bounded by 10s.
func ExampleNewTicker() {
	cleanup := service.NewTicker("cleanup", time.Minute, deleteExpired,
		service.WithTickerTimeout(10*time.Second), service.WithTickerJitter(0.1))

	_ = service.Run(logger.Default(), service.WithService(cleanup))
}

func deleteExpired(context.Context) error { return nil }
