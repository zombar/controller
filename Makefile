.PHONY: build test test-short test-coverage test-seo clean run install help fmt lint vet check bench deps verify coverage-html stats

# Binary name
BINARY_NAME=controller

# Build variables
VERSION?=1.0.0
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Default target
help: ## Display this help message
	@echo "PurpleTab Controller - Available targets:"
	@echo ""
	@echo "Build commands:"
	@echo "  build           - Build the application binary"
	@echo "  build-all       - Build for multiple platforms"
	@echo ""
	@echo "Test commands:"
	@echo "  test                 - Run all tests"
	@echo "  test-short           - Run only fast tests"
	@echo "  test-coverage        - Run tests with coverage report"
	@echo "  test-seo             - Run only SEO-related tests"
	@echo "  test-trace           - Run only trace propagation tests"
	@echo "  test-trace-e2e       - Run E2E trace flow tests (requires Redis)"
	@echo "  test-trace-e2e-docker- Run E2E trace tests with Docker Redis"
	@echo "  bench                - Run benchmark tests"
	@echo "  coverage-html        - Generate HTML coverage report"
	@echo ""
	@echo "Development commands:"
	@echo "  run             - Build and run the application"
	@echo "  dev             - Run application in development mode"
	@echo "  fmt             - Format Go code"
	@echo "  vet             - Run go vet linter"
	@echo "  lint            - Run go vet (alias for vet)"
	@echo "  check           - Run fmt, vet, and test"
	@echo ""
	@echo "Maintenance commands:"
	@echo "  clean           - Remove build artifacts and test databases"
	@echo "  install         - Download and tidy dependencies"
	@echo "  deps            - Download dependencies"
	@echo "  verify          - Verify dependencies"
	@echo "  stats           - Show project statistics"
	@echo ""
	@echo "SEO commands:"
	@echo "  test-seo-quick  - Quick smoke test for SEO endpoints"
	@echo ""

# Build the application
build: ## Build the application binary
	@echo "Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) ./cmd/controller
	@echo "Build complete: $(BINARY_NAME)"

# Build for multiple platforms
build-all: ## Build for multiple platforms
	@echo "Building for all platforms..."
	@mkdir -p dist
	@echo "Building for Linux (amd64)..."
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-amd64 ./cmd/controller
	@echo "Building for Linux (arm64)..."
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-linux-arm64 ./cmd/controller
	@echo "Building for macOS (amd64)..."
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-amd64 ./cmd/controller
	@echo "Building for macOS (arm64)..."
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-darwin-arm64 ./cmd/controller
	@echo "Building for Windows (amd64)..."
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(BINARY_NAME)-windows-amd64.exe ./cmd/controller
	@echo "All builds complete. Check dist/ directory"
	@ls -lh dist/

# Run all tests
test: ## Run all tests
	@echo "Running tests..."
	@TEST_DB_HOST=localhost TEST_DB_PORT=5432 TEST_DB_USER=docutab TEST_DB_PASSWORD=docutab_dev_pass go test -timeout 120s -v ./...

# Run only fast tests
test-short: ## Run only fast tests (skip slow integration tests)
	@echo "Running short tests..."
	@go test -short -v ./...

# Run tests with coverage
test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	@go test -cover ./...
	@go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | grep total
	@echo "Coverage profile: coverage.out"

# Generate HTML coverage report
coverage-html: ## Generate and open HTML coverage report
	@echo "Generating HTML coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@echo "Opening in browser..."
	@open coverage.html 2>/dev/null || xdg-open coverage.html 2>/dev/null || echo "Please open coverage.html manually"

# Test only SEO-related packages
test-seo: ## Run only SEO-related tests
	@echo "Running SEO tests..."
	@go test -v ./internal/seo/...
	@go test -v ./internal/templates/...
	@go test -v -run ".*SEO.*" ./internal/handlers/...

# Test trace propagation
test-trace: ## Run only trace propagation tests
	@echo "Running trace propagation tests..."
	@go test -v -run ".*Trace.*" ./internal/queue/...

# Test E2E trace flow (requires Redis or will skip)
test-trace-e2e: ## Run E2E trace flow tests (requires Redis on localhost:6379 or set TEST_REDIS_ADDR)
	@echo "Running E2E trace flow tests..."
	@echo "Note: Tests requiring Redis will skip if unavailable"
	@TEST_REDIS_ADDR=localhost:6379 go test -v -run ".*E2ETraceFlow.*" ./internal/queue/...

# Test E2E with Docker Redis (starts Redis container if needed)
test-trace-e2e-docker: ## Run E2E trace tests with temporary Docker Redis container
	@echo "Starting temporary Redis container..."
	@docker rm -f test-redis-controller 2>/dev/null || true
	@docker run -d --name test-redis-controller -p 16380:6379 redis:7-alpine
	@echo "Waiting for Redis to be ready..."
	@sleep 2
	@echo "Running E2E trace flow tests..."
	@TEST_REDIS_ADDR=localhost:16380 go test -v -run ".*E2ETraceFlow.*" ./internal/queue/... || (docker rm -f test-redis-controller && exit 1)
	@echo "Stopping Redis container..."
	@docker rm -f test-redis-controller
	@echo "Tests complete"

# Quick SEO smoke test (requires server running)
test-seo-quick: ## Quick smoke test for SEO endpoints (server must be running on :8080)
	@echo "Testing SEO endpoints..."
	@echo "1. Testing robots.txt..."
	@curl -s http://localhost:8080/robots.txt | head -5 || echo "❌ Server not running?"
	@echo ""
	@echo "2. Testing sitemap.xml..."
	@curl -s http://localhost:8080/sitemap.xml | head -5 || echo "❌ Server not running?"
	@echo ""
	@echo "3. Testing images-sitemap.xml..."
	@curl -s http://localhost:8080/images-sitemap.xml | head -5 || echo "❌ Server not running?"
	@echo ""
	@echo "Run 'make dev' in another terminal to start the server first"

# Run benchmark tests
bench: ## Run benchmark tests
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...

# Clean build artifacts
clean: ## Remove build artifacts and test databases
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -rf dist/
	@rm -f *.db *.db-journal *.db-wal *.db-shm
	@rm -f coverage.out coverage.html
	@find . -name "test_*.db*" -delete
	@echo "Clean complete"

# Install dependencies
install: ## Download and tidy dependencies
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy
	@echo "Dependencies installed"

# Download dependencies
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@go mod download
	@echo "Dependencies downloaded"

# Verify dependencies
verify: ## Verify dependencies
	@echo "Verifying dependencies..."
	@go mod verify
	@echo "Dependencies verified"

# Run the application
run: build ## Build and run the application
	@echo "Running $(BINARY_NAME)..."
	@./$(BINARY_NAME)

# Run with development environment
dev: ## Run application in development mode
	@echo "Running in development mode..."
	@go run ./cmd/controller/main.go

# Format code
fmt: ## Format Go code
	@echo "Formatting code..."
	@go fmt ./...
	@echo "Code formatted"

# Run go vet
vet: ## Run go vet linter
	@echo "Running go vet..."
	@go vet ./...
	@echo "Vet complete"

# Lint code (alias for vet)
lint: vet ## Run linter (alias for vet)

# Run all quality checks
check: fmt vet test ## Run all checks (fmt, vet, test)
	@echo "All checks passed!"

# Show project statistics
stats: ## Show project statistics
	@echo "Controller Project Statistics:"
	@echo "=============================="
	@echo "Go files:"
	@find . -name "*.go" -not -path "./vendor/*" | wc -l | xargs echo "  "
	@echo "Total lines of Go code:"
	@find . -name "*.go" -not -path "./vendor/*" -exec cat {} \; | wc -l | xargs echo "  "
	@echo "Test files:"
	@find . -name "*_test.go" -not -path "./vendor/*" | wc -l | xargs echo "  "
	@echo "Packages:"
	@go list ./... | wc -l | xargs echo "  "
	@echo ""
	@echo "SEO-specific packages:"
	@go list ./... | grep -E "(seo|templates)" | wc -l | xargs echo "  "

# Update dependencies to latest
update-deps: ## Update dependencies to latest versions
	@echo "Updating dependencies..."
	@go get -u ./...
	@go mod tidy
	@echo "Dependencies updated. Run 'make test' to verify."
