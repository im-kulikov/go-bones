package network

import (
	"expvar"
	"net/http"
	"net/http/pprof"
	"runtime/metrics"
	"slices"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/im-kulikov/go-bones/config"
	"github.com/im-kulikov/go-bones/logger"
	"github.com/im-kulikov/go-bones/service"
)

type opsCollector struct {
	desc *prometheus.Desc
}

const (
	defaultOPSServiceName = "ops"

	opsCollectorName        = "gm_runtime"
	opsCollectorDescription = "Raw golang runtime/metrics value"
)

func newOpsCollector() *opsCollector {
	return &opsCollector{
		desc: prometheus.NewDesc(opsCollectorName, opsCollectorDescription, []string{"name"}, nil),
	}
}

// Describe used to implement prometheus.Collector.
func (o *opsCollector) Describe(out chan<- *prometheus.Desc) {
	out <- o.desc
}

// Collect used to implement prometheus.Collector.
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

	return NewHTTPServer(
		cfg,
		log,
		mux,
		HTTPServiceName(defaultOPSServiceName),
	)
}
