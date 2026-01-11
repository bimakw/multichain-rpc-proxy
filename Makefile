.PHONY: build run test clean docker docker-up docker-down lint fmt

# Binary name
BINARY=multichain-rpc-proxy
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Build flags
LDFLAGS=-ldflags="-w -s"

all: build

## build: Build the binary
build:
	@echo "Building..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/proxy

## run: Run the application
run:
	$(GOCMD) run ./cmd/proxy -config configs/config.yaml

## test: Run tests
test:
	$(GOTEST) -v -race ./...

## test-cover: Run tests with coverage
test-cover:
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html

## clean: Clean build files
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

## deps: Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## fmt: Format code
fmt:
	$(GOFMT) -s -w .

## lint: Run linter
lint:
	$(GOLINT) run ./...

## docker: Build docker image
docker:
	docker build -t $(BINARY):latest .

## docker-up: Start docker compose
docker-up:
	docker-compose up -d

## docker-down: Stop docker compose
docker-down:
	docker-compose down

## docker-logs: View docker logs
docker-logs:
	docker-compose logs -f proxy

## health: Check health endpoint
health:
	@curl -s http://localhost:8080/health | jq .

## chains: List available chains
chains:
	@curl -s http://localhost:8080/chains | jq .

## test-eth: Test Ethereum RPC
test-eth:
	@curl -s -X POST http://localhost:8080/ethereum \
		-H "Content-Type: application/json" \
		-d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' | jq .

## test-arb: Test Arbitrum RPC
test-arb:
	@curl -s -X POST http://localhost:8080/arbitrum \
		-H "Content-Type: application/json" \
		-d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' | jq .

## help: Show this help
help:
	@echo "Available targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
