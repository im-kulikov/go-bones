package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
)

func TestPingPong(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	svc := newPingPong(logger.ForTests(t), time.Second)

	go func() {
		// we should not panic on multiple call of stop
		assert.NotPanics(t, func() {
			svc.Stop(ctx)
			svc.Stop(ctx)
			svc.Stop(ctx)
		})
	}()

	require.NoError(t, svc.Start(ctx))
}
