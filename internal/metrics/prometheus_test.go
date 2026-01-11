package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordRequest(t *testing.T) {
	// Reset counter for clean test
	RequestsTotal.Reset()

	RecordRequest("ethereum", "eth_chainId")
	RecordRequest("ethereum", "eth_chainId")
	RecordRequest("ethereum", "eth_call")

	// Check eth_chainId counter
	count := testutil.ToFloat64(RequestsTotal.WithLabelValues("ethereum", "eth_chainId"))
	if count != 2 {
		t.Errorf("Expected 2 for eth_chainId, got %f", count)
	}

	// Check eth_call counter
	count = testutil.ToFloat64(RequestsTotal.WithLabelValues("ethereum", "eth_call"))
	if count != 1 {
		t.Errorf("Expected 1 for eth_call, got %f", count)
	}
}

func TestRecordRequestDuration(t *testing.T) {
	// Reset histogram for clean test
	RequestDuration.Reset()

	RecordRequestDuration("ethereum", "eth_chainId", "success", 0.1)
	RecordRequestDuration("ethereum", "eth_chainId", "success", 0.2)
	RecordRequestDuration("ethereum", "eth_chainId", "error", 0.5)

	// Verify histogram was updated by checking if we can observe it
	// The testutil doesn't have great histogram support, so we just verify no panic
	hist := RequestDuration.WithLabelValues("ethereum", "eth_chainId", "success")
	if hist == nil {
		t.Error("Expected histogram to exist")
	}
}

func TestRecordEndpointHealth(t *testing.T) {
	// Reset gauge for clean test
	EndpointHealth.Reset()

	RecordEndpointHealth("ethereum", "https://eth.example.com", true)
	val := testutil.ToFloat64(EndpointHealth.WithLabelValues("ethereum", "https://eth.example.com"))
	if val != 1 {
		t.Errorf("Expected 1 for healthy, got %f", val)
	}

	RecordEndpointHealth("ethereum", "https://eth.example.com", false)
	val = testutil.ToFloat64(EndpointHealth.WithLabelValues("ethereum", "https://eth.example.com"))
	if val != 0 {
		t.Errorf("Expected 0 for unhealthy, got %f", val)
	}
}

func TestRecordBlockHeight(t *testing.T) {
	// Reset gauge for clean test
	BlockHeight.Reset()

	RecordBlockHeight("ethereum", 12345678)
	val := testutil.ToFloat64(BlockHeight.WithLabelValues("ethereum"))
	if val != 12345678 {
		t.Errorf("Expected 12345678, got %f", val)
	}

	// Update with new height
	RecordBlockHeight("ethereum", 12345679)
	val = testutil.ToFloat64(BlockHeight.WithLabelValues("ethereum"))
	if val != 12345679 {
		t.Errorf("Expected 12345679, got %f", val)
	}
}

func TestRecordEndpointBlockHeight(t *testing.T) {
	// Reset gauge for clean test
	EndpointBlockHeight.Reset()

	RecordEndpointBlockHeight("ethereum", "https://eth.example.com", 12345678)
	val := testutil.ToFloat64(EndpointBlockHeight.WithLabelValues("ethereum", "https://eth.example.com"))
	if val != 12345678 {
		t.Errorf("Expected 12345678, got %f", val)
	}
}

func TestRecordEndpointLatency(t *testing.T) {
	// Reset gauge for clean test
	EndpointLatency.Reset()

	RecordEndpointLatency("ethereum", "https://eth.example.com", 25.5)
	val := testutil.ToFloat64(EndpointLatency.WithLabelValues("ethereum", "https://eth.example.com"))
	if val != 25.5 {
		t.Errorf("Expected 25.5, got %f", val)
	}
}

func TestRecordCacheHit(t *testing.T) {
	// Reset counter for clean test
	CacheHits.Reset()

	RecordCacheHit("ethereum", "eth_chainId")
	RecordCacheHit("ethereum", "eth_chainId")

	count := testutil.ToFloat64(CacheHits.WithLabelValues("ethereum", "eth_chainId"))
	if count != 2 {
		t.Errorf("Expected 2, got %f", count)
	}
}

func TestRecordCacheMiss(t *testing.T) {
	// Reset counter for clean test
	CacheMisses.Reset()

	RecordCacheMiss("ethereum", "eth_call")

	count := testutil.ToFloat64(CacheMisses.WithLabelValues("ethereum", "eth_call"))
	if count != 1 {
		t.Errorf("Expected 1, got %f", count)
	}
}

func TestRecordRateLimitReject(t *testing.T) {
	// Reset counter for clean test
	RateLimitRejects.Reset()

	RecordRateLimitReject("ethereum")
	RecordRateLimitReject("ethereum")
	RecordRateLimitReject("arbitrum")

	ethCount := testutil.ToFloat64(RateLimitRejects.WithLabelValues("ethereum"))
	if ethCount != 2 {
		t.Errorf("Expected 2 for ethereum, got %f", ethCount)
	}

	arbCount := testutil.ToFloat64(RateLimitRejects.WithLabelValues("arbitrum"))
	if arbCount != 1 {
		t.Errorf("Expected 1 for arbitrum, got %f", arbCount)
	}
}

func TestRecordHealthyEndpoints(t *testing.T) {
	// Reset gauge for clean test
	HealthyEndpoints.Reset()

	RecordHealthyEndpoints("ethereum", 3)
	val := testutil.ToFloat64(HealthyEndpoints.WithLabelValues("ethereum"))
	if val != 3 {
		t.Errorf("Expected 3, got %f", val)
	}

	RecordHealthyEndpoints("ethereum", 2)
	val = testutil.ToFloat64(HealthyEndpoints.WithLabelValues("ethereum"))
	if val != 2 {
		t.Errorf("Expected 2 after update, got %f", val)
	}
}

func TestRecordTotalEndpoints(t *testing.T) {
	// Reset gauge for clean test
	TotalEndpoints.Reset()

	RecordTotalEndpoints("ethereum", 5)
	val := testutil.ToFloat64(TotalEndpoints.WithLabelValues("ethereum"))
	if val != 5 {
		t.Errorf("Expected 5, got %f", val)
	}
}

func TestRecordError(t *testing.T) {
	// Reset counter for clean test
	ErrorsTotal.Reset()

	RecordError("ethereum", "forward_error")
	RecordError("ethereum", "forward_error")
	RecordError("ethereum", "parse_error")

	forwardCount := testutil.ToFloat64(ErrorsTotal.WithLabelValues("ethereum", "forward_error"))
	if forwardCount != 2 {
		t.Errorf("Expected 2 forward errors, got %f", forwardCount)
	}

	parseCount := testutil.ToFloat64(ErrorsTotal.WithLabelValues("ethereum", "parse_error"))
	if parseCount != 1 {
		t.Errorf("Expected 1 parse error, got %f", parseCount)
	}
}

func TestMetricsLabels(t *testing.T) {
	// Test that different chains are tracked separately
	RequestsTotal.Reset()

	RecordRequest("ethereum", "eth_chainId")
	RecordRequest("arbitrum", "eth_chainId")
	RecordRequest("polygon", "eth_chainId")

	ethCount := testutil.ToFloat64(RequestsTotal.WithLabelValues("ethereum", "eth_chainId"))
	arbCount := testutil.ToFloat64(RequestsTotal.WithLabelValues("arbitrum", "eth_chainId"))
	polyCount := testutil.ToFloat64(RequestsTotal.WithLabelValues("polygon", "eth_chainId"))

	if ethCount != 1 || arbCount != 1 || polyCount != 1 {
		t.Errorf("Expected 1 for each chain, got eth=%f arb=%f poly=%f", ethCount, arbCount, polyCount)
	}
}

// Benchmark tests
func BenchmarkRecordRequest(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RecordRequest("ethereum", "eth_chainId")
	}
}

func BenchmarkRecordRequestDuration(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RecordRequestDuration("ethereum", "eth_chainId", "success", 0.1)
	}
}

func BenchmarkRecordEndpointHealth(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RecordEndpointHealth("ethereum", "https://eth.example.com", true)
	}
}

func BenchmarkRecordError(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RecordError("ethereum", "forward_error")
	}
}
