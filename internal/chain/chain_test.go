package chain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

func TestNewChain(t *testing.T) {
	cfg := &config.ChainConfig{
		ChainID: 1,
		Endpoints: []config.EndpointConfig{
			{URL: "http://ep1.example.com", Weight: 10},
			{URL: "http://ep2.example.com", Weight: 5},
		},
		HealthCheck: config.HealthCheckConfig{
			Interval:    10 * time.Second,
			Timeout:     5 * time.Second,
			MaxBlockLag: 10,
		},
	}

	chain := NewChain("ethereum", cfg)

	if chain.Name != "ethereum" {
		t.Errorf("Expected name ethereum, got %s", chain.Name)
	}
	if chain.ChainID != 1 {
		t.Errorf("Expected chainID 1, got %d", chain.ChainID)
	}
	if len(chain.endpoints) != 2 {
		t.Errorf("Expected 2 endpoints, got %d", len(chain.endpoints))
	}
}

func TestChain_HighestBlock(t *testing.T) {
	cfg := &config.ChainConfig{
		ChainID:   1,
		Endpoints: []config.EndpointConfig{},
		HealthCheck: config.HealthCheckConfig{
			Interval:    10 * time.Second,
			Timeout:     5 * time.Second,
			MaxBlockLag: 10,
		},
	}

	chain := NewChain("ethereum", cfg)

	if chain.HighestBlock() != 0 {
		t.Errorf("Expected 0 initially, got %d", chain.HighestBlock())
	}

	chain.UpdateHighestBlock(12345678)

	if chain.HighestBlock() != 12345678 {
		t.Errorf("Expected 12345678, got %d", chain.HighestBlock())
	}
}

func TestChain_EndpointCounts(t *testing.T) {
	ep1 := NewEndpoint("http://ep1.example.com", 10, 5*time.Second)
	ep1.SetHealthy(true)
	ep2 := NewEndpoint("http://ep2.example.com", 5, 5*time.Second)
	ep2.SetHealthy(false)

	chain := &Chain{
		Name:         "test",
		endpoints:    []*Endpoint{ep1, ep2},
		loadBalancer: NewLoadBalancer([]*Endpoint{ep1, ep2}),
	}

	if chain.TotalEndpoints() != 2 {
		t.Errorf("Expected 2 total endpoints, got %d", chain.TotalEndpoints())
	}
	if chain.HealthyEndpoints() != 1 {
		t.Errorf("Expected 1 healthy endpoint, got %d", chain.HealthyEndpoints())
	}
}

func TestChain_Stats(t *testing.T) {
	ep1 := NewEndpoint("http://ep1.example.com", 10, 5*time.Second)
	ep1.SetHealthy(true)
	ep1.SetBlockHeight(100)

	ep2 := NewEndpoint("http://ep2.example.com", 5, 5*time.Second)
	ep2.SetHealthy(false)

	chain := &Chain{
		Name:         "ethereum",
		ChainID:      1,
		endpoints:    []*Endpoint{ep1, ep2},
		loadBalancer: NewLoadBalancer([]*Endpoint{ep1, ep2}),
	}
	chain.UpdateHighestBlock(100)

	stats := chain.Stats()

	if stats.Name != "ethereum" {
		t.Errorf("Expected name ethereum, got %s", stats.Name)
	}
	if stats.ChainID != 1 {
		t.Errorf("Expected chainID 1, got %d", stats.ChainID)
	}
	if stats.HighestBlock != 100 {
		t.Errorf("Expected highest block 100, got %d", stats.HighestBlock)
	}
	if stats.TotalEndpoints != 2 {
		t.Errorf("Expected 2 total endpoints, got %d", stats.TotalEndpoints)
	}
	if stats.HealthyEndpoints != 1 {
		t.Errorf("Expected 1 healthy endpoint, got %d", stats.HealthyEndpoints)
	}
	if len(stats.Endpoints) != 2 {
		t.Errorf("Expected 2 endpoint stats, got %d", len(stats.Endpoints))
	}
}

func TestChain_Forward(t *testing.T) {
	expectedResponse := `{"jsonrpc":"2.0","id":1,"result":"0x1"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(expectedResponse))
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)
	ep.SetHealthy(true)

	chain := &Chain{
		Name:         "test",
		endpoints:    []*Endpoint{ep},
		loadBalancer: NewLoadBalancer([]*Endpoint{ep}),
	}

	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	resp, err := chain.Forward(context.Background(), body)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if string(resp) != expectedResponse {
		t.Errorf("Expected %s, got %s", expectedResponse, resp)
	}
}

func TestChain_Forward_NoHealthyEndpoints(t *testing.T) {
	ep := NewEndpoint("http://example.com", 10, 5*time.Second)
	ep.SetHealthy(false)

	chain := &Chain{
		Name:         "test",
		endpoints:    []*Endpoint{ep},
		loadBalancer: NewLoadBalancer([]*Endpoint{ep}),
	}

	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	_, err := chain.Forward(context.Background(), body)

	if err == nil {
		t.Error("Expected error for no healthy endpoints")
	}
}

func TestChain_ForwardWithRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 2 {
			// First call fails
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	ep1 := NewEndpoint(server.URL, 10, 5*time.Second)
	ep1.SetHealthy(true)
	ep2 := NewEndpoint(server.URL, 10, 5*time.Second) // Same server but different endpoint object
	ep2.SetHealthy(true)

	chain := &Chain{
		Name:         "test",
		endpoints:    []*Endpoint{ep1, ep2},
		loadBalancer: NewLoadBalancer([]*Endpoint{ep1, ep2}),
	}

	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	_, err := chain.ForwardWithRetry(context.Background(), body, 2)

	if err != nil {
		t.Fatalf("Expected success after retry, got error: %v", err)
	}
}

func TestChain_ForwardWithRetry_AllFail(t *testing.T) {
	// Use a server that closes connection to cause actual network error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close() // Force connection close to trigger error
		}
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)
	ep.SetHealthy(true)

	chain := &Chain{
		Name:         "test",
		endpoints:    []*Endpoint{ep},
		loadBalancer: NewLoadBalancer([]*Endpoint{ep}),
	}

	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	_, err := chain.ForwardWithRetry(context.Background(), body, 2)

	if err == nil {
		t.Error("Expected error after all retries failed")
	}
}

func TestChain_ForwardWithRetry_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 500*time.Millisecond)
	ep.SetHealthy(true)

	chain := &Chain{
		Name:         "test",
		endpoints:    []*Endpoint{ep},
		loadBalancer: NewLoadBalancer([]*Endpoint{ep}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	_, err := chain.ForwardWithRetry(ctx, body, 5)

	if err == nil {
		t.Error("Expected context error")
	}
}

// Manager tests
func TestNewManager(t *testing.T) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID: 1,
			Endpoints: []config.EndpointConfig{
				{URL: "http://eth.example.com", Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval:    10 * time.Second,
				Timeout:     5 * time.Second,
				MaxBlockLag: 10,
			},
		},
		"arbitrum": {
			ChainID: 42161,
			Endpoints: []config.EndpointConfig{
				{URL: "http://arb.example.com", Weight: 10},
			},
			HealthCheck: config.HealthCheckConfig{
				Interval:    10 * time.Second,
				Timeout:     5 * time.Second,
				MaxBlockLag: 50,
			},
		},
	}

	manager := NewManager(cfg)

	if len(manager.chains) != 2 {
		t.Errorf("Expected 2 chains, got %d", len(manager.chains))
	}
}

func TestManager_GetChain(t *testing.T) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID:   1,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval:    10 * time.Second,
				Timeout:     5 * time.Second,
				MaxBlockLag: 10,
			},
		},
	}

	manager := NewManager(cfg)

	chain, ok := manager.GetChain("ethereum")
	if !ok {
		t.Error("Expected to find ethereum chain")
	}
	if chain.Name != "ethereum" {
		t.Errorf("Expected name ethereum, got %s", chain.Name)
	}

	_, ok = manager.GetChain("nonexistent")
	if ok {
		t.Error("Should not find nonexistent chain")
	}
}

func TestManager_GetChainByID(t *testing.T) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID:   1,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval:    10 * time.Second,
				Timeout:     5 * time.Second,
				MaxBlockLag: 10,
			},
		},
		"arbitrum": {
			ChainID:   42161,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval:    10 * time.Second,
				Timeout:     5 * time.Second,
				MaxBlockLag: 50,
			},
		},
	}

	manager := NewManager(cfg)

	chain, ok := manager.GetChainByID(1)
	if !ok {
		t.Error("Expected to find chain by ID 1")
	}
	if chain.Name != "ethereum" {
		t.Errorf("Expected name ethereum, got %s", chain.Name)
	}

	chain, ok = manager.GetChainByID(42161)
	if !ok {
		t.Error("Expected to find chain by ID 42161")
	}
	if chain.Name != "arbitrum" {
		t.Errorf("Expected name arbitrum, got %s", chain.Name)
	}

	_, ok = manager.GetChainByID(99999)
	if ok {
		t.Error("Should not find chain by nonexistent ID")
	}
}

func TestManager_ListChains(t *testing.T) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID:   1,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
		"arbitrum": {
			ChainID:   42161,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)
	names := manager.ListChains()

	if len(names) != 2 {
		t.Errorf("Expected 2 chain names, got %d", len(names))
	}

	// Check both chains are in the list
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}
	if !found["ethereum"] || !found["arbitrum"] {
		t.Errorf("Expected ethereum and arbitrum, got %v", names)
	}
}

func TestManager_AllStats(t *testing.T) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID:   1,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)
	stats := manager.AllStats()

	if len(stats) != 1 {
		t.Errorf("Expected 1 chain stats, got %d", len(stats))
	}

	ethStats, ok := stats["ethereum"]
	if !ok {
		t.Error("Expected ethereum stats")
	}
	if ethStats.ChainID != 1 {
		t.Errorf("Expected chainID 1, got %d", ethStats.ChainID)
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID:   1,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)

	var wg sync.WaitGroup
	iterations := 100

	for i := 0; i < iterations; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			manager.GetChain("ethereum")
		}()
		go func() {
			defer wg.Done()
			manager.ListChains()
		}()
		go func() {
			defer wg.Done()
			manager.AllStats()
		}()
	}

	wg.Wait()
}

// Benchmark tests
func BenchmarkChain_Forward(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer server.Close()

	ep := NewEndpoint(server.URL, 10, 5*time.Second)
	ep.SetHealthy(true)

	chain := &Chain{
		Name:         "test",
		endpoints:    []*Endpoint{ep},
		loadBalancer: NewLoadBalancer([]*Endpoint{ep}),
	}

	body := []byte(`{"jsonrpc":"2.0","method":"eth_chainId","id":1}`)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		chain.Forward(ctx, body)
	}
}

func BenchmarkManager_GetChain(b *testing.B) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID:   1,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.GetChain("ethereum")
	}
}

func BenchmarkManager_AllStats(b *testing.B) {
	cfg := map[string]*config.ChainConfig{
		"ethereum": {
			ChainID:   1,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
		"arbitrum": {
			ChainID:   42161,
			Endpoints: []config.EndpointConfig{},
			HealthCheck: config.HealthCheckConfig{
				Interval: 10 * time.Second,
				Timeout:  5 * time.Second,
			},
		},
	}

	manager := NewManager(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		manager.AllStats()
	}
}
