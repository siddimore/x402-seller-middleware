.PHONY: build run test coverage clean lint fmt gateway run-gateway docker-gateway build-gateway-all testbackend run-testbackend test-e2e examples e2e

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOFMT=$(GOCMD) fmt
GOMOD=$(GOCMD) mod

# Binary name
BINARY_NAME=x402-server

# Build the project
build:
	$(GOBUILD) -o bin/$(BINARY_NAME) ./cmd/example

# Run the example server
run:
	$(GOCMD) run ./cmd/example

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Format code
fmt:
	$(GOFMT) ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Tidy dependencies
tidy:
	$(GOMOD) tidy

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

# Install dependencies
deps:
	$(GOGET) -v ./...

# Build gateway binary
gateway:
	$(GOBUILD) -o bin/x402-gateway ./cmd/gateway

# Run gateway with example backend
run-gateway:
	$(GOCMD) run ./cmd/gateway -backend=http://localhost:3000 -payment-url=https://pay.example.com

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/example
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/example
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/example
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/example

# Build gateway for all platforms
build-gateway-all:
	GOOS=linux GOARCH=amd64 $(GOBUILD) -o bin/x402-gateway-linux-amd64 ./cmd/gateway
	GOOS=darwin GOARCH=amd64 $(GOBUILD) -o bin/x402-gateway-darwin-amd64 ./cmd/gateway
	GOOS=darwin GOARCH=arm64 $(GOBUILD) -o bin/x402-gateway-darwin-arm64 ./cmd/gateway
	GOOS=windows GOARCH=amd64 $(GOBUILD) -o bin/x402-gateway-windows-amd64.exe ./cmd/gateway

# Build Docker image for gateway
docker-gateway:
	docker build -t x402-gateway:latest -f deploy/docker/Dockerfile .

# Build test backend
testbackend:
	$(GOBUILD) -o bin/testbackend ./cmd/testbackend

# Build examples
examples:
	$(GOBUILD) -o bin/premium-api ./examples/premium-api

# Run test backend server (port 3000)
run-testbackend:
	$(GOCMD) run ./cmd/testbackend

# Run premium-api example (port 8080)
run-example:
	$(GOCMD) run ./examples/premium-api

# Run full E2E test suite
e2e:
	./scripts/e2e-test.sh

# Run end-to-end test (start backend, gateway, and test)
# Run these in separate terminals:
#   Terminal 1: make run-testbackend
#   Terminal 2: make run-gateway
#   Terminal 3: make test-e2e
test-e2e:
	@echo "Testing x402 gateway end-to-end..."
	@echo ""
	@echo "1. Testing exempt path (should succeed):"
	@curl -s http://localhost:8402/health | head -c 200
	@echo ""
	@echo ""
	@echo "2. Testing protected path WITHOUT token (should get 402):"
	@curl -s -w "\nHTTP Status: %{http_code}\n" http://localhost:8402/api/data | head -c 300
	@echo ""
	@echo "3. Testing protected path WITH valid token (should succeed):"
	@curl -s -H "Authorization: Bearer valid_test123" http://localhost:8402/api/data | head -c 300
	@echo ""
	@echo ""
	@echo "✅ End-to-end test complete!"

# Help
help:
	@echo "Available targets:"
	@echo "  build           - Build the example server"
	@echo "  run             - Run the example server"
	@echo "  gateway         - Build the reverse proxy gateway"
	@echo "  run-gateway     - Run the gateway (requires -backend flag)"
	@echo "  docker-gateway  - Build Docker image for gateway"
	@echo "  testbackend     - Build test backend server"
	@echo "  run-testbackend - Run test backend (port 3000)"
	@echo "  test-e2e        - Run end-to-end tests (requires backend & gateway running)"
	@echo "  test            - Run unit tests"
	@echo "  coverage        - Run tests with coverage report"
	@echo "  fmt             - Format code"
	@echo "  lint            - Lint code"
	@echo "  tidy            - Tidy dependencies"
	@echo "  clean           - Clean build artifacts"
	@echo "  deps            - Install dependencies"
	@echo "  build-all       - Build for multiple platforms"
	@echo "  build-gateway-all - Build gateway for multiple platforms"
	@echo ""
	@echo "Quick Start (run in separate terminals):"
	@echo "  Terminal 1: make run-testbackend"
	@echo "  Terminal 2: make run-gateway"
	@echo "  Terminal 3: make test-e2e"
