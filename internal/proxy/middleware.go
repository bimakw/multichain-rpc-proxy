package proxy

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
	"github.com/bimakw/multichain-rpc-proxy/internal/metrics"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	enabled     bool
	rate        float64
	burst       int
	buckets     map[string]*bucket
	mu          sync.RWMutex
	cleanupTick time.Duration
}

type bucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg config.RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		enabled:     cfg.Enabled,
		rate:        float64(cfg.RequestsPerSecond),
		burst:       cfg.Burst,
		buckets:     make(map[string]*bucket),
		cleanupTick: 5 * time.Minute,
	}

	if rl.enabled {
		go rl.cleanup()
	}

	return rl
}

// Allow checks if a request is allowed for the given key
func (rl *RateLimiter) Allow(key string) bool {
	if !rl.enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{
			tokens:    float64(rl.burst),
			lastCheck: now,
		}
		rl.buckets[key] = b
	}

	// Add tokens based on time elapsed
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastCheck = now

	// Check if we have tokens
	if b.tokens >= 1 {
		b.tokens--
		return true
	}

	return false
}

// cleanup removes old buckets periodically
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanupTick)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, b := range rl.buckets {
			if now.Sub(b.lastCheck) > 10*time.Minute {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware returns a Fiber middleware for rate limiting
func (rl *RateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !rl.enabled {
			return c.Next()
		}

		// Use IP as key, could also use API key
		key := c.IP()

		if !rl.Allow(key) {
			chainName := c.Params("chain")
			if chainName != "" {
				metrics.RecordRateLimitReject(chainName)
			}

			return c.Status(429).JSON(fiber.Map{
				"jsonrpc": "2.0",
				"error": fiber.Map{
					"code":    -32005,
					"message": "Rate limit exceeded",
				},
				"id": nil,
			})
		}

		return c.Next()
	}
}

// LoggingMiddleware logs requests
func LoggingMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		duration := time.Since(start)
		status := c.Response().StatusCode()

		// Only log non-health endpoints
		if c.Path() != "/health" && c.Path() != "/metrics" {
			// Using structured logging format
			_ = duration
			_ = status
			// log.Printf("method=%s path=%s status=%d duration=%s ip=%s",
			// 	c.Method(), c.Path(), status, duration, c.IP())
		}

		return err
	}
}

// CORSMiddleware adds CORS headers
func CORSMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Access-Control-Allow-Origin", "*")
		c.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Method() == "OPTIONS" {
			return c.SendStatus(204)
		}

		return c.Next()
	}
}

// RecoveryMiddleware recovers from panics
func RecoveryMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				chainName := c.Params("chain")
				if chainName != "" {
					metrics.RecordError(chainName, "panic")
				}
				_ = c.Status(500).JSON(fiber.Map{
					"jsonrpc": "2.0",
					"error": fiber.Map{
						"code":    -32603,
						"message": "Internal error",
					},
					"id": nil,
				})
			}
		}()
		return c.Next()
	}
}
