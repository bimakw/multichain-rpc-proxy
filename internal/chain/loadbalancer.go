package chain

import (
	"errors"
	"sync"
	"sync/atomic"
)

var (
	ErrNoHealthyEndpoints = errors.New("no healthy endpoints available")
)

// LoadBalancer selects endpoints using weighted round-robin
type LoadBalancer struct {
	endpoints []*Endpoint
	weights   []int
	current   atomic.Int64
	mu        sync.RWMutex
}

// NewLoadBalancer creates a new load balancer
func NewLoadBalancer(endpoints []*Endpoint) *LoadBalancer {
	lb := &LoadBalancer{
		endpoints: endpoints,
		weights:   make([]int, len(endpoints)),
	}

	for i, ep := range endpoints {
		lb.weights[i] = ep.Weight
	}

	return lb
}

// Next returns the next healthy endpoint using weighted round-robin
func (lb *LoadBalancer) Next() (*Endpoint, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.endpoints) == 0 {
		return nil, ErrNoHealthyEndpoints
	}

	// Try weighted round-robin first
	totalWeight := 0
	for i, ep := range lb.endpoints {
		if ep.IsHealthy() {
			totalWeight += lb.weights[i]
		}
	}

	if totalWeight == 0 {
		return nil, ErrNoHealthyEndpoints
	}

	// Get next index
	idx := lb.current.Add(1)

	// Weighted selection
	selected := int(idx % int64(totalWeight))
	cumulative := 0

	for i, ep := range lb.endpoints {
		if !ep.IsHealthy() {
			continue
		}
		cumulative += lb.weights[i]
		if selected < cumulative {
			return ep, nil
		}
	}

	// Fallback: return first healthy endpoint
	for _, ep := range lb.endpoints {
		if ep.IsHealthy() {
			return ep, nil
		}
	}

	return nil, ErrNoHealthyEndpoints
}

// NextWithFallback tries to get an endpoint, falls back to any available if all unhealthy
func (lb *LoadBalancer) NextWithFallback() (*Endpoint, error) {
	ep, err := lb.Next()
	if err == nil {
		return ep, nil
	}

	// Fallback: try any endpoint
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.endpoints) > 0 {
		idx := lb.current.Add(1) % int64(len(lb.endpoints))
		return lb.endpoints[idx], nil
	}

	return nil, ErrNoHealthyEndpoints
}

// HealthyCount returns the number of healthy endpoints
func (lb *LoadBalancer) HealthyCount() int {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	count := 0
	for _, ep := range lb.endpoints {
		if ep.IsHealthy() {
			count++
		}
	}
	return count
}

// AllEndpoints returns all endpoints
func (lb *LoadBalancer) AllEndpoints() []*Endpoint {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.endpoints
}
