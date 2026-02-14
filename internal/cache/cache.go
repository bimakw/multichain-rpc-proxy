package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/bimakw/multichain-rpc-proxy/internal/config"
)

type Cache struct {
	client           *redis.Client
	ttl              time.Duration
	cacheableMethods map[string]bool
	enabled          bool
}

func New(cfg config.RedisConfig, cacheCfg config.CacheConfig) (*Cache, error) {
	if !cacheCfg.Enabled {
		return &Cache{enabled: false}, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	methods := make(map[string]bool)
	for _, m := range cacheCfg.CacheableMethods {
		methods[m] = true
	}

	return &Cache{
		client:           client,
		ttl:              cacheCfg.TTL,
		cacheableMethods: methods,
		enabled:          true,
	}, nil
}

func (c *Cache) IsCacheable(method string) bool {
	if !c.enabled {
		return false
	}
	return c.cacheableMethods[method]
}

func (c *Cache) Get(ctx context.Context, chain, method string, params []byte) ([]byte, bool) {
	if !c.enabled {
		return nil, false
	}

	key := c.makeKey(chain, method, params)
	val, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}

	return val, true
}

func (c *Cache) Set(ctx context.Context, chain, method string, params []byte, response []byte) error {
	if !c.enabled {
		return nil
	}

	key := c.makeKey(chain, method, params)
	return c.client.Set(ctx, key, response, c.ttl).Err()
}

// makeKey creates a cache key
func (c *Cache) makeKey(chain, method string, params []byte) string {
	h := sha256.New()
	h.Write([]byte(chain))
	h.Write([]byte(method))
	h.Write(params)
	return fmt.Sprintf("rpc:%s:%s:%s", chain, method, hex.EncodeToString(h.Sum(nil))[:16])
}

func (c *Cache) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

func (c *Cache) Enabled() bool {
	return c.enabled
}

type InMemoryCache struct {
	data             map[string]cacheEntry
	ttl              time.Duration
	cacheableMethods map[string]bool
	enabled          bool
}

type cacheEntry struct {
	value     []byte
	expiresAt time.Time
}

func NewInMemory(cacheCfg config.CacheConfig) *InMemoryCache {
	methods := make(map[string]bool)
	for _, m := range cacheCfg.CacheableMethods {
		methods[m] = true
	}

	return &InMemoryCache{
		data:             make(map[string]cacheEntry),
		ttl:              cacheCfg.TTL,
		cacheableMethods: methods,
		enabled:          cacheCfg.Enabled,
	}
}

func (c *InMemoryCache) IsCacheable(method string) bool {
	if !c.enabled {
		return false
	}
	return c.cacheableMethods[method]
}

func (c *InMemoryCache) Get(chain, method string, params []byte) ([]byte, bool) {
	if !c.enabled {
		return nil, false
	}

	key := c.makeKey(chain, method, params)
	entry, ok := c.data[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(c.data, key)
		return nil, false
	}

	return entry.value, true
}

func (c *InMemoryCache) Set(chain, method string, params []byte, response []byte) {
	if !c.enabled {
		return
	}

	key := c.makeKey(chain, method, params)
	c.data[key] = cacheEntry{
		value:     response,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *InMemoryCache) makeKey(chain, method string, params []byte) string {
	h := sha256.New()
	h.Write([]byte(chain))
	h.Write([]byte(method))
	h.Write(params)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

func (c *InMemoryCache) Enabled() bool {
	return c.enabled
}
