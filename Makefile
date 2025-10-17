.PHONY: build test clean run install help

# Display help
help:
	@echo "Available targets:"
	@echo "  build         - Build the application binary"
	@echo "  test          - Run all tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  clean         - Remove build artifacts and test databases"
	@echo "  install       - Download and tidy dependencies"
	@echo "  run           - Build and run the application"
	@echo "  dev           - Run application in development mode"
	@echo "  fmt           - Format Go code"
	@echo "  lint          - Run go vet linter"
	@echo "  help          - Display this help message"

# Build the application
build:
	@echo "Building controller..."
	@go build -o controller ./cmd/controller

# Run tests
test:
	@echo "Running tests..."
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -cover ./...
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f controller
	@rm -f *.db *.db-journal *.db-wal *.db-shm
	@rm -f coverage.out coverage.html
	@find . -name "test_*.db*" -delete

# Install dependencies
install:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Run the application
run: build
	@echo "Running controller..."
	@./controller

# Run with development environment
dev:
	@echo "Running in development mode..."
	@go run ./cmd/controller/main.go

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	@go vet ./...
