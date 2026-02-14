package chain

import (
	"context"
	"encoding/hex"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

type HealthChecker struct {
	chain       *Chain
	cfg         config.HealthCheckConfig
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	maxBlockLag int64
}

func NewHealthChecker(chain *Chain, cfg config.HealthCheckConfig) *HealthChecker {
	ctx, cancel := context.WithCancel(context.Background())
	return &HealthChecker{
		chain:       chain,
		cfg:         cfg,
		ctx:         ctx,
		cancel:      cancel,
		maxBlockLag: int64(cfg.MaxBlockLag),
	}
}

func (h *HealthChecker) Start() {
	h.wg.Add(1)
	go h.run()
}

func (h *HealthChecker) Stop() {
	h.cancel()
	h.wg.Wait()
}

func (h *HealthChecker) run() {
	defer h.wg.Done()

	h.checkAll()

	ticker := time.NewTicker(h.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.checkAll()
		}
	}
}

func (h *HealthChecker) checkAll() {
	var wg sync.WaitGroup
	results := make(chan struct {
		endpoint *Endpoint
		height   int64
		healthy  bool
	}, len(h.chain.endpoints))

	for _, ep := range h.chain.endpoints {
		wg.Add(1)
		go func(ep *Endpoint) {
			defer wg.Done()
			height, healthy := h.checkEndpoint(ep)
			results <- struct {
				endpoint *Endpoint
				height   int64
				healthy  bool
			}{ep, height, healthy}
		}(ep)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var maxHeight int64
	heights := make(map[*Endpoint]int64)

	for r := range results {
		heights[r.endpoint] = r.height
		if r.height > maxHeight {
			maxHeight = r.height
		}
	}

	for ep, height := range heights {
		if height == 0 {
			ep.SetHealthy(false)
			continue
		}

		lag := maxHeight - height
		if lag > h.maxBlockLag {
			log.Printf("[%s] %s unhealthy: block lag %d (max: %d)", h.chain.Name, ep.URL, lag, h.maxBlockLag)
			ep.SetHealthy(false)
		} else {
			ep.SetHealthy(true)
		}
		ep.SetBlockHeight(height)
	}

	h.chain.UpdateHighestBlock(maxHeight)
}

func (h *HealthChecker) checkEndpoint(ep *Endpoint) (int64, bool) {
	ctx, cancel := context.WithTimeout(h.ctx, h.cfg.Timeout)
	defer cancel()

	req := &RPCRequest{
		JSONRPC: "2.0",
		Method:  "eth_blockNumber",
		ID:      1,
	}

	resp, err := ep.Call(ctx, req)
	if err != nil {
		log.Printf("[%s] Health check failed for %s: %v", h.chain.Name, ep.URL, err)
		return 0, false
	}

	if resp.Error != nil {
		log.Printf("[%s] RPC error from %s: %s", h.chain.Name, ep.URL, resp.Error.Message)
		return 0, false
	}

	height, err := parseBlockNumber(resp.Result)
	if err != nil {
		log.Printf("[%s] Failed to parse block number from %s: %v", h.chain.Name, ep.URL, err)
		return 0, false
	}

	return height, true
}

// parseBlockNumber parses a hex block number from JSON-RPC response
func parseBlockNumber(raw []byte) (int64, error) {
	hexStr := strings.Trim(string(raw), "\"")

	hexStr = strings.TrimPrefix(hexStr, "0x")

	if hexStr == "" {
		return 0, nil
	}

	if len(hexStr)%2 != 0 {
		hexStr = "0" + hexStr
	}

	bytes, err := hex.DecodeString(hexStr)
	if err != nil {
		return strconv.ParseInt(string(raw), 10, 64)
	}

	var height int64
	for _, b := range bytes {
		height = height<<8 | int64(b)
	}

	return height, nil
}
