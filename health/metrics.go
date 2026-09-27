package health

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const metricsNamespace = "go_bones_health"

// metrics holds counters and histograms accumulated by the Monitor. Gauges are
// computed from Snapshot at scrape time.
type metrics struct {
	duration    *prometheus.HistogramVec
	runs        *prometheus.CounterVec
	transitions *prometheus.CounterVec
	skips       *prometheus.CounterVec
	drops       prometheus.Counter

	live, ready, draining *prometheus.Desc
	up, stale, success    *prometheus.Desc
}

func newMetrics() *metrics {
	return &metrics{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: metricsNamespace, Name: "check_duration_seconds",
			Help:    "Duration of health check runs.",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"check"}),
		runs: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace, Name: "check_runs_total",
			Help: "Health check runs by result (success, error, timeout, panic).",
		}, []string{"check", "result"}),
		transitions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace, Name: "check_transitions_total",
			Help: "Health check status transitions by target status.",
		}, []string{"check", "to"}),
		skips: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: metricsNamespace, Name: "check_skipped_total",
			Help: "Health check runs skipped (in_flight) or delayed (rate_limited).",
		}, []string{"check", "reason"}),
		drops: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: metricsNamespace, Name: "events_dropped_total",
			Help: "Events dropped because a subscriber was too slow.",
		}),
		live:     desc("live", "1 when the service is live.", nil),
		ready:    desc("ready", "1 when the service is ready.", nil),
		draining: desc("draining", "1 when the service is draining.", nil),
		up: desc(
			"check_up",
			"1 when the check is passing and fresh.",
			[]string{"check", "impact"},
		),
		stale: desc("check_stale", "1 when the check result is stale.", []string{"check"}),
		success: desc("check_last_success_timestamp_seconds",
			"Unix time of the last successful check run.", []string{"check"}),
	}
}

func desc(name, help string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(prometheus.BuildFQName(metricsNamespace, "", name), help, labels, nil)
}

func (x *metrics) observe(check, result string, took time.Duration) {
	x.runs.WithLabelValues(check, result).Inc()

	if took >= 0 {
		x.duration.WithLabelValues(check).Observe(took.Seconds())
	}
}

func (x *metrics) transition(check string, to Status) {
	x.transitions.WithLabelValues(check, string(to)).Inc()
}

func (x *metrics) skipped(check, reason string) {
	x.skips.WithLabelValues(check, reason).Inc()
}

func (x *metrics) dropped() { x.drops.Inc() }

// Describe implements prometheus.Collector.
func (m *Monitor) Describe(ch chan<- *prometheus.Desc) {
	x := m.metrics
	for _, d := range []*prometheus.Desc{x.live, x.ready, x.draining, x.up, x.stale, x.success} {
		ch <- d
	}

	x.duration.Describe(ch)
	x.runs.Describe(ch)
	x.transitions.Describe(ch)
	x.skips.Describe(ch)
	x.drops.Describe(ch)
}

// Collect implements prometheus.Collector.
func (m *Monitor) Collect(ch chan<- prometheus.Metric) {
	x := m.metrics
	snap := m.Snapshot()

	ch <- prometheus.MustNewConstMetric(x.live, prometheus.GaugeValue, b2f(snap.Live))
	ch <- prometheus.MustNewConstMetric(x.ready, prometheus.GaugeValue, b2f(snap.Ready))
	ch <- prometheus.MustNewConstMetric(x.draining, prometheus.GaugeValue, b2f(snap.Draining))

	for _, name := range snap.Names() {
		res := snap.Checks[name]
		ch <- prometheus.MustNewConstMetric(x.up, prometheus.GaugeValue, b2f(res.Up()), name, res.Impact.String())
		ch <- prometheus.MustNewConstMetric(x.stale, prometheus.GaugeValue, b2f(res.Stale), name)

		if !res.LastSuccess.IsZero() {
			ch <- prometheus.MustNewConstMetric(x.success, prometheus.GaugeValue,
				float64(res.LastSuccess.UnixNano())/float64(time.Second), name)
		}
	}

	x.duration.Collect(ch)
	x.runs.Collect(ch)
	x.transitions.Collect(ch)
	x.skips.Collect(ch)
	x.drops.Collect(ch)
}

func b2f(v bool) float64 {
	if v {
		return 1
	}

	return 0
}
