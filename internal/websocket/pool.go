package websocket

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type PoolConfig struct {
	MaxConnections      int           `yaml:"max_connections"`
	IdleTimeout         time.Duration `yaml:"idle_timeout"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	WaitTimeout         time.Duration `yaml:"wait_timeout"`
	MaxLifetime         time.Duration `yaml:"max_lifetime"`
}

func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConnections:      10,
		IdleTimeout:         5 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
		WaitTimeout:         10 * time.Second,
		MaxLifetime:         30 * time.Minute,
	}
}

type PooledConnection struct {
	Conn       *websocket.Conn
	URL        string
	CreatedAt  time.Time
	LastUsedAt time.Time
	InUse      bool
}

type ConnectionPool struct {
	url         string
	config      PoolConfig
	connections []*PooledConnection
	mu          sync.Mutex
	closed      bool
	done        chan struct{}
}

func NewConnectionPool(url string, config PoolConfig) *ConnectionPool {
	pool := &ConnectionPool{
		url:         url,
		config:      config,
		connections: make([]*PooledConnection, 0, config.MaxConnections),
		done:        make(chan struct{}),
	}

	go pool.healthCheck()

	return pool
}

func (p *ConnectionPool) Get() (*PooledConnection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil, ErrPoolClosed
	}

	for _, conn := range p.connections {
		if !conn.InUse && p.isHealthy(conn) {
			conn.InUse = true
			conn.LastUsedAt = time.Now()
			return conn, nil
		}
	}

	if len(p.connections) < p.config.MaxConnections {
		conn, err := p.createConnection()
		if err != nil {
			return nil, err
		}
		p.connections = append(p.connections, conn)
		return conn, nil
	}

	return nil, ErrPoolExhausted
}

func (p *ConnectionPool) GetWithWait(ctx context.Context) (*PooledConnection, error) {
	conn, err := p.Get()
	if err != ErrPoolExhausted {
		return conn, err
	}

	waitTimeout := p.config.WaitTimeout
	if waitTimeout == 0 {
		waitTimeout = 10 * time.Second
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(waitTimeout)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, ErrPoolExhausted
		case <-ticker.C:
			conn, err := p.Get()
			if err != ErrPoolExhausted {
				return conn, err
			}
		}
	}
}

func (p *ConnectionPool) Put(conn *PooledConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	conn.InUse = false
	conn.LastUsedAt = time.Now()
}

func (p *ConnectionPool) Remove(conn *PooledConnection) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, c := range p.connections {
		if c == conn {
			c.Conn.Close()
			p.connections = append(p.connections[:i], p.connections[i+1:]...)
			return
		}
	}
}

func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	close(p.done)

	for _, conn := range p.connections {
		conn.Conn.Close()
	}
	p.connections = nil
}

// createConnection creates a new WebSocket connection
func (p *ConnectionPool) createConnection() (*PooledConnection, error) {
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(httpToWS(p.url), nil)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	return &PooledConnection{
		Conn:       conn,
		URL:        p.url,
		CreatedAt:  now,
		LastUsedAt: now,
		InUse:      true,
	}, nil
}

// isHealthy checks if a connection is still healthy
func (p *ConnectionPool) isHealthy(conn *PooledConnection) bool {
	if time.Since(conn.LastUsedAt) > p.config.IdleTimeout {
		return false
	}

	if p.config.MaxLifetime > 0 && time.Since(conn.CreatedAt) > p.config.MaxLifetime {
		return false
	}

	return true
}

// healthCheck periodically checks and cleans up connections
func (p *ConnectionPool) healthCheck() {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.cleanup()
		}
	}
}

// cleanup removes stale connections
func (p *ConnectionPool) cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	now := time.Now()
	alive := make([]*PooledConnection, 0, len(p.connections))

	for _, conn := range p.connections {
		if conn.InUse {
			alive = append(alive, conn)
			continue
		}

		if now.Sub(conn.LastUsedAt) > p.config.IdleTimeout {
			log.Printf("[Pool] Closing idle connection to %s", conn.URL)
			conn.Conn.Close()
			continue
		}

		alive = append(alive, conn)
	}

	p.connections = alive
}

func (p *ConnectionPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	stats := PoolStats{
		URL:              p.url,
		TotalConnections: len(p.connections),
	}

	for _, conn := range p.connections {
		if conn.InUse {
			stats.ActiveConnections++
		} else {
			stats.IdleConnections++
		}
	}

	return stats
}

type PoolStats struct {
	URL               string
	TotalConnections  int
	ActiveConnections int
	IdleConnections   int
}

type PoolError string

func (e PoolError) Error() string {
	return string(e)
}

const (
	ErrPoolClosed    PoolError = "connection pool is closed"
	ErrPoolExhausted PoolError = "connection pool exhausted"
)
