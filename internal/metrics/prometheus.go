package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal counts total requests per chain and method
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_requests_total",
			Help: "Total number of RPC requests",
		},
		[]string{"chain", "method"},
	)

	// RequestDuration measures request latency
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rpc_proxy_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"chain", "method", "status"},
	)

	// EndpointHealth tracks endpoint health status
	EndpointHealth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_endpoint_healthy",
			Help: "Endpoint health status (1 = healthy, 0 = unhealthy)",
		},
		[]string{"chain", "endpoint"},
	)

	// BlockHeight tracks the highest block per chain
	BlockHeight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_block_height",
			Help: "Highest known block height per chain",
		},
		[]string{"chain"},
	)

	// EndpointBlockHeight tracks block height per endpoint
	EndpointBlockHeight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_endpoint_block_height",
			Help: "Block height per endpoint",
		},
		[]string{"chain", "endpoint"},
	)

	// EndpointLatency tracks endpoint latency
	EndpointLatency = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_endpoint_latency_ms",
			Help: "Endpoint latency in milliseconds",
		},
		[]string{"chain", "endpoint"},
	)

	// CacheHits counts cache hits
	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"chain", "method"},
	)

	// CacheMisses counts cache misses
	CacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"chain", "method"},
	)

	// RateLimitRejects counts rate limit rejections
	RateLimitRejects = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_rate_limit_rejects_total",
			Help: "Total number of rate limit rejections",
		},
		[]string{"chain"},
	)

	// HealthyEndpoints tracks healthy endpoints per chain
	HealthyEndpoints = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_healthy_endpoints",
			Help: "Number of healthy endpoints per chain",
		},
		[]string{"chain"},
	)

	// TotalEndpoints tracks total endpoints per chain
	TotalEndpoints = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_total_endpoints",
			Help: "Total number of endpoints per chain",
		},
		[]string{"chain"},
	)

	// ErrorsTotal counts errors by type
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_errors_total",
			Help: "Total number of errors",
		},
		[]string{"chain", "type"},
	)
)

// RecordRequest records a request metric
func RecordRequest(chain, method string) {
	RequestsTotal.WithLabelValues(chain, method).Inc()
}

// RecordRequestDuration records request duration
func RecordRequestDuration(chain, method, status string, duration float64) {
	RequestDuration.WithLabelValues(chain, method, status).Observe(duration)
}

// RecordEndpointHealth records endpoint health
func RecordEndpointHealth(chain, endpoint string, healthy bool) {
	val := 0.0
	if healthy {
		val = 1.0
	}
	EndpointHealth.WithLabelValues(chain, endpoint).Set(val)
}

// RecordBlockHeight records block height
func RecordBlockHeight(chain string, height int64) {
	BlockHeight.WithLabelValues(chain).Set(float64(height))
}

// RecordEndpointBlockHeight records endpoint block height
func RecordEndpointBlockHeight(chain, endpoint string, height int64) {
	EndpointBlockHeight.WithLabelValues(chain, endpoint).Set(float64(height))
}

// RecordEndpointLatency records endpoint latency
func RecordEndpointLatency(chain, endpoint string, latencyMs float64) {
	EndpointLatency.WithLabelValues(chain, endpoint).Set(latencyMs)
}

// RecordCacheHit records a cache hit
func RecordCacheHit(chain, method string) {
	CacheHits.WithLabelValues(chain, method).Inc()
}

// RecordCacheMiss records a cache miss
func RecordCacheMiss(chain, method string) {
	CacheMisses.WithLabelValues(chain, method).Inc()
}

// RecordRateLimitReject records a rate limit rejection
func RecordRateLimitReject(chain string) {
	RateLimitRejects.WithLabelValues(chain).Inc()
}

// RecordHealthyEndpoints records healthy endpoint count
func RecordHealthyEndpoints(chain string, count int) {
	HealthyEndpoints.WithLabelValues(chain).Set(float64(count))
}

// RecordTotalEndpoints records total endpoint count
func RecordTotalEndpoints(chain string, count int) {
	TotalEndpoints.WithLabelValues(chain).Set(float64(count))
}

// RecordError records an error
func RecordError(chain, errType string) {
	ErrorsTotal.WithLabelValues(chain, errType).Inc()
}
