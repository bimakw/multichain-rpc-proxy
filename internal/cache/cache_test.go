package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

func TestNewInMemory(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.CacheConfig
		enabled bool
	}{
		{
			name: "enabled cache with methods",
			cfg: config.CacheConfig{
				Enabled:          true,
				TTL:              60 * time.Second,
				CacheableMethods: []string{"eth_chainId", "net_version"},
			},
			enabled: true,
		},
		{
			name: "disabled cache",
			cfg: config.CacheConfig{
				Enabled: false,
			},
			enabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewInMemory(tt.cfg)
			if cache.Enabled() != tt.enabled {
				t.Errorf("Enabled() = %v, want %v", cache.Enabled(), tt.enabled)
			}
		})
	}
}

func TestInMemoryCache_IsCacheable(t *testing.T) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId", "net_version", "web3_clientVersion"},
	})

	tests := []struct {
		method   string
		expected bool
	}{
		{"eth_chainId", true},
		{"net_version", true},
		{"web3_clientVersion", true},
		{"eth_call", false},
		{"eth_sendTransaction", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := cache.IsCacheable(tt.method); got != tt.expected {
				t.Errorf("IsCacheable(%q) = %v, want %v", tt.method, got, tt.expected)
			}
		})
	}
}

func TestInMemoryCache_IsCacheable_Disabled(t *testing.T) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          false,
		CacheableMethods: []string{"eth_chainId"},
	})

	if cache.IsCacheable("eth_chainId") {
		t.Error("IsCacheable should return false when cache is disabled")
	}
}

func TestInMemoryCache_GetSet(t *testing.T) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId"},
	})

	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`)

	// Test cache miss
	_, found := cache.Get(chain, method, params)
	if found {
		t.Error("Get should return false for cache miss")
	}

	// Set cache entry
	cache.Set(chain, method, params, response)

	// Test cache hit
	got, found := cache.Get(chain, method, params)
	if !found {
		t.Error("Get should return true for cache hit")
	}
	if string(got) != string(response) {
		t.Errorf("Get() = %q, want %q", got, response)
	}
}

func TestInMemoryCache_GetSet_DifferentParams(t *testing.T) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_call"},
	})

	chain := "ethereum"
	method := "eth_call"

	params1 := []byte(`[{"to":"0x1234"}]`)
	response1 := []byte(`{"result":"0xabc"}`)

	params2 := []byte(`[{"to":"0x5678"}]`)
	response2 := []byte(`{"result":"0xdef"}`)

	// Set both entries
	cache.Set(chain, method, params1, response1)
	cache.Set(chain, method, params2, response2)

	// Verify they are stored separately
	got1, found1 := cache.Get(chain, method, params1)
	if !found1 || string(got1) != string(response1) {
		t.Errorf("First entry mismatch: got %q, want %q", got1, response1)
	}

	got2, found2 := cache.Get(chain, method, params2)
	if !found2 || string(got2) != string(response2) {
		t.Errorf("Second entry mismatch: got %q, want %q", got2, response2)
	}
}

func TestInMemoryCache_GetSet_DifferentChains(t *testing.T) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId"},
	})

	method := "eth_chainId"
	params := []byte(`[]`)

	responseEth := []byte(`{"result":"0x1"}`)
	responseArb := []byte(`{"result":"0xa4b1"}`)

	cache.Set("ethereum", method, params, responseEth)
	cache.Set("arbitrum", method, params, responseArb)

	gotEth, _ := cache.Get("ethereum", method, params)
	gotArb, _ := cache.Get("arbitrum", method, params)

	if string(gotEth) != string(responseEth) {
		t.Errorf("Ethereum result mismatch: got %q, want %q", gotEth, responseEth)
	}
	if string(gotArb) != string(responseArb) {
		t.Errorf("Arbitrum result mismatch: got %q, want %q", gotArb, responseArb)
	}
}

func TestInMemoryCache_Expiration(t *testing.T) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              50 * time.Millisecond, // Short TTL for testing
		CacheableMethods: []string{"eth_chainId"},
	})

	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"result":"0x1"}`)

	cache.Set(chain, method, params, response)

	// Verify entry exists
	_, found := cache.Get(chain, method, params)
	if !found {
		t.Error("Entry should exist immediately after setting")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Entry should be expired
	_, found = cache.Get(chain, method, params)
	if found {
		t.Error("Entry should be expired after TTL")
	}
}

func TestInMemoryCache_Disabled(t *testing.T) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          false,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId"},
	})

	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"result":"0x1"}`)

	// Set should be no-op when disabled
	cache.Set(chain, method, params, response)

	// Get should return false when disabled
	_, found := cache.Get(chain, method, params)
	if found {
		t.Error("Get should return false when cache is disabled")
	}
}

func TestInMemoryCache_EmptyParams(t *testing.T) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId"},
	})

	chain := "ethereum"
	method := "eth_chainId"
	response := []byte(`{"result":"0x1"}`)

	// Test with nil params
	cache.Set(chain, method, nil, response)
	got, found := cache.Get(chain, method, nil)
	if !found {
		t.Error("Should find entry with nil params")
	}
	if string(got) != string(response) {
		t.Errorf("Got %q, want %q", got, response)
	}

	// Test with empty slice params
	cache.Set(chain, method, []byte{}, response)
	got, found = cache.Get(chain, method, []byte{})
	if !found {
		t.Error("Should find entry with empty params")
	}
}

func TestCache_Disabled(t *testing.T) {
	cache := &Cache{enabled: false}

	if cache.Enabled() {
		t.Error("Cache should report as disabled")
	}

	if cache.IsCacheable("eth_chainId") {
		t.Error("IsCacheable should return false when disabled")
	}
}

func TestCache_Close_NilClient(t *testing.T) {
	cache := &Cache{client: nil, enabled: false}

	err := cache.Close()
	if err != nil {
		t.Errorf("Close() with nil client should not error, got %v", err)
	}
}

// Benchmark tests
func BenchmarkInMemoryCache_Get(b *testing.B) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId"},
	})

	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"result":"0x1"}`)
	cache.Set(chain, method, params, response)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(chain, method, params)
	}
}

func BenchmarkInMemoryCache_Set(b *testing.B) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId"},
	})

	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"result":"0x1"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(chain, method, params, response)
	}
}

func BenchmarkInMemoryCache_IsCacheable(b *testing.B) {
	cache := NewInMemory(config.CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId", "net_version", "web3_clientVersion"},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.IsCacheable("eth_chainId")
	}
}

// Redis Cache Tests using miniredis

func TestNew_Disabled(t *testing.T) {
	cache, err := New(config.RedisConfig{}, config.CacheConfig{Enabled: false})
	if err != nil {
		t.Fatalf("New() with disabled cache should not error: %v", err)
	}
	if cache.Enabled() {
		t.Error("Cache should be disabled")
	}
}

func TestNew_Enabled(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId", "net_version"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	if !cache.Enabled() {
		t.Error("Cache should be enabled")
	}
}

func TestNew_ConnectionError(t *testing.T) {
	_, err := New(
		config.RedisConfig{
			Addr:     "localhost:9999", // Invalid address
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err == nil {
		t.Error("New() should fail with invalid Redis address")
	}
}

func TestCache_GetSet(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`)

	// Test cache miss
	_, found := cache.Get(ctx, chain, method, params)
	if found {
		t.Error("Get should return false for cache miss")
	}

	// Set cache entry
	err = cache.Set(ctx, chain, method, params, response)
	if err != nil {
		t.Fatalf("Set() failed: %v", err)
	}

	// Test cache hit
	got, found := cache.Get(ctx, chain, method, params)
	if !found {
		t.Error("Get should return true for cache hit")
	}
	if string(got) != string(response) {
		t.Errorf("Get() = %q, want %q", got, response)
	}
}

func TestCache_GetSet_DifferentChains(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	method := "eth_chainId"
	params := []byte(`[]`)

	responseEth := []byte(`{"result":"0x1"}`)
	responseArb := []byte(`{"result":"0xa4b1"}`)

	cache.Set(ctx, "ethereum", method, params, responseEth)
	cache.Set(ctx, "arbitrum", method, params, responseArb)

	gotEth, _ := cache.Get(ctx, "ethereum", method, params)
	gotArb, _ := cache.Get(ctx, "arbitrum", method, params)

	if string(gotEth) != string(responseEth) {
		t.Errorf("Ethereum result mismatch: got %q, want %q", gotEth, responseEth)
	}
	if string(gotArb) != string(responseArb) {
		t.Errorf("Arbitrum result mismatch: got %q, want %q", gotArb, responseArb)
	}
}

func TestCache_GetSet_DifferentParams(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_call"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	chain := "ethereum"
	method := "eth_call"

	params1 := []byte(`[{"to":"0x1234"}]`)
	response1 := []byte(`{"result":"0xabc"}`)

	params2 := []byte(`[{"to":"0x5678"}]`)
	response2 := []byte(`{"result":"0xdef"}`)

	cache.Set(ctx, chain, method, params1, response1)
	cache.Set(ctx, chain, method, params2, response2)

	got1, found1 := cache.Get(ctx, chain, method, params1)
	if !found1 || string(got1) != string(response1) {
		t.Errorf("First entry mismatch: got %q, want %q", got1, response1)
	}

	got2, found2 := cache.Get(ctx, chain, method, params2)
	if !found2 || string(got2) != string(response2) {
		t.Errorf("Second entry mismatch: got %q, want %q", got2, response2)
	}
}

func TestCache_IsCacheable(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId", "net_version"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	tests := []struct {
		method   string
		expected bool
	}{
		{"eth_chainId", true},
		{"net_version", true},
		{"eth_call", false},
		{"eth_sendTransaction", false},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := cache.IsCacheable(tt.method); got != tt.expected {
				t.Errorf("IsCacheable(%q) = %v, want %v", tt.method, got, tt.expected)
			}
		})
	}
}

func TestCache_GetSet_Disabled(t *testing.T) {
	cache := &Cache{enabled: false}

	ctx := context.Background()
	_, found := cache.Get(ctx, "ethereum", "eth_chainId", []byte(`[]`))
	if found {
		t.Error("Get should return false when disabled")
	}

	err := cache.Set(ctx, "ethereum", "eth_chainId", []byte(`[]`), []byte(`{"result":"0x1"}`))
	if err != nil {
		t.Errorf("Set should not error when disabled: %v", err)
	}
}

func TestCache_Close(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	err = cache.Close()
	if err != nil {
		t.Errorf("Close() should not error: %v", err)
	}
}

func TestCache_MakeKey(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	// Test that same inputs produce same key
	key1 := cache.makeKey("ethereum", "eth_chainId", []byte(`[]`))
	key2 := cache.makeKey("ethereum", "eth_chainId", []byte(`[]`))
	if key1 != key2 {
		t.Error("Same inputs should produce same key")
	}

	// Test that different inputs produce different keys
	key3 := cache.makeKey("arbitrum", "eth_chainId", []byte(`[]`))
	if key1 == key3 {
		t.Error("Different chain should produce different key")
	}

	key4 := cache.makeKey("ethereum", "net_version", []byte(`[]`))
	if key1 == key4 {
		t.Error("Different method should produce different key")
	}

	key5 := cache.makeKey("ethereum", "eth_chainId", []byte(`["param"]`))
	if key1 == key5 {
		t.Error("Different params should produce different key")
	}

	// Test key format
	if len(key1) < 10 {
		t.Errorf("Key should have reasonable length, got %d", len(key1))
	}
}

func TestCache_EmptyParams(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	chain := "ethereum"
	method := "eth_chainId"
	response := []byte(`{"result":"0x1"}`)

	// Test with nil params
	err = cache.Set(ctx, chain, method, nil, response)
	if err != nil {
		t.Fatalf("Set with nil params failed: %v", err)
	}

	got, found := cache.Get(ctx, chain, method, nil)
	if !found {
		t.Error("Should find entry with nil params")
	}
	if string(got) != string(response) {
		t.Errorf("Got %q, want %q", got, response)
	}

	// Test with empty slice params
	err = cache.Set(ctx, chain, method, []byte{}, response)
	if err != nil {
		t.Fatalf("Set with empty params failed: %v", err)
	}

	got, found = cache.Get(ctx, chain, method, []byte{})
	if !found {
		t.Error("Should find entry with empty params")
	}
}

func TestCache_Expiration(t *testing.T) {
	mr := miniredis.RunT(t)

	cache, err := New(
		config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "",
			DB:       0,
		},
		config.CacheConfig{
			Enabled:          true,
			TTL:              100 * time.Millisecond,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"result":"0x1"}`)

	cache.Set(ctx, chain, method, params, response)

	// Verify entry exists
	_, found := cache.Get(ctx, chain, method, params)
	if !found {
		t.Error("Entry should exist immediately after setting")
	}

	// Fast-forward time in miniredis
	mr.FastForward(150 * time.Millisecond)

	// Entry should be expired
	_, found = cache.Get(ctx, chain, method, params)
	if found {
		t.Error("Entry should be expired after TTL")
	}
}

// Benchmark for Redis cache
func BenchmarkCache_Get(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("Failed to create miniredis: %v", err)
	}
	defer mr.Close()

	cache, err := New(
		config.RedisConfig{Addr: mr.Addr()},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"result":"0x1"}`)
	cache.Set(ctx, chain, method, params, response)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(ctx, chain, method, params)
	}
}

func BenchmarkCache_Set(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("Failed to create miniredis: %v", err)
	}
	defer mr.Close()

	cache, err := New(
		config.RedisConfig{Addr: mr.Addr()},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	ctx := context.Background()
	chain := "ethereum"
	method := "eth_chainId"
	params := []byte(`[]`)
	response := []byte(`{"result":"0x1"}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(ctx, chain, method, params, response)
	}
}

func BenchmarkCache_MakeKey(b *testing.B) {
	mr, err := miniredis.Run()
	if err != nil {
		b.Fatalf("Failed to create miniredis: %v", err)
	}
	defer mr.Close()

	cache, err := New(
		config.RedisConfig{Addr: mr.Addr()},
		config.CacheConfig{
			Enabled:          true,
			TTL:              60 * time.Second,
			CacheableMethods: []string{"eth_chainId"},
		},
	)
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}
	defer cache.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.makeKey("ethereum", "eth_chainId", []byte(`[]`))
	}
}
