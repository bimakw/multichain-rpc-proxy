package chain

import (
	"errors"
	"sync"
	"sync/atomic"
)

var (
	ErrNoHealthyEndpoints = errors.New("no healthy endpoints available")
)

type LoadBalancer struct {
	endpoints []*Endpoint
	weights   []int
	current   atomic.Int64
	mu        sync.RWMutex
}

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

func (lb *LoadBalancer) Next() (*Endpoint, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.endpoints) == 0 {
		return nil, ErrNoHealthyEndpoints
	}

	totalWeight := 0
	for i, ep := range lb.endpoints {
		if ep.IsHealthy() {
			totalWeight += lb.weights[i]
		}
	}

	if totalWeight == 0 {
		return nil, ErrNoHealthyEndpoints
	}

	idx := lb.current.Add(1)

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

	for _, ep := range lb.endpoints {
		if ep.IsHealthy() {
			return ep, nil
		}
	}

	return nil, ErrNoHealthyEndpoints
}

func (lb *LoadBalancer) NextWithFallback() (*Endpoint, error) {
	ep, err := lb.Next()
	if err == nil {
		return ep, nil
	}

	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.endpoints) > 0 {
		idx := lb.current.Add(1) % int64(len(lb.endpoints))
		return lb.endpoints[idx], nil
	}

	return nil, ErrNoHealthyEndpoints
}

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

func (lb *LoadBalancer) AllEndpoints() []*Endpoint {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	return lb.endpoints
}
