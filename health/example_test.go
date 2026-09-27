package health_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/logger"
)

func ExampleMonitor_Status() {
	hc := health.New(config.Health{}, logger.ForTests())

	kafka, _ := hc.Status("kafka", health.WithImpact(health.Informational))
	kafka.Set(health.PublicError("broker unavailable", errors.New("dial tcp 10.0.0.7:9092: refused")))

	res := hc.Snapshot().Checks["kafka"]
	fmt.Println(res.Status, "-", health.PublicMessage(res.Err))
	// Output: failing - broker unavailable
}

func ExampleMonitor_Register() {
	hc := health.New(config.Health{}, logger.ForTests())

	err := hc.Register("postgres", health.CheckerFunc(func(ctx context.Context) error {
		return nil // for example pool.Ping(ctx)
	}), health.WithInterval(5*time.Second), health.WithTimeout(time.Second), health.WithThresholds(2, 1))
	fmt.Println(err)

	err = hc.Register("postgres", health.CheckerFunc(func(context.Context) error { return nil }))
	fmt.Println(errors.Is(err, health.ErrDuplicateCheck))
	// Output:
	// <nil>
	// true
}
