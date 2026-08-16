package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	mu sync.RWMutex

	// Prometheus Registry & Metrics
	registry *prometheus.Registry

	httpRequestsTotal       *prometheus.CounterVec
	httpRequestDurationSecs *prometheus.HistogramVec

	transfersCreatedTotal                   prometheus.Counter
	transfersDeclinedInsufficientFundsTotal prometheus.Counter
	idempotentReplaysTotal                  prometheus.Counter

	// Snapshot tracking for RPS, P99, error rates & JSON dashboard endpoint
	totalRequests uint64
	totalErrors   uint64
	status2xx     uint64
	status4xx     uint64
	status5xx     uint64

	latencies         []float64
	requestTimestamps []time.Time

	transfersCreated          uint64
	declinedInsufficientFunds uint64
	idempotentReplays         uint64
}

var (
	globalMetrics *Metrics
	once          sync.Once
)

func GetCollector() *Metrics {
	once.Do(func() {
		reg := prometheus.NewRegistry()

		// System Go & Process metrics
		reg.MustRegister(collectors.NewGoCollector())
		reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

		httpReqTotal := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests processed by status code and group.",
			},
			[]string{"status_code", "status_group"},
		)

		httpReqDuration := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request latency histogram in seconds.",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms up to ~16s
			},
			[]string{"method", "status_group"},
		)

		transfersCreated := prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "transfers_created_total",
				Help: "Total count of transfers created/initiated.",
			},
		)

		declinedFunds := prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "transfers_declined_insufficient_funds_total",
				Help: "Total count of transfers declined due to insufficient funds.",
			},
		)

		idempotentReplays := prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "idempotent_replays_total",
				Help: "Total count of idempotent replay hits.",
			},
		)

		reg.MustRegister(httpReqTotal)
		reg.MustRegister(httpReqDuration)
		reg.MustRegister(transfersCreated)
		reg.MustRegister(declinedFunds)
		reg.MustRegister(idempotentReplays)

		globalMetrics = &Metrics{
			registry:                                reg,
			httpRequestsTotal:                       httpReqTotal,
			httpRequestDurationSecs:                 httpReqDuration,
			transfersCreatedTotal:                   transfersCreated,
			transfersDeclinedInsufficientFundsTotal: declinedFunds,
			idempotentReplaysTotal:                  idempotentReplays,
			latencies:                               make([]float64, 0, 1000),
			requestTimestamps:                       make([]time.Time, 0, 1000),
		}
	})
	return globalMetrics
}

func (m *Metrics) PrometheusHandler() promhttp.HandlerOpts {
	return promhttp.HandlerOpts{}
}

func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

func (m *Metrics) RecordRequestWithMethod(method string, statusCode int, latencyMs float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.totalRequests++
	m.requestTimestamps = append(m.requestTimestamps, now)
	m.latencies = append(m.latencies, latencyMs)

	statusGroup := "2xx"
	statusStr := fmtStatusCode(statusCode)

	if statusCode >= 500 {
		statusGroup = "5xx"
		m.totalErrors++
		m.status5xx++
	} else if statusCode >= 400 {
		statusGroup = "4xx"
		m.totalErrors++
		m.status4xx++
	} else if statusCode >= 300 {
		statusGroup = "3xx"
	} else {
		m.status2xx++
	}

	// Update Prometheus Counters and Histograms
	m.httpRequestsTotal.WithLabelValues(statusStr, statusGroup).Inc()
	m.httpRequestDurationSecs.WithLabelValues(method, statusGroup).Observe(latencyMs / 1000.0)

	// Keep sliding windows manageable
	if len(m.latencies) > 2000 {
		m.latencies = m.latencies[len(m.latencies)-1000:]
	}

	cutoff := now.Add(-60 * time.Second)
	idx := 0
	for idx < len(m.requestTimestamps) && m.requestTimestamps[idx].Before(cutoff) {
		idx++
	}
	if idx > 0 {
		m.requestTimestamps = m.requestTimestamps[idx:]
	}
}

func (m *Metrics) RecordRequest(statusCode int, latencyMs float64) {
	m.RecordRequestWithMethod("UNKNOWN", statusCode, latencyMs)
}

func (m *Metrics) IncTransfersCreated() {
	atomic.AddUint64(&m.transfersCreated, 1)
	m.transfersCreatedTotal.Inc()
}

func (m *Metrics) IncDeclinedInsufficientFunds() {
	atomic.AddUint64(&m.declinedInsufficientFunds, 1)
	m.transfersDeclinedInsufficientFundsTotal.Inc()
}

func (m *Metrics) IncIdempotentReplays() {
	atomic.AddUint64(&m.idempotentReplays, 1)
	m.idempotentReplaysTotal.Inc()
}

type MetricsSnapshot struct {
	TotalRequests             uint64  `json:"total_requests"`
	TotalErrors               uint64  `json:"total_errors"`
	Status2xxCount            uint64  `json:"status_2xx_count"`
	Status4xxCount            uint64  `json:"status_4xx_count"`
	Status5xxCount            uint64  `json:"status_5xx_count"`
	RequestRateRPS            float64 `json:"request_rate_rps"`
	LatencyP50Ms              float64 `json:"latency_p50_ms"`
	LatencyP90Ms              float64 `json:"latency_p90_ms"`
	LatencyP99Ms              float64 `json:"latency_p99_ms"`
	ErrorRatePercent          float64 `json:"error_rate_percent"`
	ErrorRate4xxPercent       float64 `json:"error_rate_4xx_percent"`
	ErrorRate5xxPercent       float64 `json:"error_rate_5xx_percent"`
	TransfersCreated          uint64  `json:"transfers_created"`
	DeclinedInsufficientFunds uint64  `json:"declined_insufficient_funds"`
	IdempotentReplays         uint64  `json:"idempotent_replays"`
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	cutoff := now.Add(-60 * time.Second)
	recentReqCount := 0
	for _, ts := range m.requestTimestamps {
		if !ts.Before(cutoff) {
			recentReqCount++
		}
	}
	rps := float64(recentReqCount) / 60.0

	var errRate, errRate4xx, errRate5xx float64
	if m.totalRequests > 0 {
		errRate = (float64(m.totalErrors) / float64(m.totalRequests)) * 100.0
		errRate4xx = (float64(m.status4xx) / float64(m.totalRequests)) * 100.0
		errRate5xx = (float64(m.status5xx) / float64(m.totalRequests)) * 100.0
	}

	p50, p90, p99 := calculateQuantiles(m.latencies)

	return MetricsSnapshot{
		TotalRequests:             m.totalRequests,
		TotalErrors:               m.totalErrors,
		Status2xxCount:            m.status2xx,
		Status4xxCount:            m.status4xx,
		Status5xxCount:            m.status5xx,
		RequestRateRPS:            rps,
		LatencyP50Ms:              p50,
		LatencyP90Ms:              p90,
		LatencyP99Ms:              p99,
		ErrorRatePercent:          errRate,
		ErrorRate4xxPercent:       errRate4xx,
		ErrorRate5xxPercent:       errRate5xx,
		TransfersCreated:          atomic.LoadUint64(&m.transfersCreated),
		DeclinedInsufficientFunds: atomic.LoadUint64(&m.declinedInsufficientFunds),
		IdempotentReplays:         atomic.LoadUint64(&m.idempotentReplays),
	}
}

func calculateQuantiles(samples []float64) (p50, p90, p99 float64) {
	if len(samples) == 0 {
		return 0, 0, 0
	}
	sorted := make([]float64, len(samples))
	copy(sorted, samples)
	sort.Float64s(sorted)

	getQuantile := func(q float64) float64 {
		idx := int(q * float64(len(sorted)))
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}

	return getQuantile(0.50), getQuantile(0.90), getQuantile(0.99)
}

func fmtStatusCode(code int) string {
	switch code {
	case 200:
		return "200"
	case 201:
		return "201"
	case 400:
		return "400"
	case 401:
		return "401"
	case 402:
		return "402"
	case 403:
		return "403"
	case 404:
		return "404"
	case 409:
		return "409"
	case 422:
		return "422"
	case 500:
		return "500"
	default:
		return "other"
	}
}
