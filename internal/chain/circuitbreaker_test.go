package chain

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(cfg)

	if cb.State() != StateClosed {
		t.Errorf("Expected initial state Closed, got %v", cb.State())
	}
}

func TestCircuitBreaker_Allow_Closed(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	// Should always allow in closed state
	for i := 0; i < 100; i++ {
		if !cb.Allow() {
			t.Error("Expected Allow() = true in closed state")
		}
	}
}

func TestCircuitBreaker_OpenAfterFailures(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		Timeout:             1 * time.Second,
		HalfOpenMaxRequests: 2,
	}
	cb := NewCircuitBreaker(cfg)

	// Record failures
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Should be open now
	if cb.State() != StateOpen {
		t.Errorf("Expected state Open after %d failures, got %v", cfg.FailureThreshold, cb.State())
	}

	// Should not allow requests
	if cb.Allow() {
		t.Error("Expected Allow() = false in open state")
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	}
	cb := NewCircuitBreaker(cfg)

	// Trip the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("Expected Open, got %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)

	// Should be half-open now
	if cb.State() != StateHalfOpen {
		t.Errorf("Expected HalfOpen after timeout, got %v", cb.State())
	}
}

func TestCircuitBreaker_CloseAfterSuccess(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	}
	cb := NewCircuitBreaker(cfg)

	// Trip the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Allow some requests
	cb.Allow()
	cb.RecordSuccess()
	cb.Allow()
	cb.RecordSuccess()

	// Should be closed now
	if cb.State() != StateClosed {
		t.Errorf("Expected Closed after successes, got %v", cb.State())
	}
}

func TestCircuitBreaker_ReopenAfterHalfOpenFailure(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 3,
	}
	cb := NewCircuitBreaker(cfg)

	// Trip the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Fail in half-open state
	cb.Allow()
	cb.RecordFailure()

	// Should be open again
	if cb.State() != StateOpen {
		t.Errorf("Expected Open after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenRequestLimit(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		HalfOpenMaxRequests: 2,
	}
	cb := NewCircuitBreaker(cfg)

	// Trip the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)

	// Should allow up to HalfOpenMaxRequests
	for i := 0; i < 2; i++ {
		if !cb.Allow() {
			t.Errorf("Expected Allow() = true for request %d in half-open", i)
		}
	}

	// Third request should be denied
	if cb.Allow() {
		t.Error("Expected Allow() = false after max half-open requests")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             1 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	// Trip the circuit
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Error("Expected Open state")
	}

	// Manual reset
	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("Expected Closed after reset, got %v", cb.State())
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(cfg)

	// Record some activity
	cb.RecordSuccess()
	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()

	if stats.State != "closed" {
		t.Errorf("Expected state closed, got %s", stats.State)
	}
	if stats.Failures != 2 {
		t.Errorf("Expected 2 failures, got %d", stats.Failures)
	}
}

func TestCircuitBreaker_Execute_Success(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	err := cb.Execute(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestCircuitBreaker_Execute_Failure(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    2,
		SuccessThreshold:    1,
		Timeout:             1 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	testErr := errors.New("test error")

	// First failure
	err := cb.Execute(func() error {
		return testErr
	})
	if err != testErr {
		t.Errorf("Expected test error, got %v", err)
	}

	// Second failure - should trip
	cb.Execute(func() error {
		return testErr
	})

	// Circuit should be open
	err = cb.Execute(func() error {
		return nil
	})
	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	var wg sync.WaitGroup
	iterations := 100

	for i := 0; i < iterations; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			cb.Allow()
		}()
		go func() {
			defer wg.Done()
			cb.RecordSuccess()
		}()
		go func() {
			defer wg.Done()
			cb.Stats()
		}()
	}

	wg.Wait()
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold:    3,
		SuccessThreshold:    1,
		Timeout:             1 * time.Second,
		HalfOpenMaxRequests: 1,
	}
	cb := NewCircuitBreaker(cfg)

	// Record 2 failures
	cb.RecordFailure()
	cb.RecordFailure()

	// Record success - should reset failure count
	cb.RecordSuccess()

	// 2 more failures should not trip (only 2 consecutive)
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateClosed {
		t.Errorf("Expected Closed (2 failures after reset), got %v", cb.State())
	}

	// One more failure should trip
	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("Expected Open after 3 failures, got %v", cb.State())
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, tt.state.String(), tt.expected)
		}
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()

	if cfg.FailureThreshold != 5 {
		t.Errorf("Expected FailureThreshold 5, got %d", cfg.FailureThreshold)
	}
	if cfg.SuccessThreshold != 2 {
		t.Errorf("Expected SuccessThreshold 2, got %d", cfg.SuccessThreshold)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Expected Timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.HalfOpenMaxRequests != 3 {
		t.Errorf("Expected HalfOpenMaxRequests 3, got %d", cfg.HalfOpenMaxRequests)
	}
}

// Benchmark tests
func BenchmarkCircuitBreaker_Allow(b *testing.B) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Allow()
	}
}

func BenchmarkCircuitBreaker_RecordSuccess(b *testing.B) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.RecordSuccess()
	}
}

func BenchmarkCircuitBreaker_Parallel(b *testing.B) {
	cb := NewCircuitBreaker(DefaultCircuitBreakerConfig())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if cb.Allow() {
				cb.RecordSuccess()
			}
		}
	})
}
