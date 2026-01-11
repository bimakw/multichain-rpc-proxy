# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Comprehensive test coverage improvements
- Documentation for architecture, configuration, and API
- Benchmark tests for critical paths

## [1.1.0] - 2025-01-11

### Added
- WebSocket support for `eth_subscribe` and other subscription-based RPC methods
- WebSocket connection pooling for efficient backend connections
- Circuit breaker pattern for improved fault tolerance
- Automatic reconnection with exponential backoff for WebSocket connections
- GitHub Actions CI/CD pipeline with multi-platform builds
- Grafana dashboard JSON for pre-built monitoring visualization
- Codecov integration for test coverage reporting

### Changed
- Improved test coverage from ~50% to ~80% across all packages
- Enhanced health check with more detailed endpoint statistics
- Better error messages with JSON-RPC compliant error codes

### Fixed
- Race condition in load balancer endpoint selection
- Memory leak in rate limiter bucket cleanup
- Proper handling of connection timeouts in health checks

## [1.0.0] - 2024-12-01

### Added
- Multi-chain RPC proxy support (Ethereum, Arbitrum, Optimism, Base, Polygon)
- Weighted round-robin load balancing with automatic failover
- Continuous health monitoring with block height validation
- Block lag detection to mark slow endpoints as unhealthy
- In-memory caching for static RPC calls (eth_chainId, net_version, web3_clientVersion)
- Token bucket rate limiting with per-IP tracking
- Prometheus metrics for comprehensive observability
- Docker and Docker Compose support for containerized deployment
- YAML-based configuration for easy customization
- Makefile for common development tasks
- MIT License with attribution requirement

### Security
- Input validation for all RPC requests
- CORS middleware for cross-origin request handling
- Rate limiting to prevent abuse

[Unreleased]: https://github.com/bimakw/multichain-rpc-proxy/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/bimakw/multichain-rpc-proxy/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/bimakw/multichain-rpc-proxy/releases/tag/v1.0.0
