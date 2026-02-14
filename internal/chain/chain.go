package chain

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

type Chain struct {
	Name          string
	ChainID       int
	endpoints     []*Endpoint
	loadBalancer  *LoadBalancer
	healthChecker *HealthChecker
	highestBlock  atomic.Int64
}

type ChainOptions struct {
	Name      string
	Config    *config.ChainConfig
	TLSConfig *tls.Config
}

func NewChain(name string, cfg *config.ChainConfig) *Chain {
	return NewChainWithOptions(ChainOptions{
		Name:   name,
		Config: cfg,
	})
}

// NewChainWithOptions creates a new chain with full options including TLS
func NewChainWithOptions(opts ChainOptions) *Chain {
	cfg := opts.Config
	endpoints := make([]*Endpoint, 0, len(cfg.Endpoints))

	for _, epCfg := range cfg.Endpoints {
		epOpts := EndpointOptions{
			URL:        epCfg.URL,
			Weight:     epCfg.Weight,
			Timeout:    cfg.HealthCheck.Timeout,
			TLSConfig:  opts.TLSConfig,
			PoolConfig: cfg.HTTPPool,
		}

		ep := NewEndpointWithOptions(epOpts)

		if cfg.CircuitBreaker.Enabled {
			cbConfig := CircuitBreakerConfig{
				FailureThreshold:    cfg.CircuitBreaker.FailureThreshold,
				SuccessThreshold:    cfg.CircuitBreaker.SuccessThreshold,
				Timeout:             cfg.CircuitBreaker.Timeout,
				HalfOpenMaxRequests: cfg.CircuitBreaker.HalfOpenMaxRequests,
			}
			if cbConfig.FailureThreshold == 0 {
				cbConfig.FailureThreshold = 5
			}
			if cbConfig.SuccessThreshold == 0 {
				cbConfig.SuccessThreshold = 2
			}
			if cbConfig.Timeout == 0 {
				cbConfig.Timeout = 30 * time.Second
			}
			if cbConfig.HalfOpenMaxRequests == 0 {
				cbConfig.HalfOpenMaxRequests = 3
			}
			ep.circuitBreaker = NewCircuitBreaker(cbConfig)
		}
		endpoints = append(endpoints, ep)
	}

	chain := &Chain{
		Name:         opts.Name,
		ChainID:      cfg.ChainID,
		endpoints:    endpoints,
		loadBalancer: NewLoadBalancer(endpoints),
	}

	chain.healthChecker = NewHealthChecker(chain, cfg.HealthCheck)

	return chain
}

func (c *Chain) Start() {
	log.Printf("Starting chain %s (chainId: %d) with %d endpoints", c.Name, c.ChainID, len(c.endpoints))
	c.healthChecker.Start()
}

func (c *Chain) Stop() {
	log.Printf("Stopping chain %s", c.Name)
	c.healthChecker.Stop()
}

func (c *Chain) Forward(ctx context.Context, body []byte) ([]byte, error) {
	ep, err := c.loadBalancer.Next()
	if err != nil {
		return nil, fmt.Errorf("chain %s: %w", c.Name, err)
	}

	return ep.Forward(ctx, body)
}

func (c *Chain) ForwardWithRetry(ctx context.Context, body []byte, maxRetries int) ([]byte, error) {
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		ep, err := c.loadBalancer.Next()
		if err != nil {
			return nil, fmt.Errorf("chain %s: %w", c.Name, err)
		}

		resp, err := ep.Forward(ctx, body)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		log.Printf("[%s] Request to %s failed (attempt %d/%d): %v", c.Name, ep.URL, i+1, maxRetries+1, err)

		ep.SetHealthy(false)

		if i < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	return nil, fmt.Errorf("all retries failed: %w", lastErr)
}

func (c *Chain) UpdateHighestBlock(height int64) {
	c.highestBlock.Store(height)
}

func (c *Chain) HighestBlock() int64 {
	return c.highestBlock.Load()
}

func (c *Chain) HealthyEndpoints() int {
	return c.loadBalancer.HealthyCount()
}

func (c *Chain) TotalEndpoints() int {
	return len(c.endpoints)
}

func (c *Chain) Stats() ChainStats {
	endpoints := make([]EndpointStats, 0, len(c.endpoints))
	for _, ep := range c.endpoints {
		endpoints = append(endpoints, ep.Stats())
	}

	return ChainStats{
		Name:             c.Name,
		ChainID:          c.ChainID,
		HighestBlock:     c.highestBlock.Load(),
		HealthyEndpoints: c.HealthyEndpoints(),
		TotalEndpoints:   len(c.endpoints),
		Endpoints:        endpoints,
	}
}

type ChainStats struct {
	Name             string
	ChainID          int
	HighestBlock     int64
	HealthyEndpoints int
	TotalEndpoints   int
	Endpoints        []EndpointStats
}

type Manager struct {
	chains map[string]*Chain
	mu     sync.RWMutex
}

type ManagerOptions struct {
	Chains    map[string]*config.ChainConfig
	TLSConfig *tls.Config
}

func NewManager(cfg map[string]*config.ChainConfig) *Manager {
	return NewManagerWithOptions(ManagerOptions{
		Chains: cfg,
	})
}

func NewManagerWithOptions(opts ManagerOptions) *Manager {
	m := &Manager{
		chains: make(map[string]*Chain),
	}

	for name, chainCfg := range opts.Chains {
		m.chains[name] = NewChainWithOptions(ChainOptions{
			Name:      name,
			Config:    chainCfg,
			TLSConfig: opts.TLSConfig,
		})
	}

	return m
}

func (m *Manager) Start() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, chain := range m.chains {
		chain.Start()
	}
}

func (m *Manager) Stop() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, chain := range m.chains {
		chain.Stop()
	}
}

func (m *Manager) GetChain(name string) (*Chain, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chain, ok := m.chains[name]
	return chain, ok
}

// GetChainByID returns a chain by chain ID
func (m *Manager) GetChainByID(chainID int) (*Chain, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, chain := range m.chains {
		if chain.ChainID == chainID {
			return chain, true
		}
	}
	return nil, false
}

func (m *Manager) ListChains() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.chains))
	for name := range m.chains {
		names = append(names, name)
	}
	return names
}

func (m *Manager) AllStats() map[string]ChainStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]ChainStats)
	for name, chain := range m.chains {
		stats[name] = chain.Stats()
	}
	return stats
}
