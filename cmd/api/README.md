# Weather Blockchain API

REST API server for monitoring weather blockchain nodes on the local network.

## Overview

The Weather Blockchain API provides HTTP endpoints to discover and query blockchain nodes. It runs independently from blockchain nodes and offers real-time monitoring capabilities.

### Installation

```bash
# Install dependencies
make deps

# Build the binary
make build
```

## Usage

### Available Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build the API server binary |
| `make run` | Run the API server (default port 8080) |
| `make run-debug` | Run with debug logging |
| `make run-port` | Run on custom port (set PORT=xxxx) |
| `make start` | Build and start the binary |
| `make clean` | Remove build artifacts |
| `make test` | Test API endpoints (requires server running) |
| `make deps` | Install Go dependencies |
| `make help` | Show help message |

### Running the Server

```bash
# Default configuration (port 8080, info logging)
make run

# Debug logging
make run-debug

# Custom port
make PORT=9000 run-port

# Build and start binary
make start
```

### Manual Execution

```bash
# Build first
make build

# Run binary with options
./weather-blockchain-api --port 8080 --log-level info
./weather-blockchain-api --port 9000 --log-level debug
```

## Configuration

### Command Line Options

- `--port` / `-p`: HTTP server port (default: 8080)
- `--log-level` / `-l`: Log level - debug, info, warn, error (default: info)

### Environment Variables

Set in Makefile or override:
- `PORT`: Server port (default: 8080)
- `LOG_LEVEL`: Logging level (default: info)

## Testing

### Automated Testing

```bash
# Start server in one terminal
make run

# Test endpoints in another terminal
make test
```

The test target checks:
- Health endpoint: `GET /api/health`
- Node discovery: `GET /api/nodes`

### Manual Testing

```bash
# Health check
curl http://localhost:8080/api/health

# Node discovery
curl http://localhost:8080/api/nodes

# Pretty print with jq
curl -s http://localhost:8080/api/health | jq '.'
```

## API Endpoints

### Core Endpoints

- `GET /api/health` - Server health status
- `GET /api/nodes` - Discovered blockchain nodes
- `POST /api/nodes/discover` - Trigger node discovery
- `GET /api/blockchain/info` - Blockchain information from all nodes
- `GET /api/blockchain/blocks/{index}` - Get block by index
- `GET /api/blockchain/height` - Get blockchain height from all nodes
- `GET /api/blockchain/latest/{n}` - Get the n latest blocks from blockchain
- `GET /api/blockchain/compare` - Compare blockchain states

## Development

### Build Process

```bash
# Clean previous builds
make clean

# Install/update dependencies
make deps

# Build binary
make build

# Verify build
ls -la weather-blockchain-api
```

### Development Workflow

1. Make code changes
2. `make clean` - Clean old artifacts
3. `make build` - Build new binary  
4. `make run-debug` - Test with debug logging
5. `make test` - Run endpoint tests

### Debugging

```bash
# Run with debug logging
make run-debug

# Or manually
./weather-blockchain-api --log-level debug
```

Debug logging shows:
- mDNS discovery process
- Node communications
- Request/response details
- Error diagnostics

## Architecture

### Components

- **API Server**: HTTP REST endpoints
- **Node Client**: mDNS discovery and TCP communication
- **Message Protocol**: JSON-based blockchain communication

### Network Discovery

1. Uses mDNS to discover `_weather_blockchain_p2p_node._tcp` services
2. Establishes TCP connections to discovered nodes
3. Exchanges JSON messages for blockchain queries
4. 10-second timeout for node responses

## Troubleshooting

### Common Issues

**Server won't start:**
```bash
# Check if port is in use
make PORT=9000 run-port
```

**No nodes discovered:**
```bash
# Ensure blockchain nodes are running
# Try manual discovery
curl -X POST http://localhost:8080/api/nodes/discover
```

**Build failures:**
```bash
# Clean and rebuild
make clean
make deps  
make build
```

### Log Analysis

```bash
# Debug mode for detailed logs
make run-debug

# Check specific endpoints
make test
```

## Examples

### Basic Usage

```bash
# Terminal 1: Start server
cd api
make run

# Terminal 2: Test API
make test

# Custom usage
make PORT=9000 LOG_LEVEL=debug run-port
```

### Development Cycle

```bash
# Setup
make deps
make build

# Development
make run-debug  # Start with debug logging
make test       # Test in another terminal

# Cleanup
make clean
```