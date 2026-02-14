package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestTokenBucketLimiter_Allow(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 5) // 10 req/s, burst of 5
	defer limiter.Stop()

	key := "test-key"

	// Should allow burst of 5
	for i := 0; i < 5; i++ {
		if !limiter.Allow(key) {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 6th request should be rejected (burst exhausted)
	if limiter.Allow(key) {
		t.Error("Expected 6th request to be rejected")
	}

	// Wait for tokens to refill
	time.Sleep(200 * time.Millisecond)

	// Should have ~2 tokens now (10 req/s * 0.2s = 2)
	if !limiter.Allow(key) {
		t.Error("Expected request after wait to be allowed")
	}
}

func TestTokenBucketLimiter_AllowN(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 10) // 10 req/s, burst of 10
	defer limiter.Stop()

	key := "test-key"

	// Should allow 5 requests at once
	if !limiter.AllowN(key, 5) {
		t.Error("Expected 5 requests to be allowed")
	}

	// Should still have 5 tokens
	if !limiter.AllowN(key, 5) {
		t.Error("Expected another 5 requests to be allowed")
	}

	// No more tokens
	if limiter.AllowN(key, 1) {
		t.Error("Expected request to be rejected")
	}
}

func TestTokenBucketLimiter_DifferentKeys(t *testing.T) {
	limiter := NewTokenBucketLimiter(10, 2)
	defer limiter.Stop()

	// Different keys should have separate buckets
	if !limiter.Allow("key1") {
		t.Error("Expected key1 request 1 to be allowed")
	}
	if !limiter.Allow("key1") {
		t.Error("Expected key1 request 2 to be allowed")
	}
	if limiter.Allow("key1") {
		t.Error("Expected key1 request 3 to be rejected")
	}

	// key2 should still have tokens
	if !limiter.Allow("key2") {
		t.Error("Expected key2 request 1 to be allowed")
	}
	if !limiter.Allow("key2") {
		t.Error("Expected key2 request 2 to be allowed")
	}
}

func TestSlidingWindowLimiter_Allow(t *testing.T) {
	limiter := NewSlidingWindowLimiter(10, 1) // 10 req/s, 1 second window
	defer limiter.Stop()

	key := "test-key"

	// Should allow 10 requests
	for i := 0; i < 10; i++ {
		if !limiter.Allow(key) {
			t.Errorf("Expected request %d to be allowed", i+1)
		}
	}

	// 11th request should be rejected
	if limiter.Allow(key) {
		t.Error("Expected 11th request to be rejected")
	}
}

func TestSlidingWindowLimiter_WindowRotation(t *testing.T) {
	limiter := NewSlidingWindowLimiter(5, 1) // 5 req/s, 1 second window
	defer limiter.Stop()

	key := "test-key"

	// Use up all requests
	for i := 0; i < 5; i++ {
		limiter.Allow(key)
	}

	// Wait for window to fully rotate and decay enough to allow new requests
	// With rate=5 and prevCount=5, we need weight <= 0.8 to allow 1 request
	// weight = 1.0 - (elapsed / windowSize), so elapsed >= 200ms into new window
	// Wait 1500ms (1000ms + 500ms) to ensure weight = 0.5, allowing 2-3 requests
	time.Sleep(1500 * time.Millisecond)

	// Should allow some requests now (sliding window effect)
	// With 500ms elapsed in new window, weight = 0.5, weightedCount = 2.5
	// So we should be able to allow 2 requests (2.5 + 2 = 4.5 <= 5)
	allowed := 0
	for i := 0; i < 5; i++ {
		if limiter.Allow(key) {
			allowed++
		}
	}

	// Should have allowed at least 1 request (previous window's count decays)
	if allowed == 0 {
		t.Errorf("Expected some allowance after window rotation, got %d", allowed)
	}
}

func TestMultiLimiter_Allow(t *testing.T) {
	global := NewTokenBucketLimiter(100, 100) // High global limit
	defer global.Stop()

	multi := NewMultiLimiter(global)

	chainLimiter := NewTokenBucketLimiter(2, 2) // Low chain limit
	multi.SetChainLimiter("ethereum", chainLimiter)

	key := "test-ip"

	// Should allow 2 requests (limited by chain)
	if !multi.Allow("ethereum", key) {
		t.Error("Expected request 1 to be allowed")
	}
	if !multi.Allow("ethereum", key) {
		t.Error("Expected request 2 to be allowed")
	}
	if multi.Allow("ethereum", key) {
		t.Error("Expected request 3 to be rejected")
	}

	// Different chain without limit should still work
	if !multi.Allow("polygon", key) {
		t.Error("Expected polygon request to be allowed")
	}
}

func TestTokenBucketLimiter_Concurrent(t *testing.T) {
	limiter := NewTokenBucketLimiter(1000, 100)
	defer limiter.Stop()

	var wg sync.WaitGroup
	allowed := make(chan bool, 1000)

	// Spawn 100 goroutines, each making 10 requests
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				result := limiter.Allow("concurrent-key")
				allowed <- result
			}
		}()
	}

	wg.Wait()
	close(allowed)

	allowedCount := 0
	for result := range allowed {
		if result {
			allowedCount++
		}
	}

	// Should have allowed around 100 (burst size)
	if allowedCount < 50 || allowedCount > 150 {
		t.Errorf("Expected around 100 allowed requests, got %d", allowedCount)
	}
}

func TestNewLimiter(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected string
	}{
		{
			name: "disabled",
			cfg: Config{
				Enabled: false,
			},
			expected: "nil",
		},
		{
			name: "token_bucket",
			cfg: Config{
				Enabled:           true,
				RequestsPerSecond: 10,
				Burst:             5,
				Strategy:          StrategyTokenBucket,
			},
			expected: "TokenBucketLimiter",
		},
		{
			name: "sliding_window",
			cfg: Config{
				Enabled:           true,
				RequestsPerSecond: 10,
				Burst:             5,
				Strategy:          StrategySlidingWindow,
			},
			expected: "SlidingWindowLimiter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewLimiter(tt.cfg)
			if tt.expected == "nil" {
				if limiter != nil {
					t.Error("Expected nil limiter")
				}
				return
			}

			if limiter == nil {
				t.Error("Expected non-nil limiter")
				return
			}

			switch tt.expected {
			case "TokenBucketLimiter":
				if _, ok := limiter.(*TokenBucketLimiter); !ok {
					t.Error("Expected TokenBucketLimiter")
				} else {
					limiter.(*TokenBucketLimiter).Stop()
				}
			case "SlidingWindowLimiter":
				if _, ok := limiter.(*SlidingWindowLimiter); !ok {
					t.Error("Expected SlidingWindowLimiter")
				} else {
					limiter.(*SlidingWindowLimiter).Stop()
				}
			}
		})
	}
}
