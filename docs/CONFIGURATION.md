# Configuration Reference

This document provides a complete reference for all configuration options available in the Multichain RPC Proxy.

## Configuration File

The proxy is configured using a YAML file. By default, it looks for `configs/config.yaml`, but you can specify a different path using the `-config` flag:

```bash
./multichain-rpc-proxy -config /path/to/config.yaml
```

## Configuration Sections

### Server Configuration

```yaml
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `port` | int | `8080` | HTTP server port for RPC endpoints |
| `read_timeout` | duration | `30s` | Maximum duration for reading request body |
| `write_timeout` | duration | `30s` | Maximum duration for writing response |

### Redis Configuration

```yaml
redis:
  addr: "localhost:6379"
  password: ""
  db: 0
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `addr` | string | `localhost:6379` | Redis server address |
| `password` | string | `""` | Redis password (empty for no auth) |
| `db` | int | `0` | Redis database number |

**Note:** Redis is optional. If not available, the proxy will use in-memory caching.

### Metrics Configuration

```yaml
metrics:
  enabled: true
  port: 9090
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable Prometheus metrics endpoint |
| `port` | int | `9090` | Port for metrics HTTP server |

When enabled, Prometheus metrics are available at `http://localhost:9090/metrics`.

### Rate Limiting Configuration

```yaml
rate_limit:
  enabled: true
  requests_per_second: 100
  burst: 200
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable rate limiting |
| `requests_per_second` | int | `100` | Sustained request rate per IP |
| `burst` | int | `200` | Maximum burst size |

The rate limiter uses the **token bucket algorithm**:
- Each IP address has a bucket that fills at `requests_per_second` rate
- Maximum tokens in bucket is `burst`
- Each request consumes one token
- Returns HTTP 429 when bucket is empty

### Cache Configuration

```yaml
cache:
  enabled: true
  ttl: 60s
  cacheable_methods:
    - eth_chainId
    - net_version
    - web3_clientVersion
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `true` | Enable response caching |
| `ttl` | duration | `60s` | Time-to-live for cached responses |
| `cacheable_methods` | []string | (see below) | RPC methods safe to cache |

**Default cacheable methods:**
- `eth_chainId` - Returns chain ID (never changes)
- `net_version` - Returns network version
- `web3_clientVersion` - Returns client version string

**Important:** Only cache methods that return static data. Never cache methods like `eth_getBalance` or `eth_blockNumber`.

### Chain Configuration

Each chain is configured under the `chains` section:

```yaml
chains:
  ethereum:
    chain_id: 1
    endpoints:
      - url: "https://eth.llamarpc.com"
        weight: 10
      - url: "https://rpc.ankr.com/eth"
        weight: 5
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 10
    circuit_breaker:
      enabled: true
      failure_threshold: 5
      success_threshold: 2
      timeout: 30s
      half_open_max_requests: 3
```

#### Chain-Level Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `chain_id` | int | Yes | EIP-155 chain ID |
| `endpoints` | []Endpoint | Yes | List of RPC endpoints |
| `health_check` | HealthCheck | Yes | Health check configuration |
| `circuit_breaker` | CircuitBreaker | No | Circuit breaker configuration |

#### Endpoint Options

| Option | Type | Required | Description |
|--------|------|----------|-------------|
| `url` | string | Yes | RPC endpoint URL |
| `weight` | int | Yes | Load balancer weight (higher = more traffic) |

**Weight distribution example:**
- EP1 (weight: 10), EP2 (weight: 5), EP3 (weight: 5)
- EP1 receives 50% of traffic (10/20)
- EP2 receives 25% of traffic (5/20)
- EP3 receives 25% of traffic (5/20)

#### Health Check Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `interval` | duration | `10s` | How often to check endpoint health |
| `timeout` | duration | `5s` | Timeout for health check request |
| `max_block_lag` | int | `10` | Maximum block lag before marking unhealthy |

**Health check process:**
1. Call `eth_blockNumber` on each endpoint
2. Compare block heights across endpoints
3. Mark endpoint unhealthy if:
   - Request fails or times out
   - Block height lags behind highest by more than `max_block_lag`

#### Circuit Breaker Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | `false` | Enable circuit breaker |
| `failure_threshold` | int | `5` | Failures before opening circuit |
| `success_threshold` | int | `2` | Successes in half-open to close |
| `timeout` | duration | `30s` | Time before attempting recovery |
| `half_open_max_requests` | int | `3` | Max requests in half-open state |

**Circuit breaker states:**
- **Closed**: Normal operation
- **Open**: Failing fast (no requests sent to endpoint)
- **Half-Open**: Testing recovery with limited requests

## Complete Example Configuration

```yaml
# Server settings
server:
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

# Redis for distributed caching (optional)
redis:
  addr: "redis:6379"
  password: ""
  db: 0

# Prometheus metrics
metrics:
  enabled: true
  port: 9090

# Rate limiting
rate_limit:
  enabled: true
  requests_per_second: 100
  burst: 200

# Response caching
cache:
  enabled: true
  ttl: 60s
  cacheable_methods:
    - eth_chainId
    - net_version
    - web3_clientVersion

# Blockchain chains
chains:
  # Ethereum Mainnet
  ethereum:
    chain_id: 1
    endpoints:
      - url: "https://eth.llamarpc.com"
        weight: 10
      - url: "https://rpc.ankr.com/eth"
        weight: 5
      - url: "https://ethereum.publicnode.com"
        weight: 5
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 10
    circuit_breaker:
      enabled: true
      failure_threshold: 5
      success_threshold: 2
      timeout: 30s
      half_open_max_requests: 3

  # Arbitrum One
  arbitrum:
    chain_id: 42161
    endpoints:
      - url: "https://arb1.arbitrum.io/rpc"
        weight: 10
      - url: "https://rpc.ankr.com/arbitrum"
        weight: 5
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 50

  # Optimism
  optimism:
    chain_id: 10
    endpoints:
      - url: "https://mainnet.optimism.io"
        weight: 10
      - url: "https://rpc.ankr.com/optimism"
        weight: 5
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 50

  # Base
  base:
    chain_id: 8453
    endpoints:
      - url: "https://mainnet.base.org"
        weight: 10
      - url: "https://base.llamarpc.com"
        weight: 5
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 50

  # Polygon
  polygon:
    chain_id: 137
    endpoints:
      - url: "https://polygon-rpc.com"
        weight: 10
      - url: "https://rpc.ankr.com/polygon"
        weight: 5
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 100
```

## Adding Custom Chains

To add a new chain, add a new entry under `chains`:

```yaml
chains:
  # Your custom chain
  avalanche:
    chain_id: 43114
    endpoints:
      - url: "https://api.avax.network/ext/bc/C/rpc"
        weight: 10
      - url: "https://rpc.ankr.com/avalanche"
        weight: 5
    health_check:
      interval: 10s
      timeout: 5s
      max_block_lag: 100
```

The chain will be available at `POST http://localhost:8080/avalanche`.

## Configuration Tips

### Choosing Block Lag Thresholds

| Chain Type | Recommended `max_block_lag` |
|------------|---------------------------|
| Mainnet (ETH, etc) | 5-15 blocks |
| L2 (Arbitrum, OP) | 50-100 blocks |
| Fast chains (BSC, Polygon) | 50-200 blocks |

### Setting Endpoint Weights

- Use higher weights for endpoints with:
  - Lower latency
  - Higher reliability
  - Better rate limits
- Start with equal weights and adjust based on monitoring

### Rate Limit Tuning

- `burst` should be 2-3x `requests_per_second`
- Monitor `rpc_proxy_rate_limit_rejects_total` metric
- Increase limits if legitimate traffic is being rejected

### Cache TTL Considerations

- Longer TTL = fewer backend requests, higher memory usage
- Shorter TTL = fresher data, more backend requests
- For static methods (chainId, version), 60s-300s is reasonable
