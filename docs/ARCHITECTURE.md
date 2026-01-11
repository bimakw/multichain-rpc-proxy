# Architecture

This document describes the internal architecture of the Multichain RPC Proxy.

## Overview

The Multichain RPC Proxy is a high-performance load balancer designed for blockchain RPC endpoints. It provides intelligent request routing, health monitoring, and fault tolerance for multiple blockchain networks.

## System Architecture

```
                                    ┌─────────────────────┐
                                    │   Client Request    │
                                    │  (HTTP/WebSocket)   │
                                    └──────────┬──────────┘
                                               │
                                               ▼
┌──────────────────────────────────────────────────────────────────────────┐
│                           HTTP Server (Fiber)                              │
│                                                                            │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────────────────┐  │
│  │ CORS Middleware│──│ Rate Limiter   │──│ Recovery Middleware        │  │
│  │                │  │ (Token Bucket) │  │ (Panic Handler)            │  │
│  └────────────────┘  └────────────────┘  └────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────┘
                                               │
                    ┌──────────────────────────┼──────────────────────────┐
                    │                          │                          │
                    ▼                          ▼                          ▼
          ┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
          │  /health        │       │  /:chain        │       │  /ws/:chain     │
          │  /health/:chain │       │  (RPC Handler)  │       │  (WebSocket)    │
          │  /chains        │       │                 │       │                 │
          └─────────────────┘       └────────┬────────┘       └────────┬────────┘
                                             │                         │
                                             ▼                         │
                                    ┌─────────────────┐                │
                                    │  Cache Layer    │                │
                                    │  (In-Memory)    │                │
                                    └────────┬────────┘                │
                                             │                         │
                                             ▼                         │
                                    ┌─────────────────┐                │
                                    │  Chain Manager  │◄───────────────┘
                                    └────────┬────────┘
                                             │
             ┌───────────────────────────────┼───────────────────────────────┐
             │                               │                               │
             ▼                               ▼                               ▼
    ┌─────────────────┐             ┌─────────────────┐             ┌─────────────────┐
    │  Ethereum Chain │             │  Arbitrum Chain │             │     ...         │
    └────────┬────────┘             └────────┬────────┘             └────────┬────────┘
             │                               │                               │
             ▼                               ▼                               ▼
    ┌─────────────────┐             ┌─────────────────┐             ┌─────────────────┐
    │  Load Balancer  │             │  Load Balancer  │             │  Load Balancer  │
    │  (Round Robin)  │             │  (Round Robin)  │             │  (Round Robin)  │
    └────────┬────────┘             └────────┬────────┘             └────────┬────────┘
             │                               │                               │
    ┌────────┼────────┐             ┌────────┼────────┐             ┌────────┼────────┐
    │        │        │             │        │        │             │        │        │
    ▼        ▼        ▼             ▼        ▼        ▼             ▼        ▼        ▼
┌───────┐┌───────┐┌───────┐   ┌───────┐┌───────┐┌───────┐   ┌───────┐┌───────┐┌───────┐
│ EP 1  ││ EP 2  ││ EP 3  │   │ EP 1  ││ EP 2  ││ EP 3  │   │ EP 1  ││ EP 2  ││ EP 3  │
│(w:10) ││(w:5)  ││(w:5)  │   │(w:10) ││(w:5)  ││(w:5)  │   │(w:10) ││(w:5)  ││(w:5)  │
└───────┘└───────┘└───────┘   └───────┘└───────┘└───────┘   └───────┘└───────┘└───────┘
    │        │        │             │        │        │             │        │        │
    └────────┴────────┴─────────────┴────────┴────────┴─────────────┴────────┴────────┘
                                             │
                                             ▼
                                    ┌─────────────────┐
                                    │  Health Checker │
                                    │  (Background)   │
                                    └─────────────────┘
```

## Core Components

### 1. HTTP Server

The HTTP server is built on [Fiber](https://gofiber.io/), a fast HTTP framework for Go. It handles:

- HTTP request routing
- WebSocket upgrades
- Middleware chain execution
- Graceful shutdown

**Key files:**
- `cmd/proxy/main.go` - Server initialization and configuration

### 2. Middleware Layer

#### Rate Limiter (`internal/proxy/middleware.go`)

Implements token bucket algorithm for per-IP rate limiting:

- Configurable requests per second
- Burst capacity for handling traffic spikes
- Automatic bucket cleanup for memory efficiency
- Returns HTTP 429 when limit exceeded

#### CORS Middleware

Handles Cross-Origin Resource Sharing:

- Allows all origins (configurable)
- Sets appropriate headers for preflight requests

#### Recovery Middleware

Catches panics and returns structured error responses:

- Prevents server crashes from unhandled errors
- Logs panic details for debugging

### 3. Chain Manager (`internal/chain/chain.go`)

Central coordinator for all blockchain chains:

- Manages chain registration and lookup
- Routes requests to appropriate chains
- Aggregates health statistics

### 4. Chain (`internal/chain/chain.go`)

Represents a single blockchain network:

- Manages multiple endpoints
- Coordinates load balancing
- Tracks highest known block

### 5. Load Balancer (`internal/chain/loadbalancer.go`)

Implements weighted round-robin distribution:

```
Request Distribution (weights: EP1=10, EP2=5, EP3=5):

Round 1-10:  → EP1
Round 11-15: → EP2
Round 16-20: → EP3
Round 21-30: → EP1
... (cycle repeats)
```

**Features:**
- Only routes to healthy endpoints
- Automatic fallback when no healthy endpoints
- Thread-safe using RWMutex

### 6. Endpoint (`internal/chain/endpoint.go`)

Represents a single RPC endpoint:

- HTTP client with configurable timeouts
- Health status tracking (atomic operations)
- Block height monitoring
- Latency measurement
- Request/failure counters
- Optional circuit breaker integration

### 7. Health Checker (`internal/chain/health.go`)

Background health monitoring:

```
┌─────────────────┐     ┌─────────────────┐
│  Health Checker │────▶│ eth_blockNumber │
│   (Periodic)    │     │    Request      │
└────────┬────────┘     └────────┬────────┘
         │                       │
         │                       ▼
         │              ┌─────────────────┐
         │              │ Parse Response  │
         │              └────────┬────────┘
         │                       │
         ▼                       ▼
┌─────────────────┐     ┌─────────────────┐
│ Update Metrics  │◄────│ Compare Blocks  │
└─────────────────┘     └─────────────────┘
```

**Health criteria:**
1. Endpoint responds to `eth_blockNumber`
2. Response time within timeout
3. Block height not lagging by more than `max_block_lag`

### 8. Circuit Breaker (`internal/chain/circuitbreaker.go`)

Implements the circuit breaker pattern for fault isolation:

```
         ┌─────────────────────────────────────────────┐
         │                                             │
         ▼                                             │
    ┌─────────┐    Failures > Threshold    ┌─────────┐│
    │ CLOSED  │──────────────────────────▶ │  OPEN   ││
    │(Normal) │                            │ (Trip)  ││
    └────┬────┘                            └────┬────┘│
         │                                      │     │
         │  Success                     Timeout │     │
         │                                      │     │
         │                                      ▼     │
         │                              ┌───────────┐ │
         │◀─────────────────────────────│ HALF-OPEN │─┘
         │     Successes > Threshold    │  (Test)   │
         │                              └───────────┘
```

**States:**
- **CLOSED**: Normal operation, requests pass through
- **OPEN**: Failures exceeded threshold, requests fail fast
- **HALF-OPEN**: Testing recovery, limited requests allowed

### 9. Cache (`internal/cache/cache.go`)

Two-tier caching system:

#### In-Memory Cache
- Hash-based key generation
- Configurable TTL
- Method-based cacheability

#### Redis Cache (Optional)
- Distributed caching for multi-instance deployments
- Same interface as in-memory cache

**Cacheable methods (default):**
- `eth_chainId` - Chain identifier
- `net_version` - Network version
- `web3_clientVersion` - Client version

### 10. WebSocket Handler (`internal/websocket/handler.go`)

Full WebSocket support for subscriptions:

- Client connection management
- Backend connection pooling
- Bidirectional message forwarding
- Automatic reconnection with backoff
- Ping/pong for connection health

### 11. Metrics (`internal/metrics/prometheus.go`)

Prometheus metrics for observability:

| Metric Type | Examples |
|-------------|----------|
| Counter | requests_total, errors_total, cache_hits |
| Gauge | healthy_endpoints, block_height |
| Histogram | request_duration_seconds |

## Request Flow

### HTTP RPC Request

```
1. Client sends POST /:chain with JSON-RPC body
2. Rate limiter checks IP token bucket
3. Handler extracts chain name from path
4. Chain manager looks up chain
5. Cache checks if method is cacheable and cached
6. If cache miss, load balancer selects endpoint
7. Endpoint forwards request to backend
8. Response recorded in cache (if cacheable)
9. Metrics recorded
10. Response sent to client
```

### WebSocket Connection

```
1. Client connects to /ws/:chain
2. Handler upgrades HTTP to WebSocket
3. Session created with client connection
4. Backend WebSocket connection established
5. Bidirectional forwarding starts
6. Messages forwarded between client and backend
7. On disconnect, cleanup resources
```

## Design Decisions

### Why Weighted Round-Robin?

Simple yet effective for RPC load balancing:
- Predictable distribution
- Easy to configure weights
- No complex state management
- Works well with health-based filtering

### Why In-Memory Cache First?

- Lowest latency for hot data
- No external dependencies for basic operation
- Redis optional for distributed setups

### Why Circuit Breaker?

- Prevents cascade failures
- Allows endpoints to recover
- Reduces wasted requests to failing endpoints

### Why Fiber over net/http?

- Better performance for high-throughput scenarios
- Built-in middleware support
- Simpler route handling

## Scalability Considerations

### Horizontal Scaling

The proxy is stateless and can be horizontally scaled:

```
┌─────────────────┐
│  Load Balancer  │
│   (External)    │
└────────┬────────┘
         │
    ┌────┴────┬────────┬────────┐
    │         │        │        │
    ▼         ▼        ▼        ▼
┌───────┐ ┌───────┐ ┌───────┐ ┌───────┐
│Proxy 1│ │Proxy 2│ │Proxy 3│ │Proxy N│
└───────┘ └───────┘ └───────┘ └───────┘
```

**Requirements for scaling:**
- Use Redis for distributed caching
- External load balancer for traffic distribution
- Shared configuration (ConfigMap in Kubernetes)

### Resource Requirements

| Deployment Size | CPU | Memory | Connections |
|-----------------|-----|--------|-------------|
| Small (dev)     | 100m | 128Mi | 100 |
| Medium (prod)   | 500m | 256Mi | 1000 |
| Large (high-traffic) | 1000m | 512Mi | 5000 |

## Security Considerations

- Input validation on all RPC requests
- Rate limiting to prevent abuse
- CORS configuration for web clients
- No credential storage (endpoints assumed public)
- Timeout protection against slow endpoints

## Monitoring

### Key Metrics to Watch

1. **Request latency** (`rpc_proxy_request_duration_seconds`)
   - P50, P95, P99 latencies
   - Alert on significant increases

2. **Error rate** (`rpc_proxy_errors_total`)
   - Track by error type
   - Alert on sudden spikes

3. **Endpoint health** (`rpc_proxy_endpoint_healthy`)
   - Monitor healthy/unhealthy ratio
   - Alert when all endpoints unhealthy

4. **Cache hit ratio** (`cache_hits / (cache_hits + cache_misses)`)
   - Target >80% for static methods
   - Tune TTL if ratio drops

5. **Rate limit rejections** (`rpc_proxy_rate_limit_rejects_total`)
   - Monitor for abuse patterns
   - Adjust limits as needed
