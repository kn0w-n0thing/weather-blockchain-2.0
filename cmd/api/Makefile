# Weather Blockchain API Makefile

# Variables
BINARY_NAME=weather-blockchain-api
MAIN_FILE=main.go
PORT=8080
LOG_LEVEL=info

# Default target
.PHONY: all
all: build

# Build the API server
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	go build -o $(BINARY_NAME) $(MAIN_FILE)
	@echo "Build complete: $(BINARY_NAME)"

# Run the API server
.PHONY: run
run:
	@echo "Running API server on port $(PORT)..."
	go run $(MAIN_FILE) --port $(PORT) --log-level $(LOG_LEVEL)

# Run with debug logging
.PHONY: run-debug
run-debug:
	@echo "Running API server with debug logging..."
	go run $(MAIN_FILE) --port $(PORT) --log-level debug

# Run on custom port
.PHONY: run-port
run-port:
	@echo "Running API server on port $(PORT)..."
	go run $(MAIN_FILE) --port $(PORT) --log-level $(LOG_LEVEL)

# Run the built binary
.PHONY: start
start: build
	@echo "Starting $(BINARY_NAME) on port $(PORT)..."
	./$(BINARY_NAME) --port $(PORT) --log-level $(LOG_LEVEL)

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BINARY_NAME)
	@echo "Clean complete"

# Test the API endpoints
.PHONY: test
test:
	@echo "Testing API endpoints..."
	@echo "Health check:"
	curl -s http://localhost:$(PORT)/api/health | jq '.' || echo "Server not running or jq not installed"
	@echo "\nNode discovery:"
	curl -s http://localhost:$(PORT)/api/nodes | jq '.' || echo "Server not running or jq not installed"

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy
	@echo "Dependencies installed"

# Help target
.PHONY: help
help:
	@echo "Weather Blockchain API Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build       - Build the API server binary"
	@echo "  run         - Run the API server (default port 8080)"
	@echo "  run-debug   - Run with debug logging"
	@echo "  run-port    - Run on custom port (set PORT=xxxx)"
	@echo "  clean       - Remove build artifacts"
	@echo "  test        - Test API endpoints (requires server running)"
	@echo "  deps        - Install Go dependencies"
	@echo "  help        - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make run"
	@echo "  make run-debug"
	@echo "  make PORT=9000 run-port"
	@echo "  make test"