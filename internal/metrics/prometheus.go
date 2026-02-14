package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_requests_total",
			Help: "Total number of RPC requests",
		},
		[]string{"chain", "method"},
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rpc_proxy_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"chain", "method", "status"},
	)

	EndpointHealth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_endpoint_healthy",
			Help: "Endpoint health status (1 = healthy, 0 = unhealthy)",
		},
		[]string{"chain", "endpoint"},
	)

	BlockHeight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_block_height",
			Help: "Highest known block height per chain",
		},
		[]string{"chain"},
	)

	EndpointBlockHeight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_endpoint_block_height",
			Help: "Block height per endpoint",
		},
		[]string{"chain", "endpoint"},
	)

	EndpointLatency = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_endpoint_latency_ms",
			Help: "Endpoint latency in milliseconds",
		},
		[]string{"chain", "endpoint"},
	)

	CacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_cache_hits_total",
			Help: "Total number of cache hits",
		},
		[]string{"chain", "method"},
	)

	CacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_cache_misses_total",
			Help: "Total number of cache misses",
		},
		[]string{"chain", "method"},
	)

	RateLimitRejects = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_rate_limit_rejects_total",
			Help: "Total number of rate limit rejections",
		},
		[]string{"chain"},
	)

	HealthyEndpoints = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_healthy_endpoints",
			Help: "Number of healthy endpoints per chain",
		},
		[]string{"chain"},
	)

	TotalEndpoints = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rpc_proxy_total_endpoints",
			Help: "Total number of endpoints per chain",
		},
		[]string{"chain"},
	)

	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_proxy_errors_total",
			Help: "Total number of errors",
		},
		[]string{"chain", "type"},
	)
)

func RecordRequest(chain, method string) {
	RequestsTotal.WithLabelValues(chain, method).Inc()
}

func RecordRequestDuration(chain, method, status string, duration float64) {
	RequestDuration.WithLabelValues(chain, method, status).Observe(duration)
}

func RecordEndpointHealth(chain, endpoint string, healthy bool) {
	val := 0.0
	if healthy {
		val = 1.0
	}
	EndpointHealth.WithLabelValues(chain, endpoint).Set(val)
}

func RecordBlockHeight(chain string, height int64) {
	BlockHeight.WithLabelValues(chain).Set(float64(height))
}

func RecordEndpointBlockHeight(chain, endpoint string, height int64) {
	EndpointBlockHeight.WithLabelValues(chain, endpoint).Set(float64(height))
}

func RecordEndpointLatency(chain, endpoint string, latencyMs float64) {
	EndpointLatency.WithLabelValues(chain, endpoint).Set(latencyMs)
}

func RecordCacheHit(chain, method string) {
	CacheHits.WithLabelValues(chain, method).Inc()
}

func RecordCacheMiss(chain, method string) {
	CacheMisses.WithLabelValues(chain, method).Inc()
}

func RecordRateLimitReject(chain string) {
	RateLimitRejects.WithLabelValues(chain).Inc()
}

func RecordHealthyEndpoints(chain string, count int) {
	HealthyEndpoints.WithLabelValues(chain).Set(float64(count))
}

func RecordTotalEndpoints(chain string, count int) {
	TotalEndpoints.WithLabelValues(chain).Set(float64(count))
}

func RecordError(chain, errType string) {
	ErrorsTotal.WithLabelValues(chain, errType).Inc()
}
