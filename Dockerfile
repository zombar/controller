# Multi-stage build for optimal image size
FROM golang:1.24-alpine AS builder

# Install minimal build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary (pure Go with modernc.org/sqlite)
RUN GOOS=linux go build -a -ldflags="-w -s" -o controller ./cmd/controller

# Final stage
FROM alpine:3.20

# Install minimal runtime dependencies
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 controller && \
    adduser -D -u 1000 -G controller controller

# Create necessary directories
RUN mkdir -p /app/data && \
    chown -R controller:controller /app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/controller .

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Switch to non-root user
USER controller

# Create volume for persistent data
VOLUME /app/data

# Expose controller API port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the controller service
CMD ["./controller"]
