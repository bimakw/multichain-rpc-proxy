package chain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNewEndpoint(t *testing.T) {
	ep := NewEndpoint("http://example.com", 10, 30*time.Second)

	if ep.URL != "http://example.com" {
		t.Errorf("Expected URL http://example.com, got %s", ep.URL)
	}
	if ep.Weight != 10 {
		t.Errorf("Expected weight 10, got %d", ep.Weight)
	}
	if !ep.IsHealthy() {
		t.Error("New endpoint should be healthy by default")
	}
}

func TestEndpoint_HealthStatus(t *testing.T) {
	ep := NewEndpoint("http://example.com", 10, 30*time.Second)

	if !ep.IsHealthy() {
		t.Error("Expected healthy initially")
	}

	ep.SetHealthy(false)
	if ep.IsHealthy() {
		t.Error("Expected unhealthy after SetHealthy(false)")
	}

	ep.SetHealthy(true)
	if !ep.IsHealthy() {
		t.Error("Expected healthy after SetHealthy(true)")
	}
}

func TestEndpoint_BlockHeight(t *testing.T) {
	ep := NewEndpoint("http://example.com", 10, 30*time.Second)

	if ep.BlockHeight() != 0 {
		t.Errorf("Expected 0 block height initially, got %d", ep.BlockHeight())
	}

	ep.SetBlockHeight(12345678)
	if ep.BlockHeight() != 12345678 {
		t.Errorf("Expected 12345678, got %d", ep.BlockHeight())
	}
}

func TestEndpoint_Latency(t *testing.T) {
	ep := NewEndpoint("http://example.com", 10, 30*time.Second)

	if ep.Latency() != 0 {
		t.Errorf("Expected 0 latency initially, got %f", ep.Latency())
	}

	// Latency is stored in microseconds, returned in milliseconds
	ep.latency.Store(5000) // 5000 microseconds = 5 ms
	if ep.Latency() != 5.0 {
		t.Errorf("Expected 5.0 ms, got %f", ep.Latency())
	}
}

func TestEndpoint_Stats(t *testing.T) {
	ep := NewEndpoint("http://example.com", 10, 30*time.Second)
	ep.SetHealthy(true)
	ep.SetBlockHeight(12345678)
	ep.latency.Store(2500) // 2.5 ms
	ep.totalReqs.Store(100)
	ep.failedReqs.Store(5)

	stats := ep.Stats()

	if stats.URL != "http://example.com" {
		t.Errorf("Expected URL http://example.com, got %s", stats.URL)
	}
	if !stats.Healthy {
		t.Error("Expected healthy=true")
	}
	if stats.BlockHeight != 12345678 {
		t.Errorf("Expected block height 12345678, got %d", stats.BlockHeight)
	}
	if stats.LatencyMs != 2.5 {
		t.Errorf("Expected latency 2.5, got %f", stats.LatencyMs)
	}
	if stats.TotalReqs != 100 {
		t.Errorf("Expected total reqs 100, got %d", stats.TotalReqs)
	}
	if stats.FailedReqs != 5 {
		t.Errorf("Expected failed reqs 5, got %d", stats.FailedReqs)
	}
}

func TestEndpoint_Call_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0x1"`),
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	req := &RPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_chainId",
		ID:      1,
	}

	ctx := context.Background()
	resp, err := ep.Call(ctx, req)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
	if string(resp.Result) != `"0x1"` {
		t.Errorf("Expected result \"0x1\", got %s", resp.Result)
	}

	// Check stats
	if ep.totalReqs.Load() != 1 {
		t.Errorf("Expected 1 total request, got %d", ep.totalReqs.Load())
	}
	if ep.failedReqs.Load() != 0 {
		t.Errorf("Expected 0 failed requests, got %d", ep.failedReqs.Load())
	}
}

func TestEndpoint_Call_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	req := &RPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_chainId",
		ID:      1,
	}

	ctx := context.Background()
	_, err := ep.Call(ctx, req)

	if err == nil {
		t.Error("Expected error for 500 response")
	}
	if ep.failedReqs.Load() != 1 {
		t.Errorf("Expected 1 failed request, got %d", ep.failedReqs.Load())
	}
}

func TestEndpoint_Call_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 50*time.Millisecond)

	req := &RPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_chainId",
		ID:      1,
	}

	ctx := context.Background()
	_, err := ep.Call(ctx, req)

	if err == nil {
		t.Error("Expected timeout error")
	}
	if ep.failedReqs.Load() != 1 {
		t.Errorf("Expected 1 failed request, got %d", ep.failedReqs.Load())
	}
}

func TestEndpoint_Call_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	req := &RPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_chainId",
		ID:      1,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := ep.Call(ctx, req)

	if err == nil {
		t.Error("Expected context canceled error")
	}
}

func TestEndpoint_Forward_Success(t *testing.T) {
	expectedResponse := `{"jsonrpc":"2.0","id":1,"result":"0x1"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expectedResponse))
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)

	ctx := context.Background()
	resp, err := ep.Forward(ctx, body)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if string(resp) != expectedResponse {
		t.Errorf("Expected %s, got %s", expectedResponse, resp)
	}
}

func TestEndpoint_Forward_ServerError(t *testing.T) {
	errorResponse := `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"internal error"}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(errorResponse))
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)

	ctx := context.Background()
	resp, err := ep.Forward(ctx, body)

	// Forward should still return body even on 500
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if string(resp) != errorResponse {
		t.Errorf("Expected error response body, got %s", resp)
	}
}

func TestEndpoint_ConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0x1"`),
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)

	var wg sync.WaitGroup
	iterations := 50

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := &RPCRequest{
				JSONRPC: "2.0",
				Method:  "eth_chainId",
				ID:      1,
			}
			ep.Call(context.Background(), req)
		}()
	}

	wg.Wait()

	if ep.totalReqs.Load() != int64(iterations) {
		t.Errorf("Expected %d total requests, got %d", iterations, ep.totalReqs.Load())
	}
}

func TestRPCRequest_Marshal(t *testing.T) {
	req := RPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_call",
		Params:  []interface{}{"0x1234", "latest"},
		ID:      1,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed RPCRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Method != "eth_call" {
		t.Errorf("Expected method eth_call, got %s", parsed.Method)
	}
}

func TestRPCResponse_Marshal(t *testing.T) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		Result:  json.RawMessage(`"0xbc614e"`),
		ID:      1,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed RPCResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if string(parsed.Result) != `"0xbc614e"` {
		t.Errorf("Expected result \"0xbc614e\", got %s", parsed.Result)
	}
}

func TestRPCError_Marshal(t *testing.T) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		Error: &RPCError{
			Code:    -32600,
			Message: "Invalid Request",
		},
		ID: 1,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed RPCResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if parsed.Error.Code != -32600 {
		t.Errorf("Expected error code -32600, got %d", parsed.Error.Code)
	}
}

// Benchmark tests
func BenchmarkEndpoint_Call(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RPCResponse{
			JSONRPC: "2.0",
			Result:  json.RawMessage(`"0x1"`),
			ID:      1,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)
	req := &RPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_chainId",
		ID:      1,
	}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ep.Call(ctx, req)
	}
}

func BenchmarkEndpoint_Stats(b *testing.B) {
	ep := NewEndpoint("http://example.com", 10, 5*time.Second)
	ep.SetHealthy(true)
	ep.SetBlockHeight(12345678)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ep.Stats()
	}
}
