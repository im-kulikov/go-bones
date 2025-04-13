package network

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"testing"

	"github.com/im-kulikov/gonfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

func Test_opsServer(t *testing.T) {
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	// set random address
	cfg.Address = lis.Addr().String()

	ops, err := NewOPSServer(cfg, log)
	require.NoError(t, err)

	ctx, cancel := service.SignalContext(context.TODO(),
		syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	done := make(chan struct{})
	wait := new(sync.WaitGroup)
	wait.Add(2)

	go func() {
		defer wait.Done()

		<-ctx.Done()
		ops.Stop(context.TODO())
	}()

	go func() {
		defer wait.Done()

		close(done)
		assert.ErrorIs(t, ops.Start(ctx), service.ErrCancelCalled)
	}()

	<-done // wait for run
	links := []string{
		cfg.ExpVarsPath,
		cfg.MetricsPath,
		cfg.ProfilePath,
	}

	for _, link := range links {
		uri, errBlock := url.Parse("//" + cfg.Address)
		require.NoError(t, errBlock)

		uri.Scheme = "http"
		req, errBlock := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			uri.JoinPath(link).String(),
			http.NoBody,
		)
		require.NoError(t, errBlock)

		resp, errBlock := http.DefaultClient.Do(req)
		require.NoError(t, errBlock)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode)

	}

	cancel()
	wait.Wait()
}
