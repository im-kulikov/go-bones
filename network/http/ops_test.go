package http

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
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

func resetOpsRegistry(t *testing.T) {
	t.Helper()

	previousRegistry := registry.Load()
	previousRuntimeMetricsRegistered := runtimeMetricsRegistered.Load()

	registry.Store(nil)
	runtimeMetricsRegistered.Store(false)

	t.Cleanup(func() {
		registry.Store(previousRegistry)
		runtimeMetricsRegistered.Store(previousRuntimeMetricsRegistered)
	})
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

func TestRegisterMetrics(t *testing.T) {
	resetOpsRegistry(t)

	custom := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "go_bones_ops_custom_metric",
		Help: "Custom OPS metric for tests.",
	})
	custom.Set(42)

	require.NoError(t, RegisterMetrics(custom))

	families, err := getRegistry().Gather()
	require.NoError(t, err)
	require.Len(t, families, 1)
	assert.Equal(t, "go_bones_ops_custom_metric", families[0].GetName())
	assert.Equal(t, float64(42), families[0].GetMetric()[0].GetGauge().GetValue())
}

func TestRegisterMetrics_ReturnsCollectorError(t *testing.T) {
	resetOpsRegistry(t)

	custom := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "go_bones_ops_duplicate_metric",
		Help: "Duplicate OPS metric for tests.",
	})

	require.NoError(t, RegisterMetrics(custom))
	require.Error(t, RegisterMetrics(custom))
}

func TestGetRegistry_InitializesOnceConcurrently(t *testing.T) {
	resetOpsRegistry(t)

	const workers = 128
	start := make(chan struct{})
	results := make(chan *prometheus.Registry, workers)

	var wait sync.WaitGroup
	for range workers {
		wait.Go(func() {
			<-start
			results <- getRegistry()
		})
	}

	close(start)
	wait.Wait()
	close(results)

	got := make(map[*prometheus.Registry]int)
	for item := range results {
		require.NotNil(t, item)
		got[item]++
	}

	require.Len(t, got, 1)
}

func TestRegisterRuntimeMetrics_IgnoresAlreadyRegisteredRuntimeCollector(t *testing.T) {
	resetOpsRegistry(t)

	require.NoError(t, getRegistry().Register(newOpsRuntimeCollector()))

	require.NoError(t, registerRuntimeMetrics())
	require.True(t, runtimeMetricsRegistered.Load())
}

func TestNewOPSServer_RegistersRuntimeMetricsOnce(t *testing.T) {
	resetOpsRegistry(t)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))

	_, err := NewOPSServer(cfg, log)
	require.NoError(t, err)

	_, err = NewOPSServer(cfg, log)
	require.NoError(t, err)
}

func TestNewOPSServer_Disabled(t *testing.T) {
	resetOpsRegistry(t)

	svc, err := NewOPSServer(config.Ops{}, logger.ForTests())
	require.NoError(t, err)
	require.Nil(t, svc)
	require.Nil(t, registry.Load())
	require.False(t, runtimeMetricsRegistered.Load())
}

func TestNewOPSServer_MetricsDisabledSkipsRuntimeRegistration(t *testing.T) {
	resetOpsRegistry(t)

	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))
	cfg.MetricsEnabled = false
	cfg.ProfileEnabled = false
	cfg.ExpVarsEnabled = false
	cfg.VersionEnabled = true

	svc, err := NewOPSServer(cfg, logger.ForTests())
	require.NoError(t, err)
	require.NotNil(t, svc)
	require.Nil(t, registry.Load())
	require.False(t, runtimeMetricsRegistered.Load())
}

func TestOPSServer_DisabledEndpoints(t *testing.T) {
	resetOpsRegistry(t)

	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))
	cfg.MetricsEnabled = false
	cfg.ProfileEnabled = false
	cfg.ExpVarsEnabled = false
	cfg.VersionEnabled = true

	handler, err := newOPSHandler(cfg, logger.ForTests(logger.TestLoggerWriteToTB(t)))
	require.NoError(t, err)
	require.NotNil(t, handler)

	status := func(path string) int {
		req := httptest.NewRequest(MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		return rec.Code
	}

	require.Equal(t, StatusOK, status(cfg.VersionPath))
	require.Equal(t, StatusNotFound, status(cfg.MetricsPath))
	require.Equal(t, StatusNotFound, status(cfg.ExpVarsPath))
	require.Equal(t, StatusNotFound, status(cfg.ProfilePath+"/"))
}

func TestNewOPSServer_ReturnsRuntimeMetricsRegistrationError(t *testing.T) {
	resetOpsRegistry(t)

	conflicting := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "go_sched_gomaxprocs_threads",
		Help: "Conflicting metric descriptor.",
	})
	reg := prometheus.NewRegistry()
	reg.MustRegister(conflicting)
	registry.Store(reg)

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))
	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))

	_, err := NewOPSServer(cfg, log)
	require.Error(t, err)
}

// brokenResponseWriter always fails on Write, to exercise version's error path.
type brokenResponseWriter struct {
	header Header
}

func (b *brokenResponseWriter) Header() Header {
	if b.header == nil {
		b.header = make(Header)
	}

	return b.header
}

func (b *brokenResponseWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }
func (b *brokenResponseWriter) WriteHeader(int)           {}

func TestVersion_LogsWriteFailure(t *testing.T) {
	cases := []struct {
		name  string
		query string
	}{
		{name: "text", query: ""},
		{name: "json", query: "format=json"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := logger.NewSyncBuffer()
			log := logger.ForTests(
				logger.TestLoggerWriteToTB(t),
				logger.TestLoggerWriter(logger.NewSyncWriter(buf)))

			req := httptest.NewRequest(MethodGet, "/version?"+tc.query, nil)
			version(log)(new(brokenResponseWriter), req)

			require.Contains(t, buf.String(), "could not write version response")
		})
	}
}

func TestVersion_Success(t *testing.T) {
	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	cases := []struct {
		name        string
		query       string
		contentType string
	}{
		{name: "text", query: "", contentType: "text/plain; charset=utf-8"},
		{name: "json", query: "format=json", contentType: "application/json; charset=utf-8"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(MethodGet, "/version?"+tc.query, nil)
			rec := httptest.NewRecorder()

			version(log)(rec, req)

			require.Equal(t, StatusOK, rec.Code)
			require.Equal(t, tc.contentType, rec.Header().Get("Content-Type"))
			require.NotEmpty(t, rec.Body.String())
		})
	}
}

func Test_opsServer(t *testing.T) {
	testutil.RequireNetworkIntegration(t)
	client := &Client{Transport: &Transport{DisableKeepAlives: true}}

	log := logger.ForTests(logger.TestLoggerWriteToTB(t))

	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))

	cfg.VersionEnabled = true

	cfg.Address = testutil.FreeTCPAddr(t)

	ops, err := NewOPSServer(cfg, log)
	require.NoError(t, err)

	ctx, cancel := service.SignalContext(
		t.Context(),
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)
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
	<-done

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
			resp, errResp := client.Do(req)
			if errResp != nil {
				return false
			}
			defer func() { _ = resp.Body.Close() }()

			return resp.StatusCode == StatusOK
		}, time.Second, 10*time.Millisecond)
	}

	metricsURL := (&url.URL{Scheme: "http", Host: cfg.Address, Path: cfg.MetricsPath}).String()
	_, _, body := opsGet(ctx, t, client, metricsURL)

	metricsText := string(body)
	for _, name := range requiredOpsRuntimeMetricNames {
		assert.Contains(t, metricsText, name)
	}

	assert.NotContains(t, metricsText, "gm_runtime{")
	testutil.WriteArtifact(t, "ops-metrics.prom", body)

	goroutineLeakURL := (&url.URL{Scheme: "http", Host: cfg.Address, Path: cfg.ProfilePath + "/goroutineleak"}).String()
	code, header, goroutineLeakBody := opsGet(ctx, t, client, goroutineLeakURL)

	if rpprof.Lookup("goroutineleak") != nil {
		assert.Equal(t, StatusOK, code)
		assert.NotEmpty(t, goroutineLeakBody)
		assert.Equal(t, "application/octet-stream", header.Get("Content-Type"))
		testutil.WriteArtifact(t, "ops-goroutineleak.pprof", goroutineLeakBody)
		t.Logf("Artifact wrote to %s", t.ArtifactDir())
	} else {
		assert.Equal(t, StatusNotFound, code)
		assert.Contains(t, string(goroutineLeakBody), "Unknown profile")
	}

	cancel()
	wait.Wait()
}

// opsGet performs a GET request and returns the status, headers and the fully read body.
func opsGet(
	ctx context.Context,
	t *testing.T,
	client *Client,
	rawURL string,
) (int, Header, []byte) {
	t.Helper()

	req, err := NewRequestWithContext(ctx, MethodGet, rawURL, NoBody)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	return resp.StatusCode, resp.Header, body
}

// TestGetRegistry_CASLoserPath deterministically covers the branch that
// TestGetRegistry_ConcurrentCASLoserPath below only covers probabilistically:
// registryCompareAndSwap is overridden to simulate another goroutine's CAS
// winning first, so getRegistry must fall back to registry.Load(). Relying
// only on the concurrent stress test for this line is flaky under load (it
// needs a real scheduling race to actually happen), so this test guarantees
// it regardless of timing.
func TestGetRegistry_CASLoserPath(t *testing.T) {
	resetOpsRegistry(t)

	winner := prometheus.NewRegistry()

	prev := registryCompareAndSwap
	t.Cleanup(func() { registryCompareAndSwap = prev })
	registryCompareAndSwap = func(*prometheus.Registry) bool {
		registry.Store(winner) // simulate the other goroutine's CAS winning first
		return false
	}

	require.Same(t, winner, getRegistry())
}

func TestGetRegistry_ConcurrentCASLoserPath(t *testing.T) {
	resetOpsRegistry(t)

	const workers = 4096
	start := make(chan struct{})
	results := make(chan *prometheus.Registry, workers)

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			<-start
			results <- getRegistry()
		})
	}

	close(start)
	wg.Wait()
	close(results)

	var first *prometheus.Registry
	for r := range results {
		require.NotNil(t, r)
		if first == nil {
			first = r
			continue
		}
		require.Same(t, first, r)
	}
}
