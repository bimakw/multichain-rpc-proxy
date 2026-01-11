package proxy

import (
	"io"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

func TestNewRateLimiter(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 100,
		Burst:             200,
	}

	rl := NewRateLimiter(cfg)

	if !rl.enabled {
		t.Error("Expected rate limiter to be enabled")
	}
	if rl.rate != 100 {
		t.Errorf("Expected rate 100, got %f", rl.rate)
	}
	if rl.burst != 200 {
		t.Errorf("Expected burst 200, got %d", rl.burst)
	}
}

func TestNewRateLimiter_Disabled(t *testing.T) {
	cfg := config.RateLimitConfig{
		Enabled: false,
	}

	rl := NewRateLimiter(cfg)

	if rl.enabled {
		t.Error("Expected rate limiter to be disabled")
	}
}

func TestRateLimiter_Allow_Disabled(t *testing.T) {
	rl := &RateLimiter{enabled: false}

	// Should always allow when disabled
	for i := 0; i < 1000; i++ {
		if !rl.Allow("test-key") {
			t.Error("Expected allow when disabled")
		}
	}
}

func TestRateLimiter_Allow_BurstLimit(t *testing.T) {
	rl := &RateLimiter{
		enabled: true,
		rate:    10,  // 10 per second
		burst:   5,   // burst of 5
		buckets: make(map[string]*bucket),
	}

	// Should allow up to burst
	for i := 0; i < 5; i++ {
		if !rl.Allow("test-key") {
			t.Errorf("Expected allow for request %d within burst", i)
		}
	}

	// Next request should be denied
	if rl.Allow("test-key") {
		t.Error("Expected deny after burst exhausted")
	}
}

func TestRateLimiter_Allow_TokenRefill(t *testing.T) {
	rl := &RateLimiter{
		enabled: true,
		rate:    100, // 100 per second
		burst:   10,
		buckets: make(map[string]*bucket),
	}

	// Exhaust burst
	for i := 0; i < 10; i++ {
		rl.Allow("test-key")
	}

	// Should be denied
	if rl.Allow("test-key") {
		t.Error("Expected deny after burst exhausted")
	}

	// Wait for token refill
	time.Sleep(100 * time.Millisecond)

	// Should be allowed now (10 new tokens added)
	if !rl.Allow("test-key") {
		t.Error("Expected allow after token refill")
	}
}

func TestRateLimiter_Allow_DifferentKeys(t *testing.T) {
	rl := &RateLimiter{
		enabled: true,
		rate:    10,
		burst:   2,
		buckets: make(map[string]*bucket),
	}

	// Exhaust key1
	for i := 0; i < 2; i++ {
		rl.Allow("key1")
	}

	// key1 should be denied
	if rl.Allow("key1") {
		t.Error("Expected deny for key1")
	}

	// key2 should still be allowed
	if !rl.Allow("key2") {
		t.Error("Expected allow for key2")
	}
}

func TestRateLimiter_Allow_Concurrent(t *testing.T) {
	rl := &RateLimiter{
		enabled: true,
		rate:    1000,
		burst:   100,
		buckets: make(map[string]*bucket),
	}

	var wg sync.WaitGroup
	iterations := 50

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.Allow("concurrent-key")
		}()
	}

	wg.Wait()
}

func TestRateLimiter_Middleware(t *testing.T) {
	rl := &RateLimiter{
		enabled: true,
		rate:    10,
		burst:   2,
		buckets: make(map[string]*bucket),
	}

	app := fiber.New()
	app.Use(rl.Middleware())
	app.Post("/:chain", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/ethereum", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	}

	// Third request should be rate limited
	req := httptest.NewRequest("POST", "/ethereum", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode != 429 {
		t.Errorf("Expected 429, got %d", resp.StatusCode)
	}
}

func TestRateLimiter_Middleware_Disabled(t *testing.T) {
	rl := &RateLimiter{enabled: false}

	app := fiber.New()
	app.Use(rl.Middleware())
	app.Post("/:chain", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// All requests should succeed
	for i := 0; i < 100; i++ {
		req := httptest.NewRequest("POST", "/ethereum", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	}
}

func TestCORSMiddleware(t *testing.T) {
	app := fiber.New()
	app.Use(CORSMiddleware())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// Regular request
	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Expected Access-Control-Allow-Origin: *")
	}
	if resp.Header.Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
		t.Error("Expected Access-Control-Allow-Methods header")
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	app := fiber.New()
	app.Use(CORSMiddleware())
	app.Post("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	// OPTIONS preflight request
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 204 {
		t.Errorf("Expected 204 for OPTIONS, got %d", resp.StatusCode)
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	app := fiber.New()
	app.Use(RecoveryMiddleware())
	app.Get("/:chain", func(c *fiber.Ctx) error {
		panic("test panic")
	})

	req := httptest.NewRequest("GET", "/ethereum", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 500 {
		t.Errorf("Expected 500 after panic, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Error("Expected error response body")
	}
}

func TestLoggingMiddleware(t *testing.T) {
	app := fiber.New()
	app.Use(LoggingMiddleware())
	app.Get("/test", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

// Benchmark tests
func BenchmarkRateLimiter_Allow(b *testing.B) {
	rl := &RateLimiter{
		enabled: true,
		rate:    10000,
		burst:   10000,
		buckets: make(map[string]*bucket),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rl.Allow("test-key")
	}
}

func BenchmarkRateLimiter_Allow_Parallel(b *testing.B) {
	rl := &RateLimiter{
		enabled: true,
		rate:    100000,
		burst:   100000,
		buckets: make(map[string]*bucket),
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			rl.Allow("test-key")
		}
	})
}
