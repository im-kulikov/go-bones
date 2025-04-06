package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

func TestPingPong(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	var wg sync.WaitGroup
	log := logger.ForTests()
	svc := newPingPong(log, time.Millisecond*25)

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		// we should not panic on multiple call of stop
		assert.NotPanics(t, func() {
			svc.Stop(ctx)
			svc.Stop(ctx)
			svc.Stop(ctx)
		})
	}()

	require.NoError(t, RunContext(ctx, log, WithService(svc)))
	wg.Wait()
}
