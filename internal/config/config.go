package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig            `yaml:"server"`
	GRPC      GRPCConfig              `yaml:"grpc"`
	Redis     RedisConfig             `yaml:"redis"`
	Metrics   MetricsConfig           `yaml:"metrics"`
	RateLimit RateLimitConfig         `yaml:"rate_limit"`
	Cache     CacheConfig             `yaml:"cache"`
	Chains    map[string]*ChainConfig `yaml:"chains"`
}

// GRPCConfig holds gRPC server configuration
type GRPCConfig struct {
	Enabled bool      `yaml:"enabled"`
	Port    int       `yaml:"port"`
	TLS     TLSConfig `yaml:"tls"`
}

type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	TLS          TLSConfig     `yaml:"tls"`
}

// TLSConfig holds TLS/SSL configuration for the server
type TLSConfig struct {
	Enabled   bool            `yaml:"enabled"`
	CertFile  string          `yaml:"cert_file"`
	KeyFile   string          `yaml:"key_file"`
	ClientTLS ClientTLSConfig `yaml:"client_tls"`
}

type ClientTLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
	CAFile             string `yaml:"ca_file"`
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type MetricsConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

type RateLimitConfig struct {
	Enabled           bool   `yaml:"enabled"`
	RequestsPerSecond int    `yaml:"requests_per_second"`
	Burst             int    `yaml:"burst"`
	Strategy          string `yaml:"strategy"` // "token_bucket" or "sliding_window"
	PerIP             bool   `yaml:"per_ip"`   // Rate limit per IP address
}

type CacheConfig struct {
	Enabled          bool          `yaml:"enabled"`
	TTL              time.Duration `yaml:"ttl"`
	CacheableMethods []string      `yaml:"cacheable_methods"`
}

type ChainConfig struct {
	ChainID        int                  `yaml:"chain_id"`
	Endpoints      []EndpointConfig     `yaml:"endpoints"`
	HealthCheck    HealthCheckConfig    `yaml:"health_check"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
	HTTPPool       HTTPPoolConfig       `yaml:"http_pool"`
	RateLimit      *RateLimitConfig     `yaml:"rate_limit,omitempty"` // Per-chain rate limit (overrides global)
}

// HTTPPoolConfig holds HTTP connection pool configuration
type HTTPPoolConfig struct {
	MaxIdleConns        int           `yaml:"max_idle_conns"`
	MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host"`
	MaxConnsPerHost     int           `yaml:"max_conns_per_host"`
	IdleConnTimeout     time.Duration `yaml:"idle_conn_timeout"`
}

type EndpointConfig struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight"`
}

type HealthCheckConfig struct {
	Interval    time.Duration `yaml:"interval"`
	Timeout     time.Duration `yaml:"timeout"`
	MaxBlockLag int           `yaml:"max_block_lag"`
}

type CircuitBreakerConfig struct {
	Enabled             bool          `yaml:"enabled"`
	FailureThreshold    int           `yaml:"failure_threshold"`
	SuccessThreshold    int           `yaml:"success_threshold"`
	Timeout             time.Duration `yaml:"timeout"`
	HalfOpenMaxRequests int           `yaml:"half_open_max_requests"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30 * time.Second
	}
	if cfg.Metrics.Port == 0 {
		cfg.Metrics.Port = 9090
	}
	if cfg.GRPC.Port == 0 {
		cfg.GRPC.Port = 9000
	}
	if cfg.Cache.TTL == 0 {
		cfg.Cache.TTL = 60 * time.Second
	}

	for _, chainCfg := range cfg.Chains {
		if chainCfg.HTTPPool.MaxIdleConns == 0 {
			chainCfg.HTTPPool.MaxIdleConns = 100
		}
		if chainCfg.HTTPPool.MaxIdleConnsPerHost == 0 {
			chainCfg.HTTPPool.MaxIdleConnsPerHost = 100
		}
		if chainCfg.HTTPPool.IdleConnTimeout == 0 {
			chainCfg.HTTPPool.IdleConnTimeout = 90 * time.Second
		}
	}

	return &cfg, nil
}

func (c *CacheConfig) IsCacheableMethod(method string) bool {
	for _, m := range c.CacheableMethods {
		if m == method {
			return true
		}
	}
	return false
}
