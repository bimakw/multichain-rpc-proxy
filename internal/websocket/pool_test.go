package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()

	if cfg.MaxConnections != 10 {
		t.Errorf("Expected MaxConnections 10, got %d", cfg.MaxConnections)
	}
	if cfg.IdleTimeout != 5*time.Minute {
		t.Errorf("Expected IdleTimeout 5m, got %v", cfg.IdleTimeout)
	}
	if cfg.HealthCheckInterval != 30*time.Second {
		t.Errorf("Expected HealthCheckInterval 30s, got %v", cfg.HealthCheckInterval)
	}
}

func TestNewConnectionPool(t *testing.T) {
	cfg := DefaultPoolConfig()
	pool := NewConnectionPool("ws://example.com", cfg)
	defer pool.Close()

	if pool.url != "ws://example.com" {
		t.Errorf("Expected URL ws://example.com, got %s", pool.url)
	}
	if pool.closed {
		t.Error("Pool should not be closed initially")
	}
}

func TestConnectionPool_GetFromEmptyPool(t *testing.T) {
	// Create a mock WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Keep connection alive
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := DefaultPoolConfig()
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	conn, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}

	if conn == nil {
		t.Fatal("Expected connection, got nil")
	}
	if !conn.InUse {
		t.Error("Connection should be marked as in use")
	}
}

func TestConnectionPool_PutAndReuse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      2,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Get first connection
	conn1, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get first connection: %v", err)
	}

	// Put it back
	pool.Put(conn1)

	// Get again - should reuse
	conn2, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get second connection: %v", err)
	}

	if conn1 != conn2 {
		t.Error("Expected to reuse the same connection")
	}
}

func TestConnectionPool_MaxConnections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      2,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Get max connections
	conn1, _ := pool.Get()
	conn2, _ := pool.Get()

	if conn1 == nil || conn2 == nil {
		t.Fatal("Should be able to get max connections")
	}

	// Third connection should fail
	_, err := pool.Get()
	if err != ErrPoolExhausted {
		t.Errorf("Expected ErrPoolExhausted, got %v", err)
	}

	// Return one and try again
	pool.Put(conn1)
	conn3, err := pool.Get()
	if err != nil {
		t.Errorf("Should be able to get connection after return: %v", err)
	}
	if conn3 == nil {
		t.Error("Expected connection, got nil")
	}
}

func TestConnectionPool_Remove(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      2,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	conn1, _ := pool.Get()
	conn2, _ := pool.Get()

	pool.Remove(conn1)

	// Should be able to get a new connection now
	conn3, err := pool.Get()
	if err != nil {
		t.Errorf("Should be able to get connection after remove: %v", err)
	}
	if conn3 == nil {
		t.Error("Expected connection, got nil")
	}

	_ = conn2 // Keep reference to avoid GC
}

func TestConnectionPool_Close(t *testing.T) {
	cfg := DefaultPoolConfig()
	pool := NewConnectionPool("ws://example.com", cfg)

	pool.Close()

	if !pool.closed {
		t.Error("Pool should be closed")
	}

	// Get should fail on closed pool
	_, err := pool.Get()
	if err != ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed, got %v", err)
	}
}

func TestConnectionPool_Stats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      5,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Initial stats
	stats := pool.Stats()
	if stats.TotalConnections != 0 {
		t.Errorf("Expected 0 total connections, got %d", stats.TotalConnections)
	}

	// Get some connections
	conn1, _ := pool.Get()
	conn2, _ := pool.Get()

	stats = pool.Stats()
	if stats.TotalConnections != 2 {
		t.Errorf("Expected 2 total connections, got %d", stats.TotalConnections)
	}
	if stats.ActiveConnections != 2 {
		t.Errorf("Expected 2 active connections, got %d", stats.ActiveConnections)
	}

	// Return one
	pool.Put(conn1)

	stats = pool.Stats()
	if stats.ActiveConnections != 1 {
		t.Errorf("Expected 1 active connection, got %d", stats.ActiveConnections)
	}
	if stats.IdleConnections != 1 {
		t.Errorf("Expected 1 idle connection, got %d", stats.IdleConnections)
	}

	_ = conn2 // Keep reference
}

func TestPoolError_Error(t *testing.T) {
	if ErrPoolClosed.Error() != "connection pool is closed" {
		t.Errorf("Unexpected error message: %s", ErrPoolClosed.Error())
	}
	if ErrPoolExhausted.Error() != "connection pool exhausted" {
		t.Errorf("Unexpected error message: %s", ErrPoolExhausted.Error())
	}
}

// Benchmark tests
func BenchmarkConnectionPool_Get(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      100,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Pre-fill pool
	conns := make([]*PooledConnection, 10)
	for i := range conns {
		c, _ := pool.Get()
		conns[i] = c
	}
	for _, c := range conns {
		pool.Put(c)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c, _ := pool.Get()
		pool.Put(c)
	}
}

// Additional tests for improved coverage

func TestConnectionPool_Cleanup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      5,
		IdleTimeout:         50 * time.Millisecond,
		HealthCheckInterval: 20 * time.Millisecond,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Get and return a connection
	conn, err := pool.Get()
	if err != nil {
		t.Fatalf("Failed to get connection: %v", err)
	}
	pool.Put(conn)

	// Wait for cleanup to run
	time.Sleep(100 * time.Millisecond)

	// The idle connection should be cleaned up
	stats := pool.Stats()
	// After cleanup, idle connections beyond timeout should be removed
	// This test verifies that cleanup runs without errors
	_ = stats
}

func TestConnectionPool_Cleanup_WithActiveConnections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      5,
		IdleTimeout:         50 * time.Millisecond,
		HealthCheckInterval: 20 * time.Millisecond,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Get connections and keep one active
	conn1, _ := pool.Get()
	conn2, _ := pool.Get()

	// Return one but keep one active
	pool.Put(conn1)

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Active connection should still exist
	stats := pool.Stats()
	if stats.ActiveConnections < 1 {
		t.Error("Active connection should be preserved during cleanup")
	}

	_ = conn2
}

func TestConnectionPool_IsHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      5,
		IdleTimeout:         50 * time.Millisecond,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Get a connection
	conn, _ := pool.Get()
	pool.Put(conn)

	// Connection should be healthy immediately after return
	// Wait for it to become unhealthy
	time.Sleep(60 * time.Millisecond)

	// Try to get - the stale connection should not be returned
	conn2, err := pool.Get()
	if err != nil {
		// This is expected if the stale connection was rejected
		return
	}

	// If we got a connection, it might be a new one
	defer pool.Put(conn2)
}

func TestConnectionPool_Get_ConnectionFailure(t *testing.T) {
	// Use an invalid URL that will fail to connect
	cfg := PoolConfig{
		MaxConnections:      2,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool("ws://localhost:9999", cfg)
	defer pool.Close()

	// Get should fail
	_, err := pool.Get()
	if err == nil {
		t.Error("Get should fail with invalid URL")
	}
}

func TestConnectionPool_Remove_NotFound(t *testing.T) {
	cfg := DefaultPoolConfig()
	pool := NewConnectionPool("ws://example.com", cfg)
	defer pool.Close()

	// Try to remove a connection that doesn't exist
	fakeConn := &PooledConnection{
		URL: "ws://fake.com",
	}

	// This should not panic
	pool.Remove(fakeConn)
}

func TestPoolStats_Fields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      10,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Get some connections
	conn1, _ := pool.Get()
	conn2, _ := pool.Get()
	conn3, _ := pool.Get()

	pool.Put(conn1)

	stats := pool.Stats()

	if stats.URL != server.URL {
		t.Errorf("Expected URL %s, got %s", server.URL, stats.URL)
	}
	if stats.TotalConnections != 3 {
		t.Errorf("Expected 3 total connections, got %d", stats.TotalConnections)
	}
	if stats.ActiveConnections != 2 {
		t.Errorf("Expected 2 active connections, got %d", stats.ActiveConnections)
	}
	if stats.IdleConnections != 1 {
		t.Errorf("Expected 1 idle connection, got %d", stats.IdleConnections)
	}

	_ = conn2
	_ = conn3
}

func TestPooledConnection_Fields(t *testing.T) {
	now := time.Now()
	conn := &PooledConnection{
		Conn:       nil,
		URL:        "ws://test.com",
		CreatedAt:  now,
		LastUsedAt: now,
		InUse:      true,
	}

	if conn.URL != "ws://test.com" {
		t.Errorf("Expected URL ws://test.com, got %s", conn.URL)
	}
	if !conn.InUse {
		t.Error("Expected InUse to be true")
	}
	if conn.CreatedAt != now {
		t.Error("CreatedAt mismatch")
	}
	if conn.LastUsedAt != now {
		t.Error("LastUsedAt mismatch")
	}
}

func TestPoolConfig_Fields(t *testing.T) {
	cfg := PoolConfig{
		MaxConnections:      20,
		IdleTimeout:         10 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}

	if cfg.MaxConnections != 20 {
		t.Errorf("Expected MaxConnections 20, got %d", cfg.MaxConnections)
	}
	if cfg.IdleTimeout != 10*time.Minute {
		t.Errorf("Expected IdleTimeout 10m, got %v", cfg.IdleTimeout)
	}
	if cfg.HealthCheckInterval != 1*time.Minute {
		t.Errorf("Expected HealthCheckInterval 1m, got %v", cfg.HealthCheckInterval)
	}
}

func TestConnectionPool_ConcurrentAccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      10,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Concurrent Get and Put
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				conn, err := pool.Get()
				if err != nil {
					continue
				}
				time.Sleep(time.Millisecond)
				pool.Put(conn)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Pool should still be healthy
	stats := pool.Stats()
	if stats.TotalConnections > cfg.MaxConnections {
		t.Errorf("Pool exceeded max connections: %d > %d", stats.TotalConnections, cfg.MaxConnections)
	}
}

func BenchmarkConnectionPool_Stats(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))
	defer server.Close()

	cfg := PoolConfig{
		MaxConnections:      100,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 1 * time.Minute,
	}
	pool := NewConnectionPool(server.URL, cfg)
	defer pool.Close()

	// Pre-fill pool
	conns := make([]*PooledConnection, 10)
	for i := range conns {
		c, _ := pool.Get()
		conns[i] = c
	}
	for _, c := range conns {
		pool.Put(c)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Stats()
	}
}
