package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/im-kulikov/gonfig"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/logger"
)

var errDSN = errors.New("dial postgres://user:secret@10.0.0.7:5432: connection refused")

// fakeReader returns a fixed snapshot recomputed like the monitor does.
type fakeReader struct{ snap health.Snapshot }

func (f fakeReader) Snapshot() health.Snapshot         { return f.snap }
func (fakeReader) Subscribe(func(health.Event)) func() { return func() {} }

func snapshotOf(running, draining bool, results ...health.Result) health.Snapshot {
	checks := make(map[string]health.Result, len(results))
	for _, res := range results {
		checks[res.Name] = res
	}

	// Without() with a name that does not exist recomputes the aggregate.
	return health.Snapshot{Running: running, Draining: draining, Checks: checks}.Without("\x00")
}

func opsConfig(t *testing.T) config.Ops {
	t.Helper()

	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))
	cfg.MetricsEnabled = false
	cfg.ProfileEnabled = false
	cfg.ExpVarsEnabled = false

	return cfg
}

type response struct {
	code   int
	body   string
	header Header
}

func call(t *testing.T, h Handler, method, target string) response {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))

	body, err := io.ReadAll(rec.Result().Body)
	require.NoError(t, err)

	return response{code: rec.Code, body: string(body), header: rec.Header()}
}

func TestHealthEndpoints_WithoutReader(t *testing.T) {
	cfg := opsConfig(t)
	cfg.LivePath, cfg.ReadyPath, cfg.HealthPath = "", "", ""

	h, err := newOPSHandler(cfg, logger.ForTests(), nil)
	require.NoError(t, err)

	for _, path := range []string{"/livez", "/readyz"} {
		res := call(t, h, MethodGet, path)
		require.Equal(t, StatusOK, res.code, path)
		require.Equal(t, "ok\n", res.body)
		require.Equal(t, "no-store", res.header.Get("Cache-Control"))
		require.Equal(t, contentTypeText, res.header.Get("Content-Type"))
	}

	res := call(t, h, MethodGet, "/healthz")
	require.Equal(t, StatusOK, res.code)
	require.JSONEq(
		t,
		`{"status":"ok","live":true,"ready":true,"draining":false,"checks":{}}`,
		res.body,
	)

	res = call(t, h, MethodPost, "/readyz")
	require.Equal(t, StatusMethodNotAllowed, res.code)
	require.Equal(t, "GET, HEAD", res.header.Get("Allow"))

	require.Equal(t, StatusNotFound, call(t, h, MethodGet, "/livez/missing").code)
	require.Equal(t, StatusNotFound, call(t, h, MethodGet, "/readyz/").code)
}

func TestHealthEndpoints_Disabled(t *testing.T) {
	cfg := opsConfig(t)
	cfg.HealthEnabled = false
	cfg.VersionEnabled = true

	h, err := newOPSHandler(cfg, logger.ForTests())
	require.NoError(t, err)
	require.Equal(t, StatusNotFound, call(t, h, MethodGet, "/readyz").code)
}

func TestHealthEndpoints_Snapshot(t *testing.T) {
	now := time.Date(2026, 9, 22, 10, 15, 4, 0, time.UTC)
	reader := fakeReader{snap: snapshotOf(
		true,
		false,
		health.Result{
			Name: "postgres", Impact: health.Readiness, Status: health.StatusFailing,
			Err: errDSN, CheckedAt: now, Duration: 4 * time.Millisecond, ConsecutiveFailures: 3,
		},
		health.Result{
			Name: "kafka", Impact: health.Informational, Status: health.StatusFailing,
			Err: health.PublicError("broker unavailable", errDSN), CheckedAt: now,
		},
		health.Result{
			Name:      "worker",
			Impact:    health.Liveness,
			Status:    health.StatusPassing,
			CheckedAt: now,
		},
		health.Result{Name: "boot", Impact: health.Liveness, Status: health.StatusUnknown},
		health.Result{Name: "cache", Impact: health.Readiness, Status: health.StatusUnknown},
		health.Result{
			Name:   "redis",
			Impact: health.Readiness,
			Status: health.StatusPassing,
			Stale:  true,
		},
	)}

	h, err := newOPSHandler(opsConfig(t), logger.ForTests(), WithHealth(reader))
	require.NoError(t, err)

	t.Run("livez ignores readiness", func(t *testing.T) {
		res := call(t, h, MethodGet, "/livez?verbose")
		require.Equal(t, StatusOK, res.code)
		require.Equal(t, "[+]boot ok\n[+]worker ok\nlivez check passed\n", res.body)
		require.Equal(t, StatusOK, call(t, h, MethodGet, "/livez/worker").code)
		require.Equal(t, StatusOK, call(t, h, MethodGet, "/livez/boot").code)
		require.Equal(t, StatusNotFound, call(t, h, MethodGet, "/livez/postgres").code)
	})

	t.Run("readyz short", func(t *testing.T) {
		res := call(t, h, MethodGet, "/readyz")
		require.Equal(t, StatusServiceUnavailable, res.code)
		require.Equal(t, "[-]cache failed: not checked yet\n[-]postgres failed: error\n"+
			"[-]redis failed: stale\nreadyz check failed\n", res.body)
		require.NotContains(t, res.body, "secret")
	})

	t.Run("readyz exclude", func(t *testing.T) {
		res := call(t, h, MethodGet, "/readyz?exclude=postgres&exclude=cache&exclude=redis&verbose")
		require.Equal(t, StatusOK, res.code)
		require.Equal(
			t,
			"[+]boot ok\n[+]worker ok\n[+]postgres excluded: ok\n[+]cache excluded: ok\n"+
				"[+]redis excluded: ok\nreadyz check passed\n",
			res.body,
		)
	})

	t.Run("single check", func(t *testing.T) {
		res := call(t, h, MethodGet, "/readyz/postgres")
		require.Equal(t, StatusServiceUnavailable, res.code)
		require.Equal(t, "[-]postgres failed: error\n", res.body)
		require.Equal(t, StatusNotFound, call(t, h, MethodGet, "/readyz/kafka").code)
	})

	t.Run("readyz json", func(t *testing.T) {
		res := call(t, h, MethodGet, "/readyz?format=json")
		require.Equal(t, StatusServiceUnavailable, res.code)
		require.Equal(t, contentTypeJSON, res.header.Get("Content-Type"))

		var out healthReport
		require.NoError(t, json.Unmarshal([]byte(res.body), &out))
		require.Equal(t, "failing", out.Status)
		require.NotContains(t, out.Checks, "kafka")
		require.Contains(t, out.Checks, "worker")
	})

	t.Run("healthz", func(t *testing.T) {
		res := call(t, h, MethodGet, "/healthz")
		require.Equal(t, StatusServiceUnavailable, res.code)
		require.NotContains(t, res.body, "secret")
		require.NotContains(t, res.body, "10.0.0.7")

		var out map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.body), &out))
		require.Equal(t, "failing", out["status"])

		checks := out["checks"].(map[string]any)
		require.Equal(t, map[string]any{
			"impact": "readiness", "status": "failing", "checked_at": "2026-09-22T10:15:04Z",
			"duration_ms": float64(4), "error": "error", "consecutive_failures": float64(3),
		}, checks["postgres"])
		require.Equal(t, "broker unavailable", checks["kafka"].(map[string]any)["error"])
		require.Equal(t, true, checks["redis"].(map[string]any)["stale"])
	})

	t.Run("healthz degraded is 200", func(t *testing.T) {
		res := call(t, h, MethodGet, "/healthz?exclude=postgres&exclude=cache&exclude=redis")
		require.Equal(t, StatusOK, res.code)
		require.Contains(t, res.body, `"status":"degraded"`)
	})

	t.Run("head", func(t *testing.T) {
		res := call(t, h, MethodHead, "/readyz")
		require.Equal(t, StatusServiceUnavailable, res.code)
	})
}

func TestHealthEndpoints_DrainingAndStopped(t *testing.T) {
	draining := fakeReader{snap: snapshotOf(true, true)}
	h, err := newOPSHandler(opsConfig(t), logger.ForTests(), WithHealth(draining))
	require.NoError(t, err)

	res := call(t, h, MethodGet, "/readyz")
	require.Equal(t, StatusServiceUnavailable, res.code)
	require.Equal(t, "[-]draining failed: shutting down\nreadyz check failed\n", res.body)
	require.Equal(t, StatusOK, call(t, h, MethodGet, "/livez").code)

	stopped := fakeReader{snap: snapshotOf(false, false)}
	h, err = newOPSHandler(opsConfig(t), logger.ForTests(), WithHealth(stopped))
	require.NoError(t, err)

	res = call(t, h, MethodGet, "/livez")
	require.Equal(t, StatusServiceUnavailable, res.code)
	require.Equal(t, "[-]monitor failed: not running\nlivez check failed\n", res.body)
}

func TestHealthEndpoints_HungCheckerDoesNotBlock(t *testing.T) {
	hc := health.New(config.Health{}, logger.ForTests())
	release := make(chan struct{})
	defer close(release)

	require.NoError(t, hc.Register("hung", health.CheckerFunc(func(context.Context) error {
		<-release

		return nil
	})))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() { _ = hc.Start(ctx) }()

	h, err := newOPSHandler(opsConfig(t), logger.ForTests(), WithHealth(hc))
	require.NoError(t, err)

	const requests = 100

	begin := time.Now()
	for range requests {
		require.Equal(t, StatusServiceUnavailable, call(t, h, MethodGet, "/readyz").code)
		call(t, h, MethodGet, "/healthz")
	}

	require.Less(t, time.Since(begin)/(2*requests), time.Millisecond)
}

func TestHealthEndpoints_Metrics(t *testing.T) {
	resetOpsRegistry(t)

	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))

	hc := health.New(config.Health{}, logger.ForTests())

	h, err := newOPSHandler(cfg, logger.ForTests(), WithHealth(hc))
	require.NoError(t, err)

	_, err = NewOPSServer(cfg, logger.ForTests(), WithHealth(hc))
	require.NoError(t, err, "the same collector registered twice is fine")

	_, err = newOPSHandler(cfg, logger.ForTests(), WithHealth(fakeReader{}))
	require.NoError(t, err, "readers that are not collectors are skipped")

	res := call(t, h, MethodGet, cfg.MetricsPath)
	require.Equal(t, StatusOK, res.code)

	for _, name := range []string{"go_bones_health_live", "go_bones_health_ready", "go_bones_health_draining"} {
		require.True(t, strings.Contains(res.body, name+" 0"), name)
	}

	require.Contains(
		t,
		res.body,
		"go_sched_goroutines_goroutines",
		"runtime metrics are still exported",
	)
}

func TestHealthEndpoints_MetricsConflict(t *testing.T) {
	resetOpsRegistry(t)

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGauge(prometheus.GaugeOpts{Name: "go_bones_health_live", Help: "conflict"}),
	)
	registry.Store(reg)
	runtimeMetricsRegistered.Store(true)

	var cfg config.Ops
	require.NoError(t, gonfig.SetDefaults(&cfg))

	_, err := newOPSHandler(
		cfg,
		logger.ForTests(),
		WithHealth(health.New(config.Health{}, logger.ForTests())),
	)
	require.Error(t, err)
}

// failingWriter fails on Write to exercise logging of write errors.
type failingWriter struct{ brokenResponseWriter }

func TestHealthEndpoints_WriteErrors(t *testing.T) {
	h := &healthHandlers{reader: staticReader{}, log: logger.ForTests()}
	req := httptest.NewRequest(MethodGet, "/readyz", nil)

	require.NotPanics(t, func() {
		h.probe(probeReady, "readyz")(new(failingWriter), req)
		h.report(new(failingWriter), req)
	})

	unsubscribe := staticReader{}.Subscribe(nil)
	require.NotNil(t, unsubscribe)
	require.NotPanics(t, unsubscribe)
}

func TestResultMessage(t *testing.T) {
	require.Equal(t, "error", resultMessage(health.Result{Status: health.StatusFailing}))
	require.Equal(
		t,
		"timeout",
		resultMessage(health.Result{Status: health.StatusFailing, Err: health.ErrTimeout}),
	)
}

func TestHealthEndpoints_StaleUnknownLiveness(t *testing.T) {
	// A heartbeat that never beat within its TTL is unknown and stale: the
	// aggregate treats it as down, so every endpoint must agree.
	reader := fakeReader{snap: snapshotOf(true, false, health.Result{
		Name: "worker", Impact: health.Liveness, Status: health.StatusUnknown, Stale: true,
	})}

	h, err := newOPSHandler(opsConfig(t), logger.ForTests(), WithHealth(reader))
	require.NoError(t, err)

	res := call(t, h, MethodGet, "/livez?verbose")
	require.Equal(t, StatusServiceUnavailable, res.code)
	require.Contains(t, res.body, "[-]worker failed: stale")

	res = call(t, h, MethodGet, "/livez")
	require.Equal(t, StatusServiceUnavailable, res.code)
	require.Contains(t, res.body, "[-]worker failed: stale")

	res = call(t, h, MethodGet, "/livez/worker")
	require.Equal(t, StatusServiceUnavailable, res.code)
}

func TestHealthEndpoints_TrailingSlashPaths(t *testing.T) {
	cfg := opsConfig(t)
	cfg.LivePath, cfg.ReadyPath = "/livez/", "/readyz/"

	reader := fakeReader{snap: snapshotOf(true, false, health.Result{
		Name: "worker", Impact: health.Liveness, Status: health.StatusPassing,
	})}

	var h Handler
	require.NotPanics(t, func() {
		var err error
		h, err = newOPSHandler(cfg, logger.ForTests(), WithHealth(reader))
		require.NoError(t, err)
	})

	require.Equal(t, StatusOK, call(t, h, MethodGet, "/livez").code)
	require.Equal(t, StatusOK, call(t, h, MethodGet, "/readyz").code)
	require.Equal(t, StatusOK, call(t, h, MethodGet, "/livez/worker").code)
}

func TestHealthEndpoints_NilLogger(t *testing.T) {
	reader := fakeReader{snap: snapshotOf(true, false)}

	h, err := newOPSHandler(opsConfig(t), nil, WithHealth(reader))
	require.NoError(t, err)

	require.NotPanics(t, func() {
		h.ServeHTTP(new(failingWriter), httptest.NewRequest(MethodGet, "/livez", nil))
	})
}

func TestHealthEndpoints_RootPath(t *testing.T) {
	cfg := opsConfig(t)
	cfg.LivePath = "/"

	reader := fakeReader{snap: snapshotOf(true, false, health.Result{
		Name: "worker", Impact: health.Liveness, Status: health.StatusPassing,
	})}

	h, err := newOPSHandler(cfg, logger.ForTests(), WithHealth(reader))
	require.NoError(t, err)

	res := call(t, h, MethodGet, "/?verbose")
	require.Equal(t, StatusOK, res.code)
	require.Contains(t, res.body, "livez check passed")

	require.Equal(t, StatusOK, call(t, h, MethodGet, "/worker").code)
	require.Equal(t, StatusNotFound, call(t, h, MethodGet, "/livez").code,
		"a configured root path must not be replaced with the default")
	require.Equal(t, StatusOK, call(t, h, MethodGet, "/readyz").code)
}
