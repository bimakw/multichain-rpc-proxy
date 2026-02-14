# Multichain RPC Proxy

[![CI](https://github.com/bimakw/multichain-rpc-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/bimakw/multichain-rpc-proxy/actions/workflows/ci.yml)

Multi-chain RPC load balancer and proxy. Weighted round-robin, health checking, circuit breaker, WebSocket, response caching, rate limiting, gRPC, TLS, and hot config reload.

Supports Ethereum, Arbitrum, Optimism, Base, Polygon — add more via `configs/config.yaml`.

## Running

```bash
go run ./cmd/proxy -config configs/config.yaml
```

Or via Docker:

```bash
docker-compose up -d
```

## Usage

```bash
curl -X POST http://localhost:8080/ethereum \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

Health: `GET /health`, `GET /health/{chain}`, `GET /chains`

Metrics: `GET :9090/metrics`

## Docs

- [Architecture](docs/ARCHITECTURE.md)
- [Configuration](docs/CONFIGURATION.md)
- [API Reference](docs/API.md)
- [Changelog](CHANGELOG.md)

## Testing

```bash
make test
```

## License

MIT
