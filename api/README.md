# Weather Blockchain API

A REST API service for monitoring and interacting with weather blockchain nodes on the local network.

## Overview

This API service runs independently from the blockchain nodes and provides HTTP endpoints to:
- Discover blockchain nodes on the local network using mDNS
- Query blockchain state from multiple nodes
- Compare blockchain states across nodes to detect forks
- Retrieve specific blocks from nodes
- Monitor node health and connectivity

## Features

- **Node Discovery**: Automatically discovers blockchain nodes using mDNS
- **Multi-Node Queries**: Queries multiple nodes simultaneously for comprehensive data
- **Fork Detection**: Compares blockchain states to identify potential forks
- **Real-time Data**: Gets live data directly from blockchain nodes
- **RESTful API**: Standard HTTP endpoints with JSON responses
- **CORS Support**: Cross-origin requests enabled for web frontends

## Installation & Usage

### Building the API Server

```bash
# From the project root directory
cd api
go build -o weather-blockchain-api main.go
```

### Running the API Server

```bash
# Run with default settings (port 8080)
./weather-blockchain-api

# Run on custom port
./weather-blockchain-api --port 9000

# Run with debug logging
./weather-blockchain-api --log-level debug

# Show help
./weather-blockchain-api --help
```

### Running from Source

```bash
# Run directly with Go
go run main.go

# With custom port and debug logging
go run main.go --port 9000 --log-level debug
```

## API Endpoints

### Home & Health

- **GET /** - API information and available endpoints
- **GET /api/health** - Health check and server status

### Node Management

- **GET /api/nodes** - List all discovered blockchain nodes
- **POST /api/nodes/discover** - Manually trigger node discovery

### Blockchain Data

- **GET /api/blockchain/info** - Get blockchain information from all nodes
- **GET /api/blockchain/blocks/{index}** - Get specific block by index
  - Query parameter: `?node={nodeId}` to query specific node
- **GET /api/blockchain/compare** - Compare blockchain states across all nodes

## API Response Format

All API responses follow this standard format:

```json
{
  "success": true,
  "data": {
    // Response data
  },
  "message": "Operation description",
  "error": null
}
```

## Example Usage

### Discover Nodes

```bash
curl http://localhost:8080/api/nodes
```

```json
{
  "success": true,
  "data": {
    "nodes": [
      {
        "node_id": "node-1-abc123",
        "address": "192.168.1.100",
        "port": "8001",
        "last_seen": "2025-06-21T10:30:00Z",
        "block_height": 25,
        "latest_hash": "abc123def456..."
      }
    ],
    "node_count": 1,
    "timestamp": "2025-06-21T10:30:00Z"
  },
  "message": "Found 1 blockchain nodes"
}
```

### Get Blockchain Info

```bash
curl http://localhost:8080/api/blockchain/info
```

```json
{
  "success": true,
  "data": {
    "nodes": {
      "node-1-abc123": {
        "blockchain_info": {
          "total_blocks": 26,
          "latest_hash": "abc123def456...",
          "genesis_hash": "",
          "last_updated": "2025-06-21T10:30:00Z",
          "chain_valid": true
        },
        "node_info": {
          "node_id": "node-1-abc123",
          "address": "192.168.1.100",
          "port": "8001"
        }
      }
    },
    "total_nodes": 1,
    "errors": 0,
    "timestamp": "2025-06-21T10:30:00Z"
  },
  "message": "Retrieved blockchain info from 1 nodes (0 errors)"
}
```

### Get Specific Block

```bash
# Get block 5 from all nodes
curl http://localhost:8080/api/blockchain/blocks/5

# Get block 5 from specific node
curl "http://localhost:8080/api/blockchain/blocks/5?node=node-1-abc123"
```

### Compare Node States

```bash
curl http://localhost:8080/api/blockchain/compare
```

```json
{
  "success": true,
  "data": {
    "node_states": {
      // Individual node states
    },
    "analysis": {
      "hash_distribution": {
        "abc123def456...": 2,
        "def456ghi789...": 1
      },
      "height_distribution": {
        "25": 2,
        "24": 1
      },
      "potential_forks": true,
      "height_consensus": false
    },
    "total_nodes": 3,
    "timestamp": "2025-06-21T10:30:00Z"
  },
  "message": "Blockchain state comparison completed"
}
```

## Architecture

The API service consists of:

- **NodeClient**: Handles mDNS discovery and TCP communication with blockchain nodes
- **Server**: HTTP server with REST endpoints
- **Message Protocol**: Uses the same JSON message format as blockchain nodes

### Network Communication

1. **Discovery**: Uses mDNS to find nodes advertising `_weather_blockchain_p2p_node._tcp`
2. **Communication**: Establishes TCP connections to nodes
3. **Protocol**: Sends JSON messages with BlockRequest/BlockResponse format
4. **Timeout**: 10-second timeout for node responses

## Configuration

### Environment Variables

The API server can be configured using command-line flags:

- `--port` / `-p`: HTTP server port (default: 8080)
- `--log-level` / `-l`: Logging level (default: info)

### Log Levels

- `debug`: Detailed debugging information
- `info`: General operational messages  
- `warn`: Warning messages
- `error`: Error messages only

## Development

### Adding New Endpoints

1. Add handler function to `server.go`
2. Register route in `Start()` method
3. Update this README with endpoint documentation

### Adding New Message Types

1. Define message types in `node_client.go`
2. Implement request/response logic
3. Update blockchain nodes to handle new message types

## Troubleshooting

### Common Issues

1. **No nodes discovered**
   - Ensure blockchain nodes are running on the same network
   - Check that nodes are advertising mDNS services
   - Try manual discovery: `POST /api/nodes/discover`

2. **Connection timeouts**
   - Verify node addresses and ports
   - Check firewall settings
   - Ensure nodes are accepting connections

3. **Empty responses**
   - Check node logs for errors
   - Verify blockchain has blocks
   - Try different nodes

### Debugging

Run with debug logging to see detailed information:

```bash
./weather-blockchain-api --log-level debug
```

This will show:
- mDNS discovery process
- TCP connections to nodes
- Message exchanges
- Response parsing