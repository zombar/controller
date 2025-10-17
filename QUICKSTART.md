# Quick Start Guide

This guide will help you get the controller service up and running in minutes.

## Prerequisites

Make sure you have the following services running:

1. **Scraper Service** - Running on port 8081 (or configure with `SCRAPER_BASE_URL`)
2. **Text Analyzer Service** - Running on port 8082 (or configure with `TEXTANALYZER_BASE_URL`)

## Step 1: Install and Build

```bash
# Clone or navigate to the controller directory
cd controller

# Install dependencies
go mod download

# Build the application
go build -o controller ./cmd/controller

# Or use the Makefile
make build
```

## Step 2: Configure Environment

Create a `.env` file (optional - defaults work if services are on standard ports):

```bash
cp .env.example .env
```

Edit `.env` if needed:
```
SCRAPER_BASE_URL=http://localhost:8081
TEXTANALYZER_BASE_URL=http://localhost:8082
CONTROLLER_PORT=8080
DATABASE_PATH=./controller.db
```

## Step 3: Run the Service

```bash
# Run the built binary
./controller

# Or run directly with go
go run ./cmd/controller/main.go

# Or use the Makefile
make run
```

You should see:
```
Starting controller service on port 8080
Scraper service: http://localhost:8081
Text analyzer service: http://localhost:8082
Database: ./controller.db
Applied migration 1: initial_schema
Applied migration 2: add_tags_table
Server listening on :8080
```

## Step 4: Test the Service

### Health Check

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy"
}
```

### Scrape and Analyze a URL

```bash
curl -X POST http://localhost:8080/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://www.bbc.co.uk/news/articles/cy40gq2882xo"
  }'
```

Expected response (example):
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "created_at": "2025-10-17T12:34:56.789Z",
  "source_type": "url",
  "source_url": "https://example.com",
  "scraper_uuid": "abc123-scraper-uuid",
  "textanalyzer_uuid": "def456-analyzer-uuid",
  "tags": ["example", "domain", "web"],
  "metadata": {
    "scraper_metadata": { ... },
    "analyzer_metadata": { ... }
  }
}
```

### Analyze Text Directly

```bash
curl -X POST http://localhost:8080/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Artificial intelligence is transforming software development with new tools and techniques."
  }'
```

### Search by Tags

```bash
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["web"],
    "fuzzy": true
  }'
```

Expected response:
```json
{
  "request_ids": [
    "550e8400-e29b-41d4-a716-446655440000"
  ],
  "count": 1
}
```

### Get Request Details

Use an ID from a previous response:

```bash
curl http://localhost:8080/requests/550e8400-e29b-41d4-a716-446655440000
```

### List All Requests

```bash
curl http://localhost:8080/requests

# With pagination
curl "http://localhost:8080/requests?limit=10&offset=0"
```

## Common Issues

### "Failed to connect to scraper service"

Make sure your scraper service is running and accessible at the configured URL:

```bash
# Test scraper connectivity
curl http://localhost:8081/health
```

### "Failed to connect to text analyzer service"

Make sure your text analyzer service is running:

```bash
# Test text analyzer connectivity
curl http://localhost:8082/health
```

### Port Already in Use

If port 8080 is in use, change it:

```bash
export CONTROLLER_PORT=9090
./controller
```

### Database Issues

If you see database errors, try removing the old database:

```bash
rm controller.db
./controller  # This will recreate with migrations
```

## Development Workflow

### Run Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run specific package tests
go test ./internal/storage -v
```

### Format and Lint

```bash
make fmt   # Format code
make lint  # Run linter
```

### Clean Up

```bash
make clean  # Remove binaries and test databases
```

## Next Steps

- Read the full [README.md](README.md) for detailed API documentation
- Check out the database schema in `internal/storage/migrations.go`
- Explore the test files to see usage examples
- Consider deploying with Docker (see README.md)

## Example Complete Workflow

```bash
# 1. Start the service
./controller &

# 2. Scrape a URL
CONTROLLER_ID=$(curl -s -X POST http://localhost:8080/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' | jq -r '.id')

echo "Created request: $CONTROLLER_ID"

# 3. Get the details
curl http://localhost:8080/requests/$CONTROLLER_ID | jq

# 4. Search by a tag from the response
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{"tags": ["example"], "fuzzy": false}' | jq

# 5. List all requests
curl http://localhost:8080/requests | jq
```

## Support

For issues or questions:
- Check the [README.md](README.md) documentation
- Review test files for usage examples
- Open an issue in the repository
