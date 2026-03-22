package http

import (
	"io"
	"net"
	"net/url"
	rpprof "runtime/pprof"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/internal/testutil"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

var requiredOpsRuntimeMetricNames = []string{ //nolint:gochecknoglobals
	"go_sched_gomaxprocs_threads",
	"go_sched_goroutines_goroutines",
	"go_sched_goroutines_runnable_goroutines",
	"go_sched_goroutines_running_goroutines",
	"go_sched_goroutines_waiting_goroutines",
	"go_sched_goroutines_not_in_go_goroutines",
	"go_sched_threads_total_threads",
	"go_sched_latencies_seconds",
	"go_gc_cycles_total_gc_cycles_total",
	"go_gc_heap_goal_bytes",
	"go_gc_heap_live_bytes",
	"go_memory_classes_total_bytes",
	"go_memory_classes_heap_objects_bytes",
	"go_memory_classes_heap_stacks_bytes",
}

func Test_opsRuntimeCollector_ExportsGoRuntimeMetrics(t *testing.T) {
	register := prometheus.NewRegistry()
	register.MustRegister(newOpsRuntimeCollector())

	families, err := register.Gather()
	require.NoError(t, err)

	gotNames := make([]string, 0, len(families))
	for _, family := range families {
		gotNames = append(gotNames, family.GetName())
	}

	for _, name := range requiredOpsRuntimeMetricNames {
		assert.Contains(t, gotNames, name)
	}

	assert.NotContains(t, gotNames, "gm_runtime")
}

func Test_opsServer(t *testing.T) {
	testutil.RequireNetworkIntegration(t)

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

	wait.Go(func() {
		<-ctx.Done()
		ops.Stop(t.Context())
	})

	wait.Go(func() {
		close(done)
		assert.NoError(t, ops.Start(ctx))
	})

	<-done // wait for run
	links := []string{
		cfg.ExpVarsPath,
		cfg.MetricsPath,
		cfg.ProfilePath,
		cfg.ProfilePath + "/goroutine",
		cfg.ProfilePath + "/heap",
		cfg.VersionPath,
		cfg.VersionPath + "?format=json",
	}

	for i, link := range links {
		require.Eventually(t, func() bool {
			uri, errBlock := url.Parse("//" + cfg.Address)
			require.NoError(t, errBlock)

			ref, errRef := url.Parse(link)
			require.NoError(t, errRef)

			uri.Scheme = "http"
			req, errReq := NewRequestWithContext(
				ctx,
				MethodGet,
				uri.ResolveReference(ref).String(),
				NoBody,
			)
			require.NoError(t, errReq)

			t.Logf("Request #%d: %s", i, link)

			resp, errResp := DefaultClient.Do(req)
			if errResp != nil {
				return false
			}

			defer func() { _ = resp.Body.Close() }()

			return resp.StatusCode == StatusOK
		}, time.Second, 10*time.Millisecond)
	}

	metricsURL := (&url.URL{
		Scheme: "http",
		Host:   cfg.Address,
		Path:   cfg.MetricsPath,
	}).String()
	req, err := NewRequestWithContext(ctx, MethodGet, metricsURL, NoBody)
	require.NoError(t, err)

	resp, err := DefaultClient.Do(req)
	if err != nil && resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.NoError(t, err)
	defer func(body io.ReadCloser) { _ = body.Close() }(resp.Body)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	metricsText := string(body)
	for _, name := range requiredOpsRuntimeMetricNames {
		assert.Contains(t, metricsText, name)
	}

	assert.NotContains(t, metricsText, "gm_runtime{")
	testutil.WriteArtifact(t, "ops-metrics.prom", body)

	goroutineLeakURL := (&url.URL{
		Scheme: "http",
		Host:   cfg.Address,
		Path:   cfg.ProfilePath + "/goroutineleak",
	}).String()
	req, err = NewRequestWithContext(
		ctx,
		MethodGet,
		goroutineLeakURL,
		NoBody,
	)
	require.NoError(t, err)

	resp, err = DefaultClient.Do(req)
	if err != nil && resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.NoError(t, err)
	defer func(body io.ReadCloser) { _ = body.Close() }(resp.Body)

	goroutineLeakBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	if rpprof.Lookup("goroutineleak") != nil {
		assert.Equal(t, StatusOK, resp.StatusCode)
		assert.NotEmpty(t, goroutineLeakBody)
		assert.Equal(t, "application/octet-stream", resp.Header.Get("Content-Type"))
		testutil.WriteArtifact(t, "ops-goroutineleak.pprof", goroutineLeakBody)
		t.Logf("Artifact wrote to %s", t.ArtifactDir())
	} else {
		assert.Equal(t, StatusNotFound, resp.StatusCode)
		assert.Contains(t, string(goroutineLeakBody), "Unknown profile")
	}

	cancel()
	wait.Wait()
}
