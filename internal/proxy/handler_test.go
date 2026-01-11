package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/bimakw/multichain-rpc-proxy/internal/cache"
	"github.com/bimakw/multichain-rpc-proxy/internal/chain"
	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

func setupTestHandler(t *testing.T, rpcServer *httptest.Server) (*Handler, *fiber.App) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: rpcServer.URL, Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval:    10 * time.Second,
				Timeout:     5 * time.Second,
				MaxBlockLag: 10,
			},
		},
	}

	manager := chain.NewManager(cfg)
	// Set endpoint as healthy
	ch, _ := manager.GetChain("ethereum")
	for _, ep := range ch.Stats().Endpoints {
		_ = ep // Endpoints are healthy by default
	}

	testCache := cache.NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId", "net_version"},
	})

	handler := NewHandler(manager, testCache)
	app := fiber.New()

	app.Get("/health", handler.HandleHealth)
	app.Get("/health/:chain", handler.HandleChainHealth)
	app.Get("/chains", handler.HandleChains)
	app.Post("/:chain", handler.HandleRPC)

	return handler, app
}

func TestHandler_HandleRPC_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	body := `{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}`
	req := httptest.NewRequest("POST", "/ethereum", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result["result"] != "0x1" {
		t.Errorf("Expected result 0x1, got %v", result["result"])
	}
}

func TestHandler_HandleRPC_ChainNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	body := `{"jsonrpc":"2.0","method":"eth_chainId","id":1}`
	req := httptest.NewRequest("POST", "/nonexistent", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestHandler_HandleRPC_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	body := `invalid json`
	req := httptest.NewRequest("POST", "/ethereum", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 400 {
		t.Errorf("Expected 400, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	errObj := result["error"].(map[string]interface{})
	if errObj["code"].(float64) != -32700 {
		t.Errorf("Expected error code -32700, got %v", errObj["code"])
	}
}

func TestHandler_HandleRPC_CacheHit(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	body := `{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}`

	// First request - cache miss
	req := httptest.NewRequest("POST", "/ethereum", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)

	if resp.Header.Get("X-Cache") != "MISS" {
		t.Error("Expected X-Cache: MISS on first request")
	}

	// Second request - cache hit
	req = httptest.NewRequest("POST", "/ethereum", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = app.Test(req)

	if resp.Header.Get("X-Cache") != "HIT" {
		t.Error("Expected X-Cache: HIT on second request")
	}

	// Server should only be called once
	if callCount != 1 {
		t.Errorf("Expected 1 server call, got %d", callCount)
	}
}

func TestHandler_HandleRPC_NonCacheableMethod(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x123"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	body := `{"jsonrpc":"2.0","method":"eth_call","params":[],"id":1}`

	// Both requests should hit the server
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/ethereum", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)
	}

	// Server should be called twice
	if callCount != 2 {
		t.Errorf("Expected 2 server calls, got %d", callCount)
	}
}

func TestHandler_HandleHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if result["status"] != "healthy" {
		t.Errorf("Expected status healthy, got %v", result["status"])
	}

	chains := result["chains"].(map[string]interface{})
	eth := chains["ethereum"].(map[string]interface{})
	if eth["total_endpoints"].(float64) != 1 {
		t.Errorf("Expected 1 endpoint, got %v", eth["total_endpoints"])
	}
}

func TestHandler_HandleChainHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	req := httptest.NewRequest("GET", "/health/ethereum", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if result["chain"] != "ethereum" {
		t.Errorf("Expected chain ethereum, got %v", result["chain"])
	}
	if result["chain_id"].(float64) != 1 {
		t.Errorf("Expected chain_id 1, got %v", result["chain_id"])
	}

	endpoints := result["endpoints"].([]interface{})
	if len(endpoints) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(endpoints))
	}
}

func TestHandler_HandleChainHealth_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	req := httptest.NewRequest("GET", "/health/nonexistent", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 404 {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestHandler_HandleChains(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	_, app := setupTestHandler(t, server)

	req := httptest.NewRequest("GET", "/chains", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	chains := result["chains"].([]interface{})
	if len(chains) != 1 {
		t.Errorf("Expected 1 chain, got %d", len(chains))
	}

	chain := chains[0].(map[string]interface{})
	if chain["name"] != "ethereum" {
		t.Errorf("Expected name ethereum, got %v", chain["name"])
	}
	if chain["rpc_url"] != "/ethereum" {
		t.Errorf("Expected rpc_url /ethereum, got %v", chain["rpc_url"])
	}
}

func TestNewHandler(t *testing.T) {
	manager := chain.NewManager(map[string]*config.ChainConfig{})
	testCache := cache.NewInMemory(config.CacheConfig{Enabled: true})

	handler := NewHandler(manager, testCache)

	if handler.manager == nil {
		t.Error("Expected manager to be set")
	}
	if handler.cache == nil {
		t.Error("Expected cache to be set")
	}
}

func TestRpcRequest_Unmarshal(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected rpcRequest
		wantErr  bool
	}{
		{
			name: "full request",
			json: `{"jsonrpc":"2.0","method":"eth_call","params":[{"to":"0x1234"},"latest"],"id":1}`,
			expected: rpcRequest{
				JSONRPC: "2.0",
				Method:  "eth_call",
				ID:      float64(1),
			},
			wantErr: false,
		},
		{
			name: "no params",
			json: `{"jsonrpc":"2.0","method":"eth_chainId","id":1}`,
			expected: rpcRequest{
				JSONRPC: "2.0",
				Method:  "eth_chainId",
				ID:      float64(1),
			},
			wantErr: false,
		},
		{
			name:    "invalid json",
			json:    `{invalid}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req rpcRequest
			err := json.Unmarshal([]byte(tt.json), &req)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if req.JSONRPC != tt.expected.JSONRPC {
				t.Errorf("Expected jsonrpc %s, got %s", tt.expected.JSONRPC, req.JSONRPC)
			}
			if req.Method != tt.expected.Method {
				t.Errorf("Expected method %s, got %s", tt.expected.Method, req.Method)
			}
		})
	}
}

// Benchmark tests
func BenchmarkHandler_HandleRPC(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: server.URL, Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}

	manager := chain.NewManager(cfg)
	testCache := cache.NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId"},
	})

	handler := NewHandler(manager, testCache)
	app := fiber.New()
	app.Post("/:chain", handler.HandleRPC)

	body := `{"jsonrpc":"2.0","method":"eth_chainId","params":[],"id":1}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("POST", "/ethereum", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		app.Test(req)
	}
}

func BenchmarkHandler_HandleHealth(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: server.URL, Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}

	manager := chain.NewManager(cfg)
	testCache := cache.NewInMemory(config.CacheConfig{Enabled: false})

	handler := NewHandler(manager, testCache)
	app := fiber.New()
	app.Get("/health", handler.HandleHealth)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/health", nil)
		app.Test(req)
	}
}
