package chain

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

func TestParseBlockNumber(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected int64
		wantErr  bool
	}{
		{
			name:     "hex with 0x prefix",
			input:    []byte(`"0xbc614e"`),
			expected: 12345678,
			wantErr:  false,
		},
		{
			name:     "hex lowercase",
			input:    []byte(`"0xabc123"`),
			expected: 11256099,
			wantErr:  false,
		},
		{
			name:     "hex uppercase",
			input:    []byte(`"0xABC123"`),
			expected: 11256099,
			wantErr:  false,
		},
		{
			name:     "hex odd length",
			input:    []byte(`"0x1"`),
			expected: 1,
			wantErr:  false,
		},
		{
			name:     "empty string",
			input:    []byte(`"0x"`),
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "just 0x",
			input:    []byte(`"0x0"`),
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "large block number",
			input:    []byte(`"0x1234567890abcdef"`),
			expected: 1311768467294899695,
			wantErr:  false,
		},
		{
			name:     "without 0x prefix",
			input:    []byte(`"bc614e"`),
			expected: 12345678,
			wantErr:  false,
		},
		{
			name:     "single digit",
			input:    []byte(`"0xf"`),
			expected: 15,
			wantErr:  false,
		},
		{
			name:     "ethereum mainnet typical block",
			input:    []byte(`"0x13bff9d"`),
			expected: 20709277,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseBlockNumber(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestNewHealthChecker(t *testing.T) {
	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{},
	}

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		MaxBlockLag: 50,
	}

	hc := NewHealthChecker(chain, cfg)

	if hc.chain != chain {
		t.Error("Chain not set correctly")
	}
	if hc.maxBlockLag != 50 {
		t.Errorf("Expected maxBlockLag 50, got %d", hc.maxBlockLag)
	}
}

func TestHealthChecker_StartStop(t *testing.T) {
	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{},
	}

	cfg := config.HealthCheckConfig{
		Interval:    100 * time.Millisecond,
		Timeout:     50 * time.Millisecond,
		MaxBlockLag: 50,
	}

	hc := NewHealthChecker(chain, cfg)

	hc.Start()
	time.Sleep(50 * time.Millisecond)
	hc.Stop()

	// Should not hang
}

func TestHealthChecker_CheckEndpoint(t *testing.T) {
	blockNumber := int64(12345678)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0xbc614e"`), // 12345678
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{ep},
	}

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		MaxBlockLag: 50,
	}

	hc := NewHealthChecker(chain, cfg)

	height, healthy := hc.checkEndpoint(ep)

	if !healthy {
		t.Error("Expected endpoint to be healthy")
	}
	if height != blockNumber {
		t.Errorf("Expected block height %d, got %d", blockNumber, height)
	}
}

func TestHealthChecker_CheckEndpoint_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{ep},
	}

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		MaxBlockLag: 50,
	}

	hc := NewHealthChecker(chain, cfg)

	height, healthy := hc.checkEndpoint(ep)

	if healthy {
		t.Error("Expected endpoint to be unhealthy")
	}
	if height != 0 {
		t.Errorf("Expected block height 0, got %d", height)
	}
}

func TestHealthChecker_CheckEndpoint_RPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Error: &RPCError{
				Code:    -32000,
				Message: "internal error",
			},
			ID: 1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{ep},
	}

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		MaxBlockLag: 50,
	}

	hc := NewHealthChecker(chain, cfg)

	_, healthy := hc.checkEndpoint(ep)

	if healthy {
		t.Error("Expected endpoint to be unhealthy on RPC error")
	}
}

func TestHealthChecker_CheckAll_BlockLag(t *testing.T) {
	// Two servers with different block heights
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0x64"`), // 100
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0x32"`), // 50 (lag of 50)
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server2.Close()

	ep1 := NewEndpoint(server1.URL, 10, 5*time.Second)
	ep2 := NewEndpoint(server2.URL, 10, 5*time.Second)

	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{ep1, ep2},
	}

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		MaxBlockLag: 10, // Only allow 10 blocks lag
	}

	hc := NewHealthChecker(chain, cfg)
	hc.checkAll()

	// ep1 should be healthy (at max height)
	if !ep1.IsHealthy() {
		t.Error("ep1 should be healthy (at max height)")
	}

	// ep2 should be unhealthy (50 blocks behind, max lag is 10)
	if ep2.IsHealthy() {
		t.Error("ep2 should be unhealthy (too far behind)")
	}

	// Chain should have highest block
	if chain.HighestBlock() != 100 {
		t.Errorf("Expected highest block 100, got %d", chain.HighestBlock())
	}
}

func TestHealthChecker_CheckAll_AllHealthy(t *testing.T) {
	// Two servers at same block height
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0x64"`), // 100
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0x5f"`), // 95 (only 5 blocks behind)
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server2.Close()

	ep1 := NewEndpoint(server1.URL, 10, 5*time.Second)
	ep2 := NewEndpoint(server2.URL, 10, 5*time.Second)

	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{ep1, ep2},
	}

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		MaxBlockLag: 10, // Allow 10 blocks lag
	}

	hc := NewHealthChecker(chain, cfg)
	hc.checkAll()

	// Both should be healthy
	if !ep1.IsHealthy() {
		t.Error("ep1 should be healthy")
	}
	if !ep2.IsHealthy() {
		t.Error("ep2 should be healthy (within acceptable lag)")
	}
}

func TestHealthChecker_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 100*time.Millisecond)

	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{ep},
	}

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Second,
		Timeout:     50 * time.Millisecond,
		MaxBlockLag: 50,
	}

	hc := NewHealthChecker(chain, cfg)
	hc.checkAll()

	// Should be unhealthy due to timeout
	if ep.IsHealthy() {
		t.Error("Endpoint should be unhealthy after timeout")
	}
}

// Benchmark tests
func BenchmarkParseBlockNumber(b *testing.B) {
	input := []byte(`"0xbc614e"`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseBlockNumber(input)
	}
}

func BenchmarkHealthChecker_CheckEndpoint(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0xbc614e"`),
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	chain := &Chain{
		Name:      "test",
		endpoints: []*Endpoint{ep},
	}

	cfg := config.HealthCheckConfig{
		Interval:    10 * time.Second,
		Timeout:     5 * time.Second,
		MaxBlockLag: 50,
	}

	hc := NewHealthChecker(chain, cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.checkEndpoint(ep)
	}
}
