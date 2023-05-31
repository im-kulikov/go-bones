package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cristalhq/aconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

type fakeHealthChecker string

type assertion func(t *testing.T, actual []byte)

const (
	defaultOPSNetwork = "tcp"

	opsRouteHealthy  = "/healthy"
	opsRouteMetrics  = "/metrics"
	opsRouteProfiler = "/debug/pprof"
)

func (f fakeHealthChecker) Name() string { return string(f) }

func (f fakeHealthChecker) Interval() time.Duration { return time.Millisecond * 5 }

func (f fakeHealthChecker) Healthy(_ context.Context) error {
	if value := string(f); strings.Contains(value, "error") {
		return errors.New(value)
	}

	return nil
}

func testURI(host, path string) string {
	return (&url.URL{
		Scheme: "http",
		Host:   host,
		Path:   path,
	}).String()
}

func prepareConfig(t *testing.T, envs ...string) OpsConfig {
	var cfg OpsConfig

	require.NoError(t, aconfig.LoaderFor(&cfg, aconfig.Config{
		Envs:         envs,
		Args:         []string{},
		SkipDefaults: false,
	}).Load())

	return cfg
}

func containsComparer(expect string) assertion {
	return func(t *testing.T, actual []byte) {
		t.Helper()

		// we expect that response body will contain our string
		assert.Contains(t, string(actual), expect)
	}
}

func TestDisabledOpsServer(t *testing.T) {
	log := logger.ForTests(t)

	lis, err := net.Listen(defaultOPSNetwork, "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	cfg := prepareConfig(t,
		"NO_TRACE=true",
		"ADDRESS="+lis.Addr().String(),
		"NETWORK="+lis.Addr().Network())

	ops := NewOpsServer(log, cfg)
	require.Empty(t, ops)
}

func TestEmptyHealthCheckers(t *testing.T) {
	log := logger.ForTests(t)

	lis, err := net.Listen(defaultOPSNetwork, "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	cfg := prepareConfig(t,
		"NO_TRACE=true",
		"ENABLED=true",
		"ADDRESS="+lis.Addr().String(),
		"NETWORK="+lis.Addr().Network())

	ops := NewOpsServer(log, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	defer cancel()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()

		time.Sleep(time.Millisecond * 10)

		req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, testURI(cfg.Address, opsRouteHealthy), nil)
		assert.NoError(t, errReq)

		res, errRes := new(http.Client).Do(req)
		assert.NoError(t, errRes)

		data, errRead := io.ReadAll(res.Body)
		assert.NoError(t, errRead)

		assert.NoError(t, res.Body.Close())
		assert.Equal(t, []byte("{}"), bytes.TrimSpace(data))

		cancel()
	}()

	require.NoError(t, service.New(log,
		service.WithService(ops),
		service.WithShutdownTimeout(time.Millisecond)).Run(ctx))

	wg.Wait()
}

func TestNewOpsServer(t *testing.T) {
	log := logger.ForTests(t)
	defer func() { require.NoError(t, log.Sync()) }()

	lis, err := net.Listen(defaultOPSNetwork, "127.0.0.1:0")
	require.NoError(t, err)
	require.NoError(t, lis.Close())

	cfg := prepareConfig(t,
		"NO_TRACE=false",
		"ENABLED=true",
		"ADDRESS="+lis.Addr().String(),
		"NETWORK="+lis.Addr().Network())

	ops := NewOpsServer(log, cfg,
		nil, // should be ignored
		fakeHealthChecker("test"),
		fakeHealthChecker("test-with-error"))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})

	require.Equal(t, ops.Name(), opsServiceName)

	go func() {
		// wait until service will start
		time.Sleep(time.Millisecond * 10)

		cli := new(http.Client)

		routes := map[string]assertion{
			opsRouteProfiler: containsComparer("Profile Descriptions"),
			opsRouteMetrics:  containsComparer("go_memstats_frees_total"),
			opsRouteHealthy:  containsComparer(`{"test":0,"test-with-error":1}`),
		}

		for route, comparer := range routes {
			t.Run("check ops router "+route, func(t *testing.T) {
				req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, testURI(cfg.Address, route), nil)
				assert.NoError(t, errReq)

				res, errRes := cli.Do(req)
				assert.NoError(t, errRes)
				assert.Equal(t, http.StatusOK, res.StatusCode)

				if comparer != nil {
					var data []byte
					data, err = io.ReadAll(res.Body)
					assert.NoError(t, err)

					comparer(t, bytes.TrimSpace(data))
				}

				assert.NoError(t, res.Body.Close())
			})
		}

		cancel()
		close(done)
	}()

	assert.NoError(t, service.New(log,
		service.WithService(ops),
		service.WithShutdownTimeout(time.Millisecond)).Run(ctx))

	<-done // wait until stop ops server
}

type bufferedOutput struct {
	*testing.T
	*bytes.Buffer

	code int
}

func newBufferedOutput(t *testing.T, w *bytes.Buffer) *bufferedOutput {
	return &bufferedOutput{T: t, Buffer: w}
}

func (b *bufferedOutput) Header() http.Header { return make(http.Header) }

func (b *bufferedOutput) WriteHeader(code int) { b.code = code }

func (b *bufferedOutput) Logf(s string, i ...interface{}) {
	b.T.Helper()

	_, _ = fmt.Fprintf(b.Buffer, "LOG: "+s, i...)
}

func (b *bufferedOutput) Errorf(s string, i ...interface{}) {
	b.T.Helper()

	_, _ = fmt.Fprintf(b.Buffer, "ERROR: "+s, i...)
}

func (b *bufferedOutput) Write(data []byte) (int, error) {
	return len(data), errShouldFailOnWrite
}

var errShouldFailOnWrite = errors.New("should fail on write")

func TestOpsWorker_ServeHTTP(t *testing.T) {
	out := newBufferedOutput(t, new(bytes.Buffer))
	log := logger.ForTests(out)

	defer func() {
		if t.Failed() {
			t.Log(out.String())
		}
	}()

	_, handler := newHealthWorkers(log)

	require.NotPanics(t, func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)

		var _ http.ResponseWriter

		handler.ServeHTTP(out, req)

		require.Equal(t, http.StatusInternalServerError, out.code)
		require.Contains(t, out.Buffer.String(), errShouldFailOnWrite.Error())
	})
}
