package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig            `yaml:"server"`
	Redis     RedisConfig             `yaml:"redis"`
	Metrics   MetricsConfig           `yaml:"metrics"`
	RateLimit RateLimitConfig         `yaml:"rate_limit"`
	Cache     CacheConfig             `yaml:"cache"`
	Chains    map[string]*ChainConfig `yaml:"chains"`
}

type ServerConfig struct {
	Port         int           `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
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
	Enabled           bool `yaml:"enabled"`
	RequestsPerSecond int  `yaml:"requests_per_second"`
	Burst             int  `yaml:"burst"`
}

type CacheConfig struct {
	Enabled          bool          `yaml:"enabled"`
	TTL              time.Duration `yaml:"ttl"`
	CacheableMethods []string      `yaml:"cacheable_methods"`
}

type ChainConfig struct {
	ChainID     int               `yaml:"chain_id"`
	Endpoints   []EndpointConfig  `yaml:"endpoints"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults
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
	if cfg.Cache.TTL == 0 {
		cfg.Cache.TTL = 60 * time.Second
	}

	return &cfg, nil
}

// IsCacheableMethod checks if the RPC method can be cached
func (c *CacheConfig) IsCacheableMethod(method string) bool {
	for _, m := range c.CacheableMethods {
		if m == method {
			return true
		}
	}
	return false
}
