package health

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/im-kulikov/go-bones/config"
)

func TestStrings(t *testing.T) {
	require.Equal(t, "readiness", Readiness.String())
	require.Equal(t, "informational", Informational.String())
	require.Equal(t, "liveness", Liveness.String())
	require.Equal(t, "unknown", Impact(99).String())

	text, err := Liveness.MarshalText()
	require.NoError(t, err)
	require.Equal(t, "liveness", string(text))

	for kind, want := range map[EventKind]string{
		EventCheck: "check", EventReady: "ready", EventNotReady: "not_ready",
		EventDraining: "draining", EventLive: "live", EventNotLive: "not_live", EventKind(99): "unknown",
	} {
		require.Equal(t, want, kind.String())
	}
}

func TestPublicMessage(t *testing.T) {
	cases := map[string]error{
		"":                 nil,
		"timeout":          fmt.Errorf("wrap: %w", context.DeadlineExceeded),
		"panic":            ErrPanic,
		"stale":            ErrStale,
		"canceled":         context.Canceled,
		"error":            errDown,
		"database is down": fmt.Errorf("outer: %w", PublicError("database is down", errDown)),
	}

	for want, err := range cases {
		require.Equal(t, want, PublicMessage(err))
	}

	err := PublicError("database is down", errDown)
	require.ErrorIs(t, err, errDown)
	require.Contains(t, err.Error(), "secret", "the full text is kept for logs")
	require.Equal(t, "msg", PublicError("msg", nil).Error())
}

func TestClassify(t *testing.T) {
	kind, err := classify(context.DeadlineExceeded, time.Second)
	require.ErrorIs(t, err, ErrTimeout)
	require.Equal(t, resultTimeout, kind)

	kind, err = classify(ErrTimeout, time.Second)
	require.ErrorIs(t, err, ErrTimeout)
	require.Equal(t, resultTimeout, kind)
}

func TestSnapshotWithout(t *testing.T) {
	checks := map[string]Result{
		"db":     {Name: "db", Impact: Readiness, Status: StatusFailing},
		"cache":  {Name: "cache", Impact: Informational, Status: StatusFailing},
		"worker": {Name: "worker", Impact: Liveness, Status: StatusFailing},
	}

	snap := aggregate(true, false, checks)
	require.False(t, snap.Live)
	require.False(t, snap.Ready)
	require.Equal(t, OverallFailing, snap.Overall)
	require.Equal(t, []string{"cache", "db", "worker"}, snap.Names())
	require.Equal(t, snap, snap.Without())

	out := snap.Without("db", "worker")
	require.True(t, out.Live)
	require.True(t, out.Ready)
	require.Equal(t, OverallDegraded, out.Overall)
	require.Len(t, out.Checks, 1)
	require.Len(t, snap.Checks, 3, "original snapshot is not modified")

	drained := aggregate(true, true, checks).Without("db", "worker", "cache")
	require.False(t, drained.Ready, "exclude never overrides draining")
}

// gather returns all metric values keyed by name{label="value",...}.
func gather(t *testing.T, m *Monitor) map[string]float64 {
	t.Helper()

	reg := prometheus.NewPedanticRegistry()
	require.NoError(t, reg.Register(m))

	families, err := reg.Gather()
	require.NoError(t, err)

	out := make(map[string]float64)

	for _, family := range families {
		for _, metric := range family.GetMetric() {
			labels := make([]string, 0, len(metric.GetLabel()))
			for _, pair := range metric.GetLabel() {
				labels = append(labels, pair.GetName()+"="+strconv.Quote(pair.GetValue()))
			}

			key := family.GetName()
			if len(labels) > 0 {
				key += "{" + strings.Join(labels, ",") + "}"
			}

			out[key] = metric.GetCounter().GetValue() + metric.GetGauge().GetValue() +
				float64(metric.GetHistogram().GetSampleCount())
		}
	}

	return out
}

func testCounterValue(t *testing.T, m *Monitor, name string) float64 {
	t.Helper()

	return gather(t, m)[name]
}

func TestCollector(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := newTestMonitor(t, config.Health{StaleAfter: time.Minute})
		c := new(counter)
		require.NoError(t, m.Register("db", c))
		require.NoError(t, m.Register("slow", CheckerFunc(func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		}), WithImpact(Informational)))

		stop := start(t, m)
		defer stop()

		time.Sleep(2 * time.Second)
		synctest.Wait()
		m.Trigger("db")
		synctest.Wait()
		m.Trigger("db")
		synctest.Wait()
		time.Sleep(time.Second)
		synctest.Wait()

		got := gather(t, m)
		for key, want := range map[string]float64{
			`go_bones_health_check_up{check="db",impact="readiness"}`:       1,
			`go_bones_health_check_up{check="slow",impact="informational"}`: 0,
			`go_bones_health_live`:     1,
			`go_bones_health_ready`:    1,
			`go_bones_health_draining`: 0,
			`go_bones_health_check_runs_total{check="db",result="success"}`:         3,
			`go_bones_health_check_runs_total{check="slow",result="timeout"}`:       1,
			`go_bones_health_check_skipped_total{check="db",reason="rate_limited"}`: 1,
			`go_bones_health_check_stale{check="db"}`:                               0,
			`go_bones_health_check_duration_seconds{check="db"}`:                    3,
			`go_bones_health_check_transitions_total{check="db",to="passing"}`:      1,
			`go_bones_health_check_transitions_total{check="slow",to="failing"}`:    1,
			`go_bones_health_events_dropped_total`:                                  0,
		} {
			v, ok := got[key]
			require.True(t, ok, key)
			require.Equal(t, want, v, key) //nolint:testifylint // exact counters
		}

		require.Contains(t, got, `go_bones_health_check_last_success_timestamp_seconds{check="db"}`)
		require.NotContains(
			t,
			got,
			`go_bones_health_check_last_success_timestamp_seconds{check="slow"}`,
		)
	})
}
