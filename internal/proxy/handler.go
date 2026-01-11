package proxy

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/bimakw/multichain-rpc-proxy/internal/cache"
	"github.com/bimakw/multichain-rpc-proxy/internal/chain"
	"github.com/bimakw/multichain-rpc-proxy/internal/metrics"
)

// Handler handles RPC proxy requests
type Handler struct {
	manager *chain.Manager
	cache   *cache.InMemoryCache
}

// NewHandler creates a new proxy handler
func NewHandler(manager *chain.Manager, c *cache.InMemoryCache) *Handler {
	return &Handler{
		manager: manager,
		cache:   c,
	}
}

// rpcRequest represents a JSON-RPC request
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id"`
}

// rpcError represents a JSON-RPC error response
type rpcError struct {
	JSONRPC string `json:"jsonrpc"`
	Error   struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	ID interface{} `json:"id"`
}

// HandleRPC handles RPC requests for a specific chain
// Route: POST /:chain
func (h *Handler) HandleRPC(c *fiber.Ctx) error {
	chainName := c.Params("chain")
	start := time.Now()

	ch, ok := h.manager.GetChain(chainName)
	if !ok {
		return c.Status(404).JSON(fiber.Map{
			"error": "chain not found",
			"chain": chainName,
		})
	}

	body := c.Body()

	// Parse request to get method for metrics/caching
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		metrics.RecordError(chainName, "parse_error")
		return c.Status(400).JSON(rpcError{
			JSONRPC: "2.0",
			Error: struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{
				Code:    -32700,
				Message: "Parse error",
			},
			ID: nil,
		})
	}

	metrics.RecordRequest(chainName, req.Method)

	// Check cache for cacheable methods
	if h.cache.IsCacheable(req.Method) {
		if cached, ok := h.cache.Get(chainName, req.Method, req.Params); ok {
			metrics.RecordCacheHit(chainName, req.Method)
			metrics.RecordRequestDuration(chainName, req.Method, "cache_hit", time.Since(start).Seconds())
			c.Set("X-Cache", "HIT")
			return c.Send(cached)
		}
		metrics.RecordCacheMiss(chainName, req.Method)
	}

	// Forward request to chain
	resp, err := ch.ForwardWithRetry(c.Context(), body, 2)
	if err != nil {
		log.Printf("[%s] Forward error: %v", chainName, err)
		metrics.RecordError(chainName, "forward_error")
		metrics.RecordRequestDuration(chainName, req.Method, "error", time.Since(start).Seconds())
		return c.Status(502).JSON(rpcError{
			JSONRPC: "2.0",
			Error: struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{
				Code:    -32603,
				Message: "Internal error: " + err.Error(),
			},
			ID: req.ID,
		})
	}

	// Cache response if cacheable
	if h.cache.IsCacheable(req.Method) {
		h.cache.Set(chainName, req.Method, req.Params, resp)
	}

	metrics.RecordRequestDuration(chainName, req.Method, "success", time.Since(start).Seconds())
	c.Set("X-Cache", "MISS")
	c.Set("Content-Type", "application/json")
	return c.Send(resp)
}

// HandleHealth handles health check endpoint
// Route: GET /health
func (h *Handler) HandleHealth(c *fiber.Ctx) error {
	stats := h.manager.AllStats()

	healthy := true
	chains := make(map[string]interface{})

	for name, s := range stats {
		chainHealthy := s.HealthyEndpoints > 0
		if !chainHealthy {
			healthy = false
		}

		chains[name] = fiber.Map{
			"healthy":           chainHealthy,
			"healthy_endpoints": s.HealthyEndpoints,
			"total_endpoints":   s.TotalEndpoints,
			"block_height":      s.HighestBlock,
		}

		// Update metrics
		metrics.RecordHealthyEndpoints(name, s.HealthyEndpoints)
		metrics.RecordTotalEndpoints(name, s.TotalEndpoints)
		metrics.RecordBlockHeight(name, s.HighestBlock)
	}

	status := 200
	if !healthy {
		status = 503
	}

	return c.Status(status).JSON(fiber.Map{
		"status": func() string {
			if healthy {
				return "healthy"
			}
			return "degraded"
		}(),
		"chains": chains,
	})
}

// HandleChainHealth handles health check for a specific chain
// Route: GET /health/:chain
func (h *Handler) HandleChainHealth(c *fiber.Ctx) error {
	chainName := c.Params("chain")

	ch, ok := h.manager.GetChain(chainName)
	if !ok {
		return c.Status(404).JSON(fiber.Map{
			"error": "chain not found",
		})
	}

	stats := ch.Stats()
	healthy := stats.HealthyEndpoints > 0

	endpoints := make([]fiber.Map, 0, len(stats.Endpoints))
	for _, ep := range stats.Endpoints {
		endpoints = append(endpoints, fiber.Map{
			"url":          ep.URL,
			"healthy":      ep.Healthy,
			"block_height": ep.BlockHeight,
			"latency_ms":   ep.LatencyMs,
			"total_reqs":   ep.TotalReqs,
			"failed_reqs":  ep.FailedReqs,
		})

		// Update metrics
		metrics.RecordEndpointHealth(chainName, ep.URL, ep.Healthy)
		metrics.RecordEndpointBlockHeight(chainName, ep.URL, ep.BlockHeight)
		metrics.RecordEndpointLatency(chainName, ep.URL, ep.LatencyMs)
	}

	status := 200
	if !healthy {
		status = 503
	}

	return c.Status(status).JSON(fiber.Map{
		"chain":             chainName,
		"chain_id":          stats.ChainID,
		"healthy":           healthy,
		"highest_block":     stats.HighestBlock,
		"healthy_endpoints": stats.HealthyEndpoints,
		"total_endpoints":   stats.TotalEndpoints,
		"endpoints":         endpoints,
	})
}

// HandleChains lists all available chains
// Route: GET /chains
func (h *Handler) HandleChains(c *fiber.Ctx) error {
	stats := h.manager.AllStats()

	chains := make([]fiber.Map, 0, len(stats))
	for name, s := range stats {
		chains = append(chains, fiber.Map{
			"name":              name,
			"chain_id":          s.ChainID,
			"healthy":           s.HealthyEndpoints > 0,
			"healthy_endpoints": s.HealthyEndpoints,
			"total_endpoints":   s.TotalEndpoints,
			"block_height":      s.HighestBlock,
			"rpc_url":           "/" + name,
		})
	}

	return c.JSON(fiber.Map{
		"chains": chains,
	})
}
