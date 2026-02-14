package ratelimit

import (
	"sync"
	"time"
)

// Strategy defines the rate limiting strategy
type Strategy string

const (
	StrategyTokenBucket   Strategy = "token_bucket"
	StrategySlidingWindow Strategy = "sliding_window"
)

// Config holds rate limiter configuration
type Config struct {
	Enabled           bool
	RequestsPerSecond int
	Burst             int
	Strategy          Strategy
	PerIP             bool
}

// Limiter is the interface for rate limiters
type Limiter interface {
	Allow(key string) bool
	AllowN(key string, n int) bool
}

// TokenBucketLimiter implements token bucket algorithm
type TokenBucketLimiter struct {
	rate        float64
	burst       int
	buckets     map[string]*tokenBucket
	mu          sync.RWMutex
	cleanupTick time.Duration
	stopCleanup chan struct{}
}

type tokenBucket struct {
	tokens    float64
	lastCheck time.Time
}

// NewTokenBucketLimiter creates a new token bucket limiter
func NewTokenBucketLimiter(rate float64, burst int) *TokenBucketLimiter {
	l := &TokenBucketLimiter{
		rate:        rate,
		burst:       burst,
		buckets:     make(map[string]*tokenBucket),
		cleanupTick: 5 * time.Minute,
		stopCleanup: make(chan struct{}),
	}
	go l.cleanup()
	return l
}

// Allow checks if a single request is allowed
func (l *TokenBucketLimiter) Allow(key string) bool {
	return l.AllowN(key, 1)
}

// AllowN checks if n requests are allowed
func (l *TokenBucketLimiter) AllowN(key string, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &tokenBucket{
			tokens:    float64(l.burst),
			lastCheck: now,
		}
		l.buckets[key] = b
	}

	// Add tokens based on time elapsed
	elapsed := now.Sub(b.lastCheck).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > float64(l.burst) {
		b.tokens = float64(l.burst)
	}
	b.lastCheck = now

	// Check if we have enough tokens
	if b.tokens >= float64(n) {
		b.tokens -= float64(n)
		return true
	}

	return false
}

func (l *TokenBucketLimiter) cleanup() {
	ticker := time.NewTicker(l.cleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			for key, b := range l.buckets {
				if now.Sub(b.lastCheck) > 10*time.Minute {
					delete(l.buckets, key)
				}
			}
			l.mu.Unlock()
		case <-l.stopCleanup:
			return
		}
	}
}

// Stop stops the cleanup goroutine
func (l *TokenBucketLimiter) Stop() {
	close(l.stopCleanup)
}

// SlidingWindowLimiter implements sliding window algorithm
type SlidingWindowLimiter struct {
	rate        int           // requests per window
	windowSize  time.Duration // window duration
	windows     map[string]*slidingWindow
	mu          sync.RWMutex
	cleanupTick time.Duration
	stopCleanup chan struct{}
}

type slidingWindow struct {
	prevCount  int
	currCount  int
	prevWindow time.Time
	currWindow time.Time
	windowSize time.Duration
}

// NewSlidingWindowLimiter creates a new sliding window limiter
func NewSlidingWindowLimiter(requestsPerSecond int, windowSeconds int) *SlidingWindowLimiter {
	windowSize := time.Duration(windowSeconds) * time.Second
	l := &SlidingWindowLimiter{
		rate:        requestsPerSecond * windowSeconds,
		windowSize:  windowSize,
		windows:     make(map[string]*slidingWindow),
		cleanupTick: 5 * time.Minute,
		stopCleanup: make(chan struct{}),
	}
	go l.cleanup()
	return l
}

// Allow checks if a single request is allowed
func (l *SlidingWindowLimiter) Allow(key string) bool {
	return l.AllowN(key, 1)
}

// AllowN checks if n requests are allowed
func (l *SlidingWindowLimiter) AllowN(key string, n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	w, ok := l.windows[key]
	if !ok {
		w = &slidingWindow{
			currWindow: now.Truncate(l.windowSize),
			windowSize: l.windowSize,
		}
		l.windows[key] = w
	}

	currWindowStart := now.Truncate(l.windowSize)

	// Rotate windows if needed
	if currWindowStart.After(w.currWindow) {
		timeDiff := currWindowStart.Sub(w.currWindow)
		if timeDiff >= 2*l.windowSize {
			// More than 2 windows have passed, reset everything
			w.prevCount = 0
			w.currCount = 0
		} else if timeDiff >= l.windowSize {
			// One window has passed
			w.prevCount = w.currCount
			w.currCount = 0
		}
		w.prevWindow = w.currWindow
		w.currWindow = currWindowStart
	}

	// Calculate weighted count using sliding window
	elapsed := now.Sub(currWindowStart)
	weight := 1.0 - (float64(elapsed) / float64(l.windowSize))
	weightedCount := float64(w.prevCount)*weight + float64(w.currCount)

	if weightedCount+float64(n) <= float64(l.rate) {
		w.currCount += n
		return true
	}

	return false
}

func (l *SlidingWindowLimiter) cleanup() {
	ticker := time.NewTicker(l.cleanupTick)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			for key, w := range l.windows {
				if now.Sub(w.currWindow) > 10*time.Minute {
					delete(l.windows, key)
				}
			}
			l.mu.Unlock()
		case <-l.stopCleanup:
			return
		}
	}
}

// Stop stops the cleanup goroutine
func (l *SlidingWindowLimiter) Stop() {
	close(l.stopCleanup)
}

// MultiLimiter combines multiple limiters (global + per-chain)
type MultiLimiter struct {
	global   Limiter
	perChain map[string]Limiter
	mu       sync.RWMutex
}

// NewMultiLimiter creates a new multi-limiter
func NewMultiLimiter(global Limiter) *MultiLimiter {
	return &MultiLimiter{
		global:   global,
		perChain: make(map[string]Limiter),
	}
}

// SetChainLimiter sets a limiter for a specific chain
func (m *MultiLimiter) SetChainLimiter(chain string, limiter Limiter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.perChain[chain] = limiter
}

// Allow checks if a request is allowed for the given chain and key
func (m *MultiLimiter) Allow(chain, key string) bool {
	// Check global limit first
	if m.global != nil && !m.global.Allow(key) {
		return false
	}

	// Check per-chain limit
	m.mu.RLock()
	chainLimiter, ok := m.perChain[chain]
	m.mu.RUnlock()

	if ok && !chainLimiter.Allow(key) {
		return false
	}

	return true
}

// NewLimiter creates a limiter based on config
func NewLimiter(cfg Config) Limiter {
	if !cfg.Enabled {
		return nil
	}

	switch cfg.Strategy {
	case StrategySlidingWindow:
		return NewSlidingWindowLimiter(cfg.RequestsPerSecond, 1) // 1 second window
	default:
		return NewTokenBucketLimiter(float64(cfg.RequestsPerSecond), cfg.Burst)
	}
}
