package http

import (
	"net"
	"net/http"
	"net/url"
	"sync"
	"syscall"
	"testing"
	"time"

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

	cfg.VersionEnabled = true
	lis, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	// set a random address
	cfg.Address = lis.Addr().String()

	ops, err := NewOPSServer(cfg, log)
	require.NoError(t, err)

	ctx, cancel := service.SignalContext(t.Context(),
		syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()

	done := make(chan struct{})
	wait := new(sync.WaitGroup)
	wait.Add(2)

	go func() {
		<-ctx.Done()
		ops.Stop(t.Context())
		wait.Done()
	}()

	go func() {
		close(done)
		assert.NoError(t, ops.Start(ctx))
		wait.Done()
	}()

	<-done // wait for run
	links := []string{
		cfg.ExpVarsPath,
		cfg.MetricsPath,
		cfg.ProfilePath,
		cfg.VersionPath,
		cfg.VersionPath + "?format=json",
	}

	time.Sleep(time.Millisecond * 100) // wait for server up

	for i, link := range links {
		uri, errBlock := url.Parse("//" + cfg.Address)
		require.NoError(t, errBlock)

		ref, errRef := url.Parse(link)
		require.NoError(t, errRef)

		uri.Scheme = "http"
		req, errReq := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			uri.ResolveReference(ref).String(),
			http.NoBody,
		)
		require.NoError(t, errReq)

		t.Logf("Request #%d: %s", i, link)

		resp, errResp := http.DefaultClient.Do(req)
		require.NoError(t, errResp)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusOK, resp.StatusCode, req.URL)
	}

	cancel()
	wait.Wait()
}
