package http

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/im-kulikov/go-bones/health"
	"github.com/im-kulikov/go-bones/logger"
)

// OPSOption configures NewOPSServer.
type OPSOption func(*opsOptions)

type opsOptions struct {
	health health.Reader
}

// WithHealth serves /livez, /readyz and /healthz from the given reader (usually
// a *health.Monitor). When the reader is also a prometheus.Collector and
// metrics are enabled, it is registered in the OPS registry.
//
// Without WithHealth (and with Ops.HealthEnabled) /livez and /readyz always
// answer 200 while the OPS server is up, so services without checks still get
// Kubernetes probes out of the box.
func WithHealth(r health.Reader) OPSOption {
	return func(o *opsOptions) { o.health = r }
}

// probe selects what a probe endpoint evaluates.
type probe uint8

const (
	probeLive probe = iota
	probeReady
)

const (
	contentTypeText = "text/plain; charset=utf-8"
	contentTypeJSON = "application/json; charset=utf-8"
)

// staticReader is used when no health reader is configured: always live and ready.
type staticReader struct{}

func (staticReader) Snapshot() health.Snapshot {
	return health.Snapshot{
		Running: true, Live: true, Ready: true,
		Overall: health.OverallOK, Checks: map[string]health.Result{},
	}
}

func (staticReader) Subscribe(func(health.Event)) func() { return func() {} }

// healthHandlers only read Snapshot, so they never block on a checker.
type healthHandlers struct {
	reader health.Reader
	log    *logger.Logger
}

func registerHealthHandlers(
	mux *ServeMux,
	cfg opsHealthPaths,
	reader health.Reader,
	log *logger.Logger,
) {
	if reader == nil {
		reader = staticReader{}
	}

	h := &healthHandlers{reader: reader, log: log}

	mux.HandleFunc(cfg.live, h.guard(h.probe(probeLive, "livez")))
	mux.HandleFunc(strings.TrimSuffix(cfg.live, "/")+"/", h.guard(h.single(probeLive, cfg.live)))
	mux.HandleFunc(cfg.ready, h.guard(h.probe(probeReady, "readyz")))
	mux.HandleFunc(strings.TrimSuffix(cfg.ready, "/")+"/", h.guard(h.single(probeReady, cfg.ready)))
	mux.HandleFunc(cfg.health, h.guard(h.report))
}

type opsHealthPaths struct {
	live, ready, health string
}

// withDefaults fills empty paths, so a config.Ops built in code still works.
func (p opsHealthPaths) withDefaults() opsHealthPaths {
	if p.live == "" {
		p.live = "/livez"
	}

	if p.ready == "" {
		p.ready = "/readyz"
	}

	if p.health == "" {
		p.health = "/healthz"
	}

	return p
}

// guard allows only GET and HEAD and disables caching.
func (h *healthHandlers) guard(next HandlerFunc) HandlerFunc {
	return func(w ResponseWriter, r *Request) {
		w.Header().Set("Cache-Control", "no-store")

		if r.Method != MethodGet && r.Method != MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			h.writeText(w, r, StatusMethodNotAllowed, "method not allowed\n")

			return
		}

		next(w, r)
	}
}

func (h *healthHandlers) writeText(w ResponseWriter, r *Request, code int, body string) {
	w.Header().Set("Content-Type", contentTypeText)
	w.WriteHeader(code)

	if _, err := w.Write([]byte(body)); err != nil {
		h.log.DebugContext(r.Context(), "could not write health response", logger.Err(err))
	}
}

func (h *healthHandlers) writeJSON(w ResponseWriter, r *Request, code int, v any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.log.DebugContext(r.Context(), "could not write health response", logger.Err(err))
	}
}

// relevant reports whether a check takes part in the given probe.
func relevant(kind probe, res health.Result) bool {
	if res.Impact == health.Liveness {
		return true
	}

	return kind == probeReady && res.Impact == health.Readiness
}

// checkOK mirrors health aggregation: an unknown liveness check is ok,
// an unknown readiness check is not.
func checkOK(res health.Result) bool {
	return res.Up() || (res.Impact == health.Liveness && res.Status == health.StatusUnknown)
}

func statusCode(ok bool) int {
	if ok {
		return StatusOK
	}

	return StatusServiceUnavailable
}

// probe serves /livez and /readyz with apiserver-like ?verbose, ?exclude and ?format=json.
func (h *healthHandlers) probe(kind probe, name string) HandlerFunc {
	return func(w ResponseWriter, r *Request) {
		query := r.URL.Query()
		excluded := query["exclude"]
		snap := h.reader.Snapshot().Without(excluded...)

		ok := snap.Live
		if kind == probeReady {
			ok = snap.Ready
		}

		if query.Get("format") == "json" {
			rep := newReport(snap, func(res health.Result) bool { return relevant(kind, res) })
			rep.Status = string(health.OverallOK)

			if !ok {
				rep.Status = string(health.OverallFailing)
			}

			h.writeJSON(w, r, statusCode(ok), rep)

			return
		}

		lines, failed := probeLines(kind, snap, excluded)
		verdict := name + " check passed\n"

		if !ok {
			verdict = name + " check failed\n"
		}

		var body string

		switch {
		case query.Has("verbose"):
			body = strings.Join(lines, "\n") + "\n" + verdict
		case ok:
			body = "ok\n"
		default:
			body = strings.Join(failed, "\n") + "\n" + verdict
		}

		h.writeText(w, r, statusCode(ok), body)
	}
}

// probeLines returns all verbose lines and the failed ones only.
func probeLines(kind probe, snap health.Snapshot, excluded []string) (lines, failed []string) {
	add := func(line string, isOK bool) {
		lines = append(lines, line)
		if !isOK {
			failed = append(failed, line)
		}
	}

	if !snap.Running {
		add("[-]monitor failed: not running", false)
	}

	if kind == probeReady && snap.Draining {
		add("[-]draining failed: shutting down", false)
	}

	for _, name := range snap.Names() {
		res := snap.Checks[name]
		if !relevant(kind, res) {
			continue
		}

		if checkOK(res) {
			add("[+]"+name+" ok", true)
		} else {
			add("[-]"+name+" failed: "+resultMessage(res), false)
		}
	}

	for _, name := range excluded {
		lines = append(lines, "[+]"+name+" excluded: ok")
	}

	return lines, failed
}

func resultMessage(res health.Result) string {
	switch {
	case res.Stale:
		return "stale"
	case res.Status == health.StatusUnknown:
		return "not checked yet"
	case res.Err != nil:
		return health.PublicMessage(res.Err)
	default:
		return "error"
	}
}

// single serves /livez/<check> and /readyz/<check>.
func (h *healthHandlers) single(kind probe, base string) HandlerFunc {
	prefix := strings.TrimSuffix(base, "/") + "/"

	return func(w ResponseWriter, r *Request) {
		name := strings.TrimPrefix(r.URL.Path, prefix)

		res, ok := h.reader.Snapshot().Checks[name]
		if !ok || name == "" || !relevant(kind, res) {
			h.writeText(w, r, StatusNotFound, "check not found\n")

			return
		}

		if checkOK(res) {
			h.writeText(w, r, StatusOK, "ok\n")

			return
		}

		h.writeText(w, r, StatusServiceUnavailable,
			"[-]"+name+" failed: "+resultMessage(res)+"\n")
	}
}

// report serves /healthz: a full JSON report, degraded is still 200.
func (h *healthHandlers) report(w ResponseWriter, r *Request) {
	snap := h.reader.Snapshot().Without(r.URL.Query()["exclude"]...)

	h.writeJSON(w, r, statusCode(snap.Overall != health.OverallFailing),
		newReport(snap, func(health.Result) bool { return true }))
}

type checkReport struct {
	Impact              string     `json:"impact"`
	Status              string     `json:"status"`
	Stale               bool       `json:"stale,omitempty"`
	CheckedAt           *time.Time `json:"checked_at,omitempty"`
	DurationMS          int64      `json:"duration_ms"`
	LastSuccess         *time.Time `json:"last_success,omitempty"`
	Error               string     `json:"error,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures,omitempty"`
}

type healthReport struct {
	Status   string                 `json:"status"`
	Live     bool                   `json:"live"`
	Ready    bool                   `json:"ready"`
	Draining bool                   `json:"draining"`
	Checks   map[string]checkReport `json:"checks"`
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	t = t.UTC()

	return &t
}

// newReport converts a snapshot into the JSON report. Errors are replaced by
// their public classification, raw texts never leave the process.
func newReport(snap health.Snapshot, include func(health.Result) bool) healthReport {
	out := healthReport{
		Status:   string(snap.Overall),
		Live:     snap.Live,
		Ready:    snap.Ready,
		Draining: snap.Draining,
		Checks:   make(map[string]checkReport, len(snap.Checks)),
	}

	for name, res := range snap.Checks {
		if !include(res) {
			continue
		}

		item := checkReport{
			Impact:              res.Impact.String(),
			Status:              string(res.Status),
			Stale:               res.Stale,
			CheckedAt:           timePtr(res.CheckedAt),
			DurationMS:          res.Duration.Milliseconds(),
			LastSuccess:         timePtr(res.LastSuccess),
			ConsecutiveFailures: res.ConsecutiveFailures,
		}

		if !res.Up() && res.Status != health.StatusUnknown {
			item.Error = resultMessage(res)
		}

		out.Checks[name] = item
	}

	return out
}
