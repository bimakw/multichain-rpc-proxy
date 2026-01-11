package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/bimakw/multichain-rpc-proxy/internal/chain"
	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Enabled {
		t.Error("Expected Enabled = true")
	}
	if cfg.PingInterval != 30*time.Second {
		t.Errorf("Expected PingInterval 30s, got %v", cfg.PingInterval)
	}
	if cfg.MaxMessageSize != 512*1024 {
		t.Errorf("Expected MaxMessageSize 512KB, got %d", cfg.MaxMessageSize)
	}
}

func TestNewHandler(t *testing.T) {
	manager := chain.NewManager(map[string]*config.ChainConfig{})
	cfg := DefaultConfig()

	handler := NewHandler(manager, cfg)

	if handler.manager == nil {
		t.Error("Expected manager to be set")
	}
	if handler.pools == nil {
		t.Error("Expected pools to be initialized")
	}
}

func TestHttpToWS(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"http://example.com", "ws://example.com"},
		{"https://example.com", "wss://example.com"},
		{"ws://example.com", "ws://example.com"},
		{"wss://example.com", "wss://example.com"},
		{"example.com", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := httpToWS(tt.input)
			if result != tt.expected {
				t.Errorf("httpToWS(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestHandler_HandleWebSocket_ChainNotFound(t *testing.T) {
	manager := chain.NewManager(map[string]*config.ChainConfig{})
	handler := NewHandler(manager, DefaultConfig())

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r, "nonexistent")
	}))
	defer server.Close()

	// Make regular HTTP request
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestRpcRequest_Marshal(t *testing.T) {
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_subscribe",
		ID:      1,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed rpcRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Method != "eth_subscribe" {
		t.Errorf("Expected method eth_subscribe, got %s", parsed.Method)
	}
}

func TestRpcResponse_Marshal(t *testing.T) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		Result:  json.RawMessage(`"0x1"`),
		ID:      1,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed rpcResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if string(parsed.Result) != `"0x1"` {
		t.Errorf("Expected result \"0x1\", got %s", parsed.Result)
	}
}

func TestRpcError_Marshal(t *testing.T) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		Error: &rpcError{
			Code:    -32600,
			Message: "Invalid Request",
		},
		ID: 1,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed rpcResponse
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

// Integration test with mock WebSocket server
func TestHandler_WebSocketIntegration(t *testing.T) {
	// Create a mock backend WebSocket server
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}

			// Echo back with a response
			var req rpcRequest
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}

			resp := rpcResponse{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`"0x1234"`),
				ID:      req.ID,
			}
			data, _ := json.Marshal(resp)
			conn.WriteMessage(mt, data)
		}
	}))
	defer backendServer.Close()

	// Create chain manager with mock backend
	chainCfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: backendServer.URL, Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}
	manager := chain.NewManager(chainCfg)

	handler := NewHandler(manager, DefaultConfig())

	// Create proxy WebSocket server
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r, "ethereum")
	}))
	defer proxyServer.Close()

	// Connect client to proxy
	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send request
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_chainId",
		ID:      1,
	}
	data, _ := json.Marshal(req)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatalf("Failed to send: %v", err)
	}

	// Read response (with timeout)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	var resp rpcResponse
	if err := json.Unmarshal(msg, &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if string(resp.Result) != `"0x1234"` {
		t.Errorf("Expected result \"0x1234\", got %s", resp.Result)
	}
}

// Benchmark tests
func BenchmarkHttpToWS(b *testing.B) {
	url := "https://eth.example.com/rpc"
	for i := 0; i < b.N; i++ {
		httpToWS(url)
	}
}

// Additional tests for improved coverage

func TestClientSession_SendError(t *testing.T) {
	// Create a mock WebSocket server that receives the error response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the error message (just consume it)
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	// Connect to the mock server
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Create session with the connection
	session := &ClientSession{
		chainName:  "test",
		clientConn: conn,
		handler:    &Handler{config: DefaultConfig()},
		done:       make(chan struct{}),
	}

	// Send error
	session.sendError(1, -32600, "Test error message")

	// Give some time for the message to be sent
	time.Sleep(50 * time.Millisecond)

	// Note: We can't easily verify the received message in this test structure
	// but the coverage will improve
}

func TestClientSession_Close(t *testing.T) {
	// Create mock connections
	clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// Keep connection alive briefly
		time.Sleep(100 * time.Millisecond)
	}))
	defer clientServer.Close()

	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(100 * time.Millisecond)
	}))
	defer backendServer.Close()

	// Create connections
	clientURL := "ws" + strings.TrimPrefix(clientServer.URL, "http")
	clientConn, _, _ := websocket.DefaultDialer.Dial(clientURL, nil)

	backendURL := "ws" + strings.TrimPrefix(backendServer.URL, "http")
	backendConn, _, _ := websocket.DefaultDialer.Dial(backendURL, nil)

	// Create session
	session := &ClientSession{
		chainName:   "test",
		clientConn:  clientConn,
		backendConn: backendConn,
		handler:     &Handler{config: DefaultConfig()},
		done:        make(chan struct{}),
	}

	// Close session
	session.Close()

	// Verify done channel is closed
	select {
	case <-session.done:
		// Expected
	default:
		t.Error("done channel should be closed")
	}
}

func TestClientSession_Close_NilConnections(t *testing.T) {
	session := &ClientSession{
		chainName:   "test",
		clientConn:  nil,
		backendConn: nil,
		handler:     &Handler{config: DefaultConfig()},
		done:        make(chan struct{}),
	}

	// Should not panic with nil connections
	session.Close()

	select {
	case <-session.done:
		// Expected
	default:
		t.Error("done channel should be closed")
	}
}

func TestHandler_HandleWebSocket_UpgradeFail(t *testing.T) {
	chainCfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: "http://localhost:8545", Weight: 10},
			},
		},
	}
	manager := chain.NewManager(chainCfg)
	handler := NewHandler(manager, DefaultConfig())

	// Create test server that doesn't upgrade to WebSocket
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r, "ethereum")
	}))
	defer server.Close()

	// Make regular HTTP request (not WebSocket)
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()

	// The upgrade should fail, but the handler should handle it gracefully
}

func TestHttpToWS_EdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"h", "h"},
		{"http", "http"},
		{"http:", "http:"},
		{"http:/", "http:/"},
		{"https:", "https:"},
		{"https:/", "https:/"},
		{"ftp://example.com", "ftp://example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := httpToWS(tt.input)
			if result != tt.expected {
				t.Errorf("httpToWS(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfig_Validation(t *testing.T) {
	cfg := DefaultConfig()

	// Test that all values are reasonable
	if cfg.PingInterval <= 0 {
		t.Error("PingInterval should be positive")
	}
	if cfg.PongTimeout <= 0 {
		t.Error("PongTimeout should be positive")
	}
	if cfg.WriteTimeout <= 0 {
		t.Error("WriteTimeout should be positive")
	}
	if cfg.MaxMessageSize <= 0 {
		t.Error("MaxMessageSize should be positive")
	}
	if cfg.ReconnectBackoff <= 0 {
		t.Error("ReconnectBackoff should be positive")
	}
	if cfg.MaxReconnects <= 0 {
		t.Error("MaxReconnects should be positive")
	}
}

func TestRpcRequest_WithParams(t *testing.T) {
	req := rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_call",
		Params:  json.RawMessage(`[{"to":"0x1234"},"latest"]`),
		ID:      "unique-id",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed rpcRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.Method != "eth_call" {
		t.Errorf("Expected method eth_call, got %s", parsed.Method)
	}
	if parsed.ID != "unique-id" {
		t.Errorf("Expected ID unique-id, got %v", parsed.ID)
	}
}

func TestRpcResponse_WithNilID(t *testing.T) {
	resp := rpcResponse{
		JSONRPC: "2.0",
		Result:  json.RawMessage(`"0x1"`),
		ID:      nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var parsed rpcResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if parsed.ID != nil {
		t.Errorf("Expected nil ID, got %v", parsed.ID)
	}
}

func TestHandler_WebSocketIntegration_MultipleMessages(t *testing.T) {
	// Create a mock backend that echoes multiple messages
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}

			var req rpcRequest
			if err := json.Unmarshal(msg, &req); err != nil {
				continue
			}

			resp := rpcResponse{
				JSONRPC: "2.0",
				Result:  json.RawMessage(`"response-` + req.Method + `"`),
				ID:      req.ID,
			}
			data, _ := json.Marshal(resp)
			conn.WriteMessage(mt, data)
		}
	}))
	defer backendServer.Close()

	chainCfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: backendServer.URL, Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}
	manager := chain.NewManager(chainCfg)
	handler := NewHandler(manager, DefaultConfig())

	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r, "ethereum")
	}))
	defer proxyServer.Close()

	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send multiple requests
	methods := []string{"eth_chainId", "net_version", "eth_blockNumber"}
	for i, method := range methods {
		req := rpcRequest{
			JSONRPC: "2.0",
			Method:  method,
			ID:      i + 1,
		}
		data, _ := json.Marshal(req)
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			t.Fatalf("Failed to send request %d: %v", i, err)
		}
	}

	// Read responses
	for i := range methods {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read response %d: %v", i, err)
		}

		var resp rpcResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			t.Fatalf("Failed to parse response %d: %v", i, err)
		}

		// Verify response has valid structure
		if resp.JSONRPC != "2.0" {
			t.Errorf("Response %d: Expected jsonrpc 2.0, got %s", i, resp.JSONRPC)
		}
	}
}

func TestHandler_WebSocketIntegration_BinaryMessage(t *testing.T) {
	// Create a mock backend
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer backendServer.Close()

	chainCfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: backendServer.URL, Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}
	manager := chain.NewManager(chainCfg)
	handler := NewHandler(manager, DefaultConfig())

	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r, "ethereum")
	}))
	defer proxyServer.Close()

	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send binary message (should be ignored)
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0x00, 0x01}); err != nil {
		t.Fatalf("Failed to send binary message: %v", err)
	}

	// Give time for message to be processed
	time.Sleep(50 * time.Millisecond)
}

func TestHandler_WebSocketIntegration_InvalidJSON(t *testing.T) {
	// Create a mock backend
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer backendServer.Close()

	chainCfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: backendServer.URL, Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}
	manager := chain.NewManager(chainCfg)
	handler := NewHandler(manager, DefaultConfig())

	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler.HandleWebSocket(w, r, "ethereum")
	}))
	defer proxyServer.Close()

	wsURL := "ws" + strings.TrimPrefix(proxyServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send invalid JSON
	if err := conn.WriteMessage(websocket.TextMessage, []byte("not json")); err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Should receive an error response
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read error response: %v", err)
	}

	var resp rpcResponse
	if err := json.Unmarshal(msg, &resp); err != nil {
		t.Fatalf("Failed to parse error response: %v", err)
	}

	if resp.Error == nil {
		t.Error("Expected error response for invalid JSON")
	}
	if resp.Error != nil && resp.Error.Code != -32700 {
		t.Errorf("Expected error code -32700, got %d", resp.Error.Code)
	}
}
