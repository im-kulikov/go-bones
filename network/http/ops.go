package http

import (
	"encoding/json"
	"expvar"
	"net/http"
	"net/http/pprof"
	"runtime/debug"
	"runtime/metrics"
	"slices"
	"text/template"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

// opsCollector implements prometheus.Collector interface for collecting Go runtime metrics.
// It gathers metrics from the runtime/metrics package and exposes them in Prometheus format.
type opsCollector struct {
	desc *prometheus.Desc
}

const (
	defaultOPSServiceName = "ops"

	opsCollectorName        = "gm_runtime"
	opsCollectorDescription = "Raw golang runtime/metrics value"
)

// newOpsCollector creates and initializes a new opsCollector instance.
// It sets up a Prometheus metric descriptor with the name "gm_runtime" that will contain
// raw values from Go runtime metrics.
func newOpsCollector() *opsCollector {
	return &opsCollector{
		desc: prometheus.NewDesc(opsCollectorName, opsCollectorDescription, []string{"name"}, nil),
	}
}

// Describe implements prometheus.Collector interface.
// It sends the collector's metric descriptor to the provided channel.
// Prometheus calls this method when the collector is registered.
func (o *opsCollector) Describe(out chan<- *prometheus.Desc) {
	out <- o.desc
}

// Collect implements prometheus.Collector interface.
// It gathers all available runtime metrics that are either uint64 or float64,
// reads their current values, and sends them as Prometheus metrics through
// the provided channel. Each metric is labeled with its original name from
// the runtime/metrics package.
func (o *opsCollector) Collect(out chan<- prometheus.Metric) {
	desc := metrics.All()
	list := make([]metrics.Sample, 0, len(desc))
	kind := []metrics.ValueKind{
		metrics.KindUint64,
		metrics.KindFloat64,
	}

	for _, item := range desc {
		if !slices.Contains(kind, item.Kind) {
			continue
		}

		list = append(list, metrics.Sample{Name: item.Name})
	}

	metrics.Read(list)

	for _, sample := range list {
		var value float64
		if sample.Value.Kind() == metrics.KindUint64 {
			value = float64(sample.Value.Uint64())
		} else if sample.Value.Kind() == metrics.KindFloat64 {
			value = sample.Value.Float64()
		}

		out <- prometheus.MustNewConstMetric(o.desc, prometheus.GaugeValue, value, sample.Name)
	}
}

// buildInfo contains build information read from the embedded build info.
// Initialized at startup using debug.ReadBuildInfo().
//
// nolint:gochecknoglobals
var buildInfo, _ = debug.ReadBuildInfo()

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
func version(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("format") {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(buildInfo)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = versionTpl.Execute(w, buildInfo)
	}
}

// NewOPSServer creates a new operations server that provides monitoring and debugging endpoints.
// It sets up the following handlers:
//   - Prometheus metrics endpoint
//   - Expvar variables endpoint
//   - pprof debugging endpoints (index, cmdline, profile, symbol, and trace)
//
// Parameters:
//   - cfg: Configuration for the operations server
//   - log: Logger instance for server operations
//
// Returns a configured HTTP server as a service.Service interface and any error encountered during setup.
func NewOPSServer(cfg config.Ops, log *logger.Logger) (service.Service, error) {
	mux := http.NewServeMux()

	// prepare metrics handlers
	prometheus.MustRegister(newOpsCollector())
	mux.Handle(cfg.MetricsPath, promhttp.Handler())

	// prepare exp variables handlers
	mux.Handle(cfg.ExpVarsPath, expvar.Handler())

	// prepare pprof handler
	mux.HandleFunc(cfg.ProfilePath+"/", pprof.Index)
	mux.HandleFunc(cfg.ProfilePath+"/cmdline", pprof.Cmdline)
	mux.HandleFunc(cfg.ProfilePath+"/profile", pprof.Profile)
	mux.HandleFunc(cfg.ProfilePath+"/symbol", pprof.Symbol)
	mux.HandleFunc(cfg.ProfilePath+"/trace", pprof.Trace)

	// version handler
	if cfg.VersionEnabled {
		mux.HandleFunc(cfg.VersionPath, version)
	}

	return NewServer(
		cfg,
		log,
		ServiceName(defaultOPSServiceName),
		ServerOptions(func(srv *http.Server) {
			srv.Handler = mux
		}),
	)
}
