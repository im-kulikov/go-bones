// It's okay to expose pprof from this binary since the port it is exposed on
// is not accessible from the outside of Kubernetes cluster (only inside of it).
//
// #nosec G108 (CWE-200): Profiling endpoint is automatically exposed on /debug/pprof

package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/pprof"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

// OpsConfig provides configuration for http server.
type OpsConfig struct {
	Disable bool   `env:"DISABLE" default:"false" usage:"allows to disable ops server"`
	Address string `env:"ADDRESS" default:":8081" usage:"allows to set set ops address:port"`
	Network string `env:"NETWORK" default:"tcp" usage:"allows to set ops listen network: tcp/udp"`
	NoTrace bool   `env:"NO_TRACE" default:"true" usage:"allows to disable tracing"`

	MetricsPath string `env:"METRICS_PATH" default:"/metrics" usage:"allows to set custom metrics path"`
	HealthyPath string `env:"HEALTHY_PATH" default:"/healthy" usage:"allows to set custom healthy path"`
	ProfilePath string `env:"PROFILE_PATH" default:"/debug/pprof" usage:"allows to set custom profiler path"`
}

// opsWorker implements service.Service
// and used as worker pool for HealthChecker.
type opsWorker struct {
	log logger.Logger

	resp *sync.Map
	list []HealthChecker
}

// HealthChecker provides functionality to check health of any entity
// that implement this interface.
type HealthChecker interface {
	Name() string
	Interval() time.Duration
	Healthy(ctx context.Context) error
}

const (
	opsServiceName = "ops-server"
	opsWorkersName = "ops-health-checker"
)

func (o *OpsConfig) httpOption() HTTPOption {
	return WithHTTPConfig(HTTPConfig{
		Disable: o.Disable,
		Address: o.Address,
		Network: o.Network,
		NoTrace: o.NoTrace,
	})
}

// NewOpsServer creates new OPS server and OPS HealthChecker's worker.
func NewOpsServer(log logger.Logger, cfg OpsConfig, list ...HealthChecker) (service.Service, service.Service) {
	mux := http.NewServeMux()

	// prepare HealthChecker's worker and handler
	wrk, handler := newHealthWorkers(log, list...)

	// health checker
	mux.Handle(cfg.HealthyPath, handler)

	// Expose the registered pprof via HTTP.
	mux.HandleFunc(cfg.ProfilePath+"/", pprof.Index)
	mux.HandleFunc(cfg.ProfilePath+"/cmdline", pprof.Cmdline)
	mux.HandleFunc(cfg.ProfilePath+"/profile", pprof.Profile)
	mux.HandleFunc(cfg.ProfilePath+"/symbol", pprof.Symbol)
	mux.HandleFunc(cfg.ProfilePath+"/trace", pprof.Trace)

	// metrics
	mux.Handle(cfg.MetricsPath, promhttp.Handler())

	return wrk, NewHTTPServer(
		cfg.httpOption(),
		WithHTTPLogger(log),
		WithHTTPHandler(mux),
		WithHTTPName(opsServiceName))
}

func newHealthWorkers(log logger.Logger, list ...HealthChecker) (service.Service, http.Handler) {
	wrk := &opsWorker{log: log, list: list, resp: new(sync.Map)}
	if len(list) == 0 {
		return nil, wrk
	}

	return service.NewWorker(opsWorkersName, wrk.launcher), wrk
}

// ServeHTTP implementation of http.Handler for OPS worker.
func (o *opsWorker) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	out := make(map[string]interface{})
	o.resp.Range(func(key, val any) bool {
		if name, ok := key.(string); ok {
			out[name] = val
		}

		return true
	})

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(out); err != nil {
		o.log.Errorf("could not write response: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}
}

// implementation of service.Launcher for OPS worker.
func (o *opsWorker) launcher(ctx context.Context) error {
	wg := new(sync.WaitGroup)

	wg.Add(len(o.list))
	for i := 0; i < len(o.list); i++ {
		if o.list[i] == nil {
			wg.Done()

			continue
		}

		o.resp.Store(o.list[i].Name(), 0)

		// run health checker for each service
		go func(checker HealthChecker) {
			defer wg.Done()

			name := checker.Name()
			delay := checker.Interval()

			ticker := time.NewTimer(delay)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := checker.Healthy(ctx); err != nil {
						o.resp.Store(name, 1)
						o.log.Errorf("check service %s failed with error: %v", name, err)
					} else {
						o.resp.Store(name, 0)
					}

					ticker.Reset(delay)
				}
			}
		}(o.list[i])
	}

	<-ctx.Done()

	wg.Wait()

	return nil
}
