package chain

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

// Endpoint represents a single RPC endpoint
type Endpoint struct {
	URL            string
	Weight         int
	healthy        atomic.Bool
	blockHeight    atomic.Int64
	latency        atomic.Int64 // in microseconds
	totalReqs      atomic.Int64
	failedReqs     atomic.Int64
	mu             sync.RWMutex
	lastError      error
	lastChecked    time.Time
	client         *http.Client
	circuitBreaker *CircuitBreaker
}

// EndpointOptions holds options for creating an endpoint
type EndpointOptions struct {
	URL        string
	Weight     int
	Timeout    time.Duration
	TLSConfig  *tls.Config
	PoolConfig config.HTTPPoolConfig
}

// NewEndpoint creates a new endpoint
func NewEndpoint(url string, weight int, timeout time.Duration) *Endpoint {
	return NewEndpointWithOptions(EndpointOptions{
		URL:     url,
		Weight:  weight,
		Timeout: timeout,
		PoolConfig: config.HTTPPoolConfig{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	})
}

// NewEndpointWithOptions creates a new endpoint with full options
func NewEndpointWithOptions(opts EndpointOptions) *Endpoint {
	transport := &http.Transport{
		MaxIdleConns:        opts.PoolConfig.MaxIdleConns,
		MaxIdleConnsPerHost: opts.PoolConfig.MaxIdleConnsPerHost,
		MaxConnsPerHost:     opts.PoolConfig.MaxConnsPerHost,
		IdleConnTimeout:     opts.PoolConfig.IdleConnTimeout,
	}

	// Apply TLS config if provided
	if opts.TLSConfig != nil {
		transport.TLSClientConfig = opts.TLSConfig
	}

	e := &Endpoint{
		URL:    opts.URL,
		Weight: opts.Weight,
		client: &http.Client{
			Timeout:   opts.Timeout,
			Transport: transport,
		},
	}
	e.healthy.Store(true) // assume healthy initially
	return e
}

// NewEndpointWithCircuitBreaker creates a new endpoint with circuit breaker
func NewEndpointWithCircuitBreaker(url string, weight int, timeout time.Duration, cbConfig CircuitBreakerConfig) *Endpoint {
	e := NewEndpoint(url, weight, timeout)
	e.circuitBreaker = NewCircuitBreaker(cbConfig)
	return e
}

// CircuitBreakerState returns the current circuit breaker state, or empty if no circuit breaker
func (e *Endpoint) CircuitBreakerState() string {
	if e.circuitBreaker == nil {
		return ""
	}
	return e.circuitBreaker.State().String()
}

// CircuitBreakerStats returns circuit breaker stats, or nil if no circuit breaker
func (e *Endpoint) CircuitBreakerStats() *CircuitBreakerStats {
	if e.circuitBreaker == nil {
		return nil
	}
	stats := e.circuitBreaker.Stats()
	return &stats
}

// IsHealthy returns whether the endpoint is healthy
func (e *Endpoint) IsHealthy() bool {
	return e.healthy.Load()
}

// SetHealthy sets the health status
func (e *Endpoint) SetHealthy(healthy bool) {
	e.healthy.Store(healthy)
}

// BlockHeight returns the current block height
func (e *Endpoint) BlockHeight() int64 {
	return e.blockHeight.Load()
}

// SetBlockHeight sets the block height
func (e *Endpoint) SetBlockHeight(height int64) {
	e.blockHeight.Store(height)
}

// Latency returns the latency in milliseconds
func (e *Endpoint) Latency() float64 {
	return float64(e.latency.Load()) / 1000.0
}

// Stats returns endpoint statistics
func (e *Endpoint) Stats() EndpointStats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return EndpointStats{
		URL:         e.URL,
		Healthy:     e.healthy.Load(),
		BlockHeight: e.blockHeight.Load(),
		LatencyMs:   float64(e.latency.Load()) / 1000.0,
		TotalReqs:   e.totalReqs.Load(),
		FailedReqs:  e.failedReqs.Load(),
		LastError:   e.lastError,
		LastChecked: e.lastChecked,
	}
}

// EndpointStats holds endpoint statistics
type EndpointStats struct {
	URL         string
	Healthy     bool
	BlockHeight int64
	LatencyMs   float64
	TotalReqs   int64
	FailedReqs  int64
	LastError   error
	LastChecked time.Time
}

// RPCRequest represents a JSON-RPC request
type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params,omitempty"`
	ID      interface{}   `json:"id"`
}

// RPCResponse represents a JSON-RPC response
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      interface{}     `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// Call makes an RPC call to the endpoint
func (e *Endpoint) Call(ctx context.Context, req *RPCRequest) (*RPCResponse, error) {
	e.totalReqs.Add(1)

	body, err := json.Marshal(req)
	if err != nil {
		e.failedReqs.Add(1)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.URL, bytes.NewReader(body))
	if err != nil {
		e.failedReqs.Add(1)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := e.client.Do(httpReq)
	latency := time.Since(start)

	e.latency.Store(latency.Microseconds())

	if err != nil {
		e.failedReqs.Add(1)
		e.mu.Lock()
		e.lastError = err
		e.mu.Unlock()
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		e.failedReqs.Add(1)
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		e.mu.Lock()
		e.lastError = err
		e.mu.Unlock()
		return nil, err
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		e.failedReqs.Add(1)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		e.failedReqs.Add(1)
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &rpcResp, nil
}

// Forward forwards a raw request body to the endpoint and returns the raw response
func (e *Endpoint) Forward(ctx context.Context, body []byte) ([]byte, error) {
	// Check circuit breaker if configured
	if e.circuitBreaker != nil {
		if !e.circuitBreaker.Allow() {
			return nil, ErrCircuitOpen
		}
	}

	e.totalReqs.Add(1)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.URL, bytes.NewReader(body))
	if err != nil {
		e.failedReqs.Add(1)
		e.recordCircuitBreakerFailure()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := e.client.Do(httpReq)
	latency := time.Since(start)

	e.latency.Store(latency.Microseconds())

	if err != nil {
		e.failedReqs.Add(1)
		e.mu.Lock()
		e.lastError = err
		e.mu.Unlock()
		e.recordCircuitBreakerFailure()
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		e.failedReqs.Add(1)
		e.recordCircuitBreakerFailure()
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		e.failedReqs.Add(1)
		// Still return the body as it may contain error details
		// Don't record as circuit breaker failure for 4xx/5xx - these are valid responses
		e.recordCircuitBreakerSuccess()
		return respBody, nil
	}

	e.recordCircuitBreakerSuccess()
	return respBody, nil
}

// recordCircuitBreakerSuccess records a success if circuit breaker is configured
func (e *Endpoint) recordCircuitBreakerSuccess() {
	if e.circuitBreaker != nil {
		e.circuitBreaker.RecordSuccess()
	}
}

// recordCircuitBreakerFailure records a failure if circuit breaker is configured
func (e *Endpoint) recordCircuitBreakerFailure() {
	if e.circuitBreaker != nil {
		e.circuitBreaker.RecordFailure()
	}
}
