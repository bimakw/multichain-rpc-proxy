package chain

import (
	"sync"
	"testing"
)

func TestNewLoadBalancer(t *testing.T) {
	endpoints := []*Endpoint{
		{URL: "http://ep1.example.com", Weight: 10},
		{URL: "http://ep2.example.com", Weight: 5},
	}

	lb := NewLoadBalancer(endpoints)

	if len(lb.endpoints) != 2 {
		t.Errorf("Expected 2 endpoints, got %d", len(lb.endpoints))
	}
	if lb.weights[0] != 10 {
		t.Errorf("Expected weight 10, got %d", lb.weights[0])
	}
	if lb.weights[1] != 5 {
		t.Errorf("Expected weight 5, got %d", lb.weights[1])
	}
}

func TestLoadBalancer_Next_NoEndpoints(t *testing.T) {
	lb := NewLoadBalancer([]*Endpoint{})

	_, err := lb.Next()
	if err != ErrNoHealthyEndpoints {
		t.Errorf("Expected ErrNoHealthyEndpoints, got %v", err)
	}
}

func TestLoadBalancer_Next_AllUnhealthy(t *testing.T) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep1.SetHealthy(false)
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}
	ep2.SetHealthy(false)

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2})

	_, err := lb.Next()
	if err != ErrNoHealthyEndpoints {
		t.Errorf("Expected ErrNoHealthyEndpoints, got %v", err)
	}
}

func TestLoadBalancer_Next_SingleHealthy(t *testing.T) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep1.SetHealthy(true)
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}
	ep2.SetHealthy(false)

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2})

	for i := 0; i < 10; i++ {
		ep, err := lb.Next()
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if ep.URL != "http://ep1.example.com" {
			t.Errorf("Expected ep1, got %s", ep.URL)
		}
	}
}

func TestLoadBalancer_Next_WeightedDistribution(t *testing.T) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep1.SetHealthy(true)
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}
	ep2.SetHealthy(true)

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2})

	counts := make(map[string]int)
	iterations := 15000 // Large number for statistical significance

	for i := 0; i < iterations; i++ {
		ep, err := lb.Next()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		counts[ep.URL]++
	}

	// With weights 10:5 (2:1 ratio), ep1 should get ~10000 hits, ep2 ~5000
	ep1Count := counts["http://ep1.example.com"]
	ep2Count := counts["http://ep2.example.com"]

	// Allow 10% variance
	expectedEp1 := iterations * 10 / 15 // 10/15 = 2/3
	expectedEp2 := iterations * 5 / 15  // 5/15 = 1/3

	variance := float64(iterations) * 0.10

	if abs(float64(ep1Count-expectedEp1)) > variance {
		t.Errorf("ep1 count %d not within 10%% of expected %d", ep1Count, expectedEp1)
	}
	if abs(float64(ep2Count-expectedEp2)) > variance {
		t.Errorf("ep2 count %d not within 10%% of expected %d", ep2Count, expectedEp2)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestLoadBalancer_NextWithFallback_AllUnhealthy(t *testing.T) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep1.SetHealthy(false)
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}
	ep2.SetHealthy(false)

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2})

	// Should still return an endpoint even if unhealthy
	ep, err := lb.NextWithFallback()
	if err != nil {
		t.Errorf("Expected endpoint (fallback), got error: %v", err)
	}
	if ep == nil {
		t.Error("Expected endpoint (fallback), got nil")
	}
}

func TestLoadBalancer_NextWithFallback_NoEndpoints(t *testing.T) {
	lb := NewLoadBalancer([]*Endpoint{})

	_, err := lb.NextWithFallback()
	if err != ErrNoHealthyEndpoints {
		t.Errorf("Expected ErrNoHealthyEndpoints, got %v", err)
	}
}

func TestLoadBalancer_HealthyCount(t *testing.T) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep1.SetHealthy(true)
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}
	ep2.SetHealthy(false)
	ep3 := &Endpoint{URL: "http://ep3.example.com", Weight: 5}
	ep3.SetHealthy(true)

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2, ep3})

	if lb.HealthyCount() != 2 {
		t.Errorf("Expected 2 healthy, got %d", lb.HealthyCount())
	}

	ep1.SetHealthy(false)
	if lb.HealthyCount() != 1 {
		t.Errorf("Expected 1 healthy after change, got %d", lb.HealthyCount())
	}
}

func TestLoadBalancer_AllEndpoints(t *testing.T) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2})

	eps := lb.AllEndpoints()
	if len(eps) != 2 {
		t.Errorf("Expected 2 endpoints, got %d", len(eps))
	}
}

func TestLoadBalancer_ConcurrentAccess(t *testing.T) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep1.SetHealthy(true)
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}
	ep2.SetHealthy(true)

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2})

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := lb.Next()
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent access error: %v", err)
	}
}

// Benchmark tests
func BenchmarkLoadBalancer_Next(b *testing.B) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep1.SetHealthy(true)
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}
	ep2.SetHealthy(true)

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.Next()
	}
}

func BenchmarkLoadBalancer_Next_Parallel(b *testing.B) {
	ep1 := &Endpoint{URL: "http://ep1.example.com", Weight: 10}
	ep1.SetHealthy(true)
	ep2 := &Endpoint{URL: "http://ep2.example.com", Weight: 5}
	ep2.SetHealthy(true)

	lb := NewLoadBalancer([]*Endpoint{ep1, ep2})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.Next()
		}
	})
}

func BenchmarkLoadBalancer_HealthyCount(b *testing.B) {
	endpoints := make([]*Endpoint, 10)
	for i := range endpoints {
		endpoints[i] = &Endpoint{URL: "http://ep.example.com", Weight: 1}
		endpoints[i].SetHealthy(i%2 == 0)
	}

	lb := NewLoadBalancer(endpoints)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.HealthyCount()
	}
}
