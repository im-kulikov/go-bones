package http

import (
	"encoding/json"
	"errors"
	"expvar"
	"net/http/pprof" // #nosec G108
	"runtime/debug"
	rprof "runtime/pprof" // #nosec G108
	"sync/atomic"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

const (
	defaultOPSServiceName = "ops"
)

// newOpsRuntimeCollector returns the standard Prometheus Go collector with
// runtime/metrics groups enabled for scheduler, memory, and GC diagnostics.
func newOpsRuntimeCollector() prometheus.Collector {
	return collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
	)
}

// buildInfo contains build information read from the embedded build info.
// Initialized at startup using debug.ReadBuildInfo().
//
// nolint:gochecknoglobals
var buildInfo, _ = debug.ReadBuildInfo()

// registry stores the active Prometheus registry used by OPS handlers.
//
//nolint:gochecknoglobals // OPS metrics registry is process-wide by design.
var registry atomic.Pointer[prometheus.Registry]

// runtimeMetricsRegistered tracks the default runtime collector registration.
//
//nolint:gochecknoglobals // Runtime collector registration follows the process-wide OPS registry.
var runtimeMetricsRegistered atomic.Bool

// versionTpl is a pre-compiled template for displaying version information.
// The template formats:
// - Go runtime version
// - Application version and module details
// - Dependency information with optional replace directives
// - Build settings (flags, environment variables, etc.)
//
// Template structure:
//
//	Go Version: {{.GoVersion}}
//	App Version: {{.Main.Version}}
//	App Module: {{.Main.Path}}
//	App Path: {{.Path}}
//
//	Dependencies:
//	- Module: {path}
//	  Version: {version}
//	  Replace: {replace} (if applicable)
//
//	BuildSettings:
//	- {key} = {value}
//
// nolint:gochecknoglobals
var versionTpl = template.Must(template.New("version").Parse(`{{- if . -}}
Go Version: {{ .GoVersion }}
App Version: {{ .Main.Version }}
App Module: {{ .Main.Path }}
App Path: {{ .Path }}

Dependencies:
{{- range .Deps }}
- Module: {{ .Path }}
  Version: {{ .Version }}
{{- if .Sum }}
  Sum: {{ .Sum }}
{{- end }}
{{- if .Replace }}
  Replace: {{ .Replace.Path | printf "%q" }}{{- if .Replace.Version }} = {{ .Replace.Version}}{{- end }}
{{- end }}
{{ end }}
BuildSettings:
{{- range .Settings }}
- {{ .Key }} = {{ .Value | printf "%q" }}
{{- end }}
{{- else -}}
BuildInfo is empty
{{- end }}`))

// version is an HTTP handler that returns application version information.
// The handler supports multiple response formats:
//   - JSON format when "format=json" query parameter is provided
//   - Plain text format by default (using versionTpl template)
//
// Response characteristics:
//   - Content-Type: "application/json; charset=utf-8" for JSON format
//   - Content-Type: "text/plain; charset=utf-8" for plain text format
//   - HTTP 200 status code on success
//
// The response includes:
//   - Build information from the embedded build context
//   - Dependency tree with versions
//   - Build settings and configuration
//
// Usage: Typically mounted on routes like "/version" or "/debug/version"
// for health checks, deployment verification, and operational monitoring.
func version(w ResponseWriter, r *Request) {
	switch r.URL.Query().Get("format") {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(StatusOK)
		_ = json.NewEncoder(w).Encode(buildInfo)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(StatusOK)
		_ = versionTpl.Execute(w, buildInfo)
	}
}

// registerPprofHandlers wires standard pprof handlers and any named runtime
// profiles currently exposed by runtime/pprof.
func registerPprofHandlers(mux *ServeMux, base string) {
	mux.HandleFunc(base+"/", pprof.Index)
	mux.HandleFunc(base+"/cmdline", pprof.Cmdline)
	mux.HandleFunc(base+"/profile", pprof.Profile)
	mux.HandleFunc(base+"/symbol", pprof.Symbol)
	mux.HandleFunc(base+"/trace", pprof.Trace)

	for _, profile := range rprof.Profiles() {
		mux.Handle(base+"/"+profile.Name(), pprof.Handler(profile.Name()))
	}
}

// RegisterMetrics registers Prometheus collectors in the active OPS registry.
func RegisterMetrics(cs ...prometheus.Collector) error {
	r := getRegistry()

	for _, item := range cs {
		if err := r.Register(item); err != nil {
			return err
		}
	}

	return nil
}

// getRegistry returns the process-wide OPS registry, initializing it on first use.
func getRegistry() *prometheus.Registry {
	if r := registry.Load(); r != nil {
		return r
	}

	if next := prometheus.NewRegistry(); registry.CompareAndSwap(nil, next) {
		return next
	}

	return registry.Load()
}

// registerRuntimeMetrics installs the default Go runtime collector exactly once.
func registerRuntimeMetrics() error {
	if runtimeMetricsRegistered.Load() {
		return nil
	}

	err := RegisterMetrics(newOpsRuntimeCollector())
	if _, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
		runtimeMetricsRegistered.Store(true)

		return nil
	} else if err != nil {
		return err
	}

	runtimeMetricsRegistered.Store(true)

	return nil
}

// NewOPSServer creates an HTTP service exposing monitoring and debugging endpoints.
// It sets up the following handlers:
//   - Prometheus metrics endpoint
//   - Expvar variables endpoint
//   - `pprof` debugging endpoints (index, cmdline, profile, symbol, trace, and
//     any named runtime profiles such as goroutine, heap, or goroutineleak when available)
//
// Parameters:
//   - cfg: Configuration for the operations server
//   - log: Logger instance for server operations
//
// Returns a configured HTTP server as a service.Service interface and any error encountered during setup.
func NewOPSServer(cfg config.Ops, log *logger.Logger) (service.Service, error) {
	mux := NewServeMux()

	// OPS intentionally serves Prometheus/runtime diagnostics independently of
	// any OTEL metrics pipeline, so applications can choose one or both paths.
	if err := registerRuntimeMetrics(); err != nil {
		return nil, err
	}

	reg := getRegistry()
	mux.Handle(cfg.MetricsPath, promhttp.InstrumentMetricHandler(
		reg, promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
	))

	// prepare exp variables handlers
	mux.Handle(cfg.ExpVarsPath, expvar.Handler())

	// prepare pprof handlers
	registerPprofHandlers(mux, cfg.ProfilePath)

	// version handler
	if cfg.VersionEnabled {
		mux.HandleFunc(cfg.VersionPath, version)
	}

	return NewServer(
		cfg,
		log,
		ServiceName(defaultOPSServiceName),
		ServerOptions(func(srv *Server) {
			srv.Handler = mux
		}),
	)
}
