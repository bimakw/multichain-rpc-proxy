package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_FullConfig(t *testing.T) {
	content := `
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

redis:
  addr: "localhost:6379"
  password: "secret"
  db: 1

metrics:
  enabled: true
  port: 9090

rate_limit:
  enabled: true
  requests_per_second: 100
  burst: 200

cache:
  enabled: true
  ttl: 120s
  cacheable_methods:
    - eth_chainId
    - net_version

chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "https://eth.example.com"
        weight: 10
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 10
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Server config
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Expected read_timeout 30s, got %v", cfg.Server.ReadTimeout)
	}

	// Redis config
	if cfg.Redis.Addr != "localhost:6379" {
		t.Errorf("Expected redis addr localhost:6379, got %s", cfg.Redis.Addr)
	}
	if cfg.Redis.Password != "secret" {
		t.Errorf("Expected redis password secret, got %s", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 1 {
		t.Errorf("Expected redis db 1, got %d", cfg.Redis.DB)
	}

	// Metrics config
	if !cfg.Metrics.Enabled {
		t.Error("Expected metrics enabled")
	}
	if cfg.Metrics.Port != 9090 {
		t.Errorf("Expected metrics port 9090, got %d", cfg.Metrics.Port)
	}

	// Rate limit config
	if !cfg.RateLimit.Enabled {
		t.Error("Expected rate limit enabled")
	}
	if cfg.RateLimit.RequestsPerSecond != 100 {
		t.Errorf("Expected 100 rps, got %d", cfg.RateLimit.RequestsPerSecond)
	}
	if cfg.RateLimit.Burst != 200 {
		t.Errorf("Expected burst 200, got %d", cfg.RateLimit.Burst)
	}

	// Cache config
	if !cfg.Cache.Enabled {
		t.Error("Expected cache enabled")
	}
	if cfg.Cache.TTL != 120*time.Second {
		t.Errorf("Expected TTL 120s, got %v", cfg.Cache.TTL)
	}
	if len(cfg.Cache.CacheableMethods) != 2 {
		t.Errorf("Expected 2 cacheable methods, got %d", len(cfg.Cache.CacheableMethods))
	}

	// Chain config
	eth, ok := cfg.Chains["ethereum"]
	if !ok {
		t.Fatal("Expected ethereum chain")
	}
	if eth.ChainID != 1 {
		t.Errorf("Expected chain_id 1, got %d", eth.ChainID)
	}
	if len(eth.Endpoints) != 1 {
		t.Errorf("Expected 1 endpoint, got %d", len(eth.Endpoints))
	}
	if eth.Endpoints[0].URL != "https://eth.example.com" {
		t.Errorf("Expected URL https://eth.example.com, got %s", eth.Endpoints[0].URL)
	}
	if eth.HealthCheck.MaxBlockLag != 10 {
		t.Errorf("Expected max_block_lag 10, got %d", eth.HealthCheck.MaxBlockLag)
	}
}

func TestLoad_Defaults(t *testing.T) {
	content := `
chains:
  ethereum:
    chain_id: 1
    endpoints: []
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Default server port
	if cfg.Server.Port != 8080 {
		t.Errorf("Expected default port 8080, got %d", cfg.Server.Port)
	}

	// Default read timeout
	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Expected default read_timeout 30s, got %v", cfg.Server.ReadTimeout)
	}

	// Default write timeout
	if cfg.Server.WriteTimeout != 30*time.Second {
		t.Errorf("Expected default write_timeout 30s, got %v", cfg.Server.WriteTimeout)
	}

	// Default metrics port
	if cfg.Metrics.Port != 9090 {
		t.Errorf("Expected default metrics port 9090, got %d", cfg.Metrics.Port)
	}

	// Default cache TTL
	if cfg.Cache.TTL != 60*time.Second {
		t.Errorf("Expected default cache TTL 60s, got %v", cfg.Cache.TTL)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	content := `
server:
  port: "not-a-number"
  invalid yaml here
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoad_MultipleChains(t *testing.T) {
	content := `
chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "https://eth1.example.com"
        weight: 10
      - url: "https://eth2.example.com"
        weight: 5
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 10
  arbitrum:
    chain_id: 42161
    endpoints:
      - url: "https://arb.example.com"
        weight: 10
    health_check:
      interval: 15s
      timeout: 5s
      max_block_lag: 50
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Chains) != 2 {
		t.Errorf("Expected 2 chains, got %d", len(cfg.Chains))
	}

	eth, ok := cfg.Chains["ethereum"]
	if !ok {
		t.Fatal("Expected ethereum chain")
	}
	if eth.ChainID != 1 {
		t.Errorf("Expected chain_id 1, got %d", eth.ChainID)
	}
	if len(eth.Endpoints) != 2 {
		t.Errorf("Expected 2 endpoints, got %d", len(eth.Endpoints))
	}

	arb, ok := cfg.Chains["arbitrum"]
	if !ok {
		t.Fatal("Expected arbitrum chain")
	}
	if arb.ChainID != 42161 {
		t.Errorf("Expected chain_id 42161, got %d", arb.ChainID)
	}
	if arb.HealthCheck.MaxBlockLag != 50 {
		t.Errorf("Expected max_block_lag 50, got %d", arb.HealthCheck.MaxBlockLag)
	}
}

func TestCacheConfig_IsCacheableMethod(t *testing.T) {
	cfg := &CacheConfig{
		Enabled:          true,
		TTL:              60 * time.Second,
		CacheableMethods: []string{"eth_chainId", "net_version", "web3_clientVersion"},
	}

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
		{"ETH_CHAINID", false}, // case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if got := cfg.IsCacheableMethod(tt.method); got != tt.expected {
				t.Errorf("IsCacheableMethod(%q) = %v, want %v", tt.method, got, tt.expected)
			}
		})
	}
}

func TestCacheConfig_IsCacheableMethod_Empty(t *testing.T) {
	cfg := &CacheConfig{
		Enabled:          true,
		CacheableMethods: []string{},
	}

	if cfg.IsCacheableMethod("eth_chainId") {
		t.Error("Expected false for empty cacheable methods")
	}
}

// Benchmark tests
func BenchmarkLoad(b *testing.B) {
	content := `
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "https://eth.example.com"
        weight: 10
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 10
`
	tmpDir := b.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		b.Fatalf("Failed to write test config: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Load(configPath)
	}
}

func BenchmarkCacheConfig_IsCacheableMethod(b *testing.B) {
	cfg := &CacheConfig{
		CacheableMethods: []string{"eth_chainId", "net_version", "web3_clientVersion", "eth_blockNumber", "eth_gasPrice"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.IsCacheableMethod("eth_chainId")
		cfg.IsCacheableMethod("eth_call")
	}
}
