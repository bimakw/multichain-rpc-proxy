# API Documentation

This document describes all HTTP endpoints exposed by the Multichain RPC Proxy.

## Base URL

```
http://localhost:8080
```

## Endpoints Overview

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Service information |
| GET | `/health` | Overall health status |
| GET | `/health/:chain` | Chain-specific health |
| GET | `/chains` | List all configured chains |
| POST | `/:chain` | Forward RPC request to chain |

## Service Information

### GET /

Returns basic service information.

**Request:**
```bash
curl http://localhost:8080/
```

**Response:**
```json
{
  "name": "multichain-rpc-proxy",
  "version": "1.0.0",
  "docs": "https://github.com/bimakw/multichain-rpc-proxy"
}
```

## Health Endpoints

### GET /health

Returns overall service health status.

**Request:**
```bash
curl http://localhost:8080/health
```

**Response (healthy):**
```json
{
  "status": "healthy",
  "chains": {
    "arbitrum": {
      "healthy": true,
      "total_endpoints": 2,
      "healthy_endpoints": 2,
      "highest_block": 285934567
    },
    "base": {
      "healthy": true,
      "total_endpoints": 2,
      "healthy_endpoints": 2,
      "highest_block": 23456789
    },
    "ethereum": {
      "healthy": true,
      "total_endpoints": 3,
      "healthy_endpoints": 3,
      "highest_block": 19234567
    },
    "optimism": {
      "healthy": true,
      "total_endpoints": 2,
      "healthy_endpoints": 2,
      "highest_block": 117654321
    },
    "polygon": {
      "healthy": true,
      "total_endpoints": 2,
      "healthy_endpoints": 2,
      "highest_block": 54321098
    }
  }
}
```

**Response (degraded):**
```json
{
  "status": "degraded",
  "chains": {
    "ethereum": {
      "healthy": true,
      "total_endpoints": 3,
      "healthy_endpoints": 2,
      "highest_block": 19234567
    }
  }
}
```

**Status codes:**
- `200 OK` - All chains healthy
- `503 Service Unavailable` - One or more chains have issues

### GET /health/:chain

Returns detailed health information for a specific chain.

**Request:**
```bash
curl http://localhost:8080/health/ethereum
```

**Response:**
```json
{
  "chain": "ethereum",
  "chain_id": 1,
  "healthy": true,
  "highest_block": 19234567,
  "endpoints": [
    {
      "url": "https://eth.llamarpc.com",
      "healthy": true,
      "block_height": 19234567,
      "latency_ms": 45.2,
      "requests": 15234,
      "failures": 12
    },
    {
      "url": "https://rpc.ankr.com/eth",
      "healthy": true,
      "block_height": 19234565,
      "latency_ms": 78.5,
      "requests": 7621,
      "failures": 5
    },
    {
      "url": "https://ethereum.publicnode.com",
      "healthy": true,
      "block_height": 19234566,
      "latency_ms": 62.1,
      "requests": 7613,
      "failures": 8
    }
  ]
}
```

**Status codes:**
- `200 OK` - Chain found
- `404 Not Found` - Chain does not exist

**Error response:**
```json
{
  "error": "chain not found: invalid_chain"
}
```

## Chain Listing

### GET /chains

Returns list of all configured chains.

**Request:**
```bash
curl http://localhost:8080/chains
```

**Response:**
```json
{
  "chains": [
    {
      "name": "ethereum",
      "chain_id": 1,
      "endpoints": 3,
      "healthy_endpoints": 3
    },
    {
      "name": "arbitrum",
      "chain_id": 42161,
      "endpoints": 2,
      "healthy_endpoints": 2
    },
    {
      "name": "optimism",
      "chain_id": 10,
      "endpoints": 2,
      "healthy_endpoints": 2
    },
    {
      "name": "base",
      "chain_id": 8453,
      "endpoints": 2,
      "healthy_endpoints": 2
    },
    {
      "name": "polygon",
      "chain_id": 137,
      "endpoints": 2,
      "healthy_endpoints": 2
    }
  ]
}
```

## RPC Proxy Endpoint

### POST /:chain

Forwards JSON-RPC request to the specified chain.

**URL Parameters:**
- `:chain` - Chain name (e.g., `ethereum`, `arbitrum`)

**Headers:**
- `Content-Type: application/json` (required)

**Request body:** Standard JSON-RPC 2.0 request

### Examples

#### Get Block Number

```bash
curl -X POST http://localhost:8080/ethereum \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_blockNumber",
    "params": [],
    "id": 1
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x125a95f"
}
```

#### Get Chain ID

```bash
curl -X POST http://localhost:8080/ethereum \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_chainId",
    "params": [],
    "id": 1
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x1"
}
```

#### Get Balance

```bash
curl -X POST http://localhost:8080/ethereum \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getBalance",
    "params": ["0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045", "latest"],
    "id": 1
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x1bc16d674ec80000"
}
```

#### Call Contract

```bash
curl -X POST http://localhost:8080/ethereum \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_call",
    "params": [
      {
        "to": "0xdAC17F958D2ee523a2206206994597C13D831ec7",
        "data": "0x06fdde03"
      },
      "latest"
    ],
    "id": 1
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000065465746865720000000000000000000000000000000000000000000000000000"
}
```

#### Get Transaction by Hash

```bash
curl -X POST http://localhost:8080/ethereum \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getTransactionByHash",
    "params": ["0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b"],
    "id": 1
  }'
```

#### Get Logs

```bash
curl -X POST http://localhost:8080/ethereum \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getLogs",
    "params": [{
      "fromBlock": "0x125a900",
      "toBlock": "0x125a950",
      "address": "0xdAC17F958D2ee523a2206206994597C13D831ec7"
    }],
    "id": 1
  }'
```

### Multi-Chain Examples

#### Arbitrum

```bash
curl -X POST http://localhost:8080/arbitrum \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_blockNumber",
    "params": [],
    "id": 1
  }'
```

#### Optimism

```bash
curl -X POST http://localhost:8080/optimism \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_chainId",
    "params": [],
    "id": 1
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0xa"
}
```

#### Base

```bash
curl -X POST http://localhost:8080/base \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_chainId",
    "params": [],
    "id": 1
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x2105"
}
```

#### Polygon

```bash
curl -X POST http://localhost:8080/polygon \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_chainId",
    "params": [],
    "id": 1
  }'
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x89"
}
```

## Error Responses

### Chain Not Found

```bash
curl -X POST http://localhost:8080/invalid_chain \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

**Response (404):**
```json
{
  "error": "chain not found: invalid_chain"
}
```

### Invalid JSON

```bash
curl -X POST http://localhost:8080/ethereum \
  -H "Content-Type: application/json" \
  -d 'not valid json'
```

**Response (400):**
```json
{
  "jsonrpc": "2.0",
  "id": null,
  "error": {
    "code": -32700,
    "message": "Parse error"
  }
}
```

### Rate Limited

When rate limit is exceeded:

**Response (429):**
```json
{
  "error": "rate limit exceeded"
}
```

### No Healthy Endpoints

When all endpoints are unhealthy:

**Response (503):**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32603,
    "message": "no healthy endpoints available"
  }
}
```

## JSON-RPC Error Codes

| Code | Message | Description |
|------|---------|-------------|
| -32700 | Parse error | Invalid JSON |
| -32600 | Invalid Request | Invalid JSON-RPC request |
| -32601 | Method not found | Method does not exist |
| -32602 | Invalid params | Invalid method parameters |
| -32603 | Internal error | Internal JSON-RPC error |

## Metrics Endpoint

### GET /metrics (port 9090)

Returns Prometheus metrics.

**Request:**
```bash
curl http://localhost:9090/metrics
```

**Response (text/plain):**
```
# HELP rpc_proxy_requests_total Total number of RPC requests
# TYPE rpc_proxy_requests_total counter
rpc_proxy_requests_total{chain="ethereum",method="eth_blockNumber"} 1523
rpc_proxy_requests_total{chain="ethereum",method="eth_chainId"} 4521

# HELP rpc_proxy_request_duration_seconds RPC request duration in seconds
# TYPE rpc_proxy_request_duration_seconds histogram
rpc_proxy_request_duration_seconds_bucket{chain="ethereum",method="eth_blockNumber",status="success",le="0.01"} 1234
...

# HELP rpc_proxy_endpoint_healthy Whether the endpoint is healthy
# TYPE rpc_proxy_endpoint_healthy gauge
rpc_proxy_endpoint_healthy{chain="ethereum",endpoint="https://eth.llamarpc.com"} 1

# HELP rpc_proxy_cache_hits_total Total cache hits
# TYPE rpc_proxy_cache_hits_total counter
rpc_proxy_cache_hits_total{chain="ethereum",method="eth_chainId"} 4200
```

## WebSocket Support

WebSocket connections are supported for subscription-based RPC methods.

### Connect

```javascript
const ws = new WebSocket('ws://localhost:8080/ws/ethereum');

ws.onopen = () => {
  // Subscribe to new blocks
  ws.send(JSON.stringify({
    jsonrpc: '2.0',
    method: 'eth_subscribe',
    params: ['newHeads'],
    id: 1
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Received:', data);
};
```

### Supported Subscription Methods

- `eth_subscribe` - Subscribe to events
- `eth_unsubscribe` - Unsubscribe from events

### Subscription Types

- `newHeads` - New block headers
- `logs` - Contract event logs
- `newPendingTransactions` - New pending transactions

## Usage with Libraries

### ethers.js

```javascript
const { ethers } = require('ethers');

// HTTP Provider
const provider = new ethers.JsonRpcProvider('http://localhost:8080/ethereum');

// Get block number
const blockNumber = await provider.getBlockNumber();
console.log('Block:', blockNumber);

// Get balance
const balance = await provider.getBalance('0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045');
console.log('Balance:', ethers.formatEther(balance));
```

### web3.js

```javascript
const Web3 = require('web3');

const web3 = new Web3('http://localhost:8080/ethereum');

// Get block number
const blockNumber = await web3.eth.getBlockNumber();
console.log('Block:', blockNumber);

// Get chain ID
const chainId = await web3.eth.getChainId();
console.log('Chain ID:', chainId);
```

### Python (web3.py)

```python
from web3 import Web3

w3 = Web3(Web3.HTTPProvider('http://localhost:8080/ethereum'))

# Get block number
block = w3.eth.block_number
print(f'Block: {block}')

# Get balance
balance = w3.eth.get_balance('0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045')
print(f'Balance: {w3.from_wei(balance, "ether")} ETH')
```

### Go (go-ethereum)

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/ethereum/go-ethereum/ethclient"
)

func main() {
    client, err := ethclient.Dial("http://localhost:8080/ethereum")
    if err != nil {
        log.Fatal(err)
    }

    blockNumber, err := client.BlockNumber(context.Background())
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Block:", blockNumber)
}
```
