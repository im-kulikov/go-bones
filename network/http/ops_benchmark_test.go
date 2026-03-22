package http

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func BenchmarkOpsRuntimeCollectorGather(b *testing.B) {
	register := prometheus.NewRegistry()
	register.MustRegister(newOpsRuntimeCollector())

	for b.Loop() {
		families, err := register.Gather()
		if err != nil {
			b.Fatal(err)
		}

		if len(families) == 0 {
			b.Fatal("expected runtime metrics")
		}
	}
}
