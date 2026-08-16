package metrics

import (
	"testing"
)

func TestMetricsCollector(t *testing.T) {
	m := GetCollector()

	t.Run("Record Requests and Calculate Quantiles", func(t *testing.T) {
		for i := 1; i <= 100; i++ {
			m.RecordRequestWithMethod("GET", 200, float64(i))
		}
		m.RecordRequestWithMethod("POST", 500, 150)

		snap := m.Snapshot()
		if snap.TotalRequests < 101 {
			t.Errorf("expected total requests >= 101, got %d", snap.TotalRequests)
		}
		if snap.TotalErrors < 1 {
			t.Errorf("expected total errors >= 1, got %d", snap.TotalErrors)
		}
		if snap.LatencyP99Ms < 99 {
			t.Errorf("expected P99 latency >= 99, got %f", snap.LatencyP99Ms)
		}
	})

	t.Run("Domain Counters", func(t *testing.T) {
		m.IncTransfersCreated()
		m.IncTransfersCreated()
		m.IncDeclinedInsufficientFunds()
		m.IncIdempotentReplays()

		snap := m.Snapshot()
		if snap.TransfersCreated < 2 {
			t.Errorf("expected >= 2 transfers created, got %d", snap.TransfersCreated)
		}
		if snap.DeclinedInsufficientFunds < 1 {
			t.Errorf("expected >= 1 declined transfer, got %d", snap.DeclinedInsufficientFunds)
		}
		if snap.IdempotentReplays < 1 {
			t.Errorf("expected >= 1 idempotent replay, got %d", snap.IdempotentReplays)
		}
	})

	t.Run("Prometheus Registry Verification", func(t *testing.T) {
		if m.Registry() == nil {
			t.Errorf("expected non-nil Prometheus Registry")
		}
	})
}
