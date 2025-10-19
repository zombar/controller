# Controller Service

A central orchestration service that coordinates the scraper and textanalyzer microservices, providing a unified API for URL scraping, text analysis, and content search.

## Features

- Orchestrates scraper and textanalyzer services
- Unified API for URL scraping with automatic text analysis
- Asynchronous scrape request processing with progress tracking
- In-memory scrape request management with auto-expiration
- Direct text analysis without scraping
- AI-powered link extraction and filtering
- Batch URL scraping with caching support
- Link quality scoring to filter low-quality content
- Tag-based search with fuzzy matching support
- Image search across scraped content
- SQLite storage with audit trails
- UUID tracking for all service calls
- Designed for PostgreSQL migration

## Requirements

- Go 1.21 or higher
- Running scraper service
- Running textanalyzer service
- SQLite3

## Installation

```bash
# Navigate to directory
cd controller

# Install dependencies
go mod download

# Build the binary
go build -o controller ./cmd/controller

# Run the service
./controller
```

## Usage

### Starting the Service

```bash
# Default configuration
./controller

# Using environment variables
export SCRAPER_BASE_URL=http://localhost:8081
export TEXTANALYZER_BASE_URL=http://localhost:8082
export CONTROLLER_PORT=8080
export DATABASE_PATH=./controller.db
./controller
```

### Environment Variables

- `SCRAPER_BASE_URL` - Scraper service URL (default: http://localhost:8081)
- `TEXTANALYZER_BASE_URL` - TextAnalyzer service URL (default: http://localhost:8082)
- `CONTROLLER_PORT` - HTTP server port (default: 8080)
- `DATABASE_PATH` - SQLite database path (default: ./controller.db)

## Quick Examples

```bash
# Health check
curl http://localhost:8080/health

# Scrape URL and analyze
curl -X POST http://localhost:8080/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'

# Analyze text directly
curl -X POST http://localhost:8080/analyze \
  -H "Content-Type: application/json" \
  -d '{"text": "Your text to analyze..."}'

# Search by tags
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{"tags": ["programming", "web"], "fuzzy": false}'

# Score a link for quality
curl -X POST http://localhost:8080/api/score \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'

# Extract links from a URL
curl -X POST http://localhost:8080/api/extract-links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'

# Create async scrape request
curl -X POST http://localhost:8080/api/scrape-requests \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'

# List scrape requests
curl http://localhost:8080/api/scrape-requests

# Get scrape request status
curl http://localhost:8080/api/scrape-requests/550e8400-e29b-41d4-a716-446655440000

# Retry failed scrape request
curl -X POST http://localhost:8080/api/scrape-requests/550e8400-e29b-41d4-a716-446655440000/retry

# Delete scrape request
curl -X DELETE http://localhost:8080/api/scrape-requests/550e8400-e29b-41d4-a716-446655440000

# Batch scrape multiple URLs
curl -X POST http://localhost:8080/api/scrape/batch \
  -H "Content-Type: application/json" \
  -d '{"urls": ["https://example.com/article-1", "https://example.com/article-2"], "force": false}'

# Search images by tags
curl -X POST http://localhost:8080/api/images/search \
  -H "Content-Type: application/json" \
  -d '{"tags": ["cat", "animal"]}'

# Get request by ID
curl http://localhost:8080/requests/550e8400-e29b-41d4-a716-446655440000

# List all requests
curl "http://localhost:8080/requests?limit=10&offset=0"
```

## Architecture

The controller acts as an orchestrator for the scraper and textanalyzer services:

```
┌──────────┐
│  Client  │
└────┬─────┘
     │
     v
┌────────────┐       ┌──────────┐
│ Controller │──────>│ Scraper  │
│  Service   │       └──────────┘
└────┬───────┘
     │              ┌──────────────┐
     └─────────────>│TextAnalyzer  │
                    └──────────────┘
```

### Workflow

**URL Scraping Flow:**
1. Client sends URL to controller
2. Controller calls scraper service
3. Scraper returns extracted content
4. Controller sends content to textanalyzer
5. Controller stores all UUIDs and tags
6. Controller returns combined result

**Direct Analysis Flow:**
1. Client sends text to controller
2. Controller calls textanalyzer service
3. Controller stores UUID and tags
4. Controller returns analysis result

**Async Scrape Request Flow:**
1. Client creates scrape request
2. Controller returns request with UUID and pending status
3. Background goroutine processes request asynchronously
4. Client polls for status updates with progress percentage
5. On completion, result is stored and linked to scrape request
6. Requests auto-expire after 15 minutes

## Output Format

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "created_at": "2025-10-17T12:34:56.789Z",
  "source_type": "url",
  "source_url": "https://example.com/article",
  "scraper_uuid": "abc123-scraper-uuid",
  "textanalyzer_uuid": "def456-analyzer-uuid",
  "tags": ["technology", "programming", "web"],
  "metadata": {
    "scraper_metadata": {
      "title": "Example Article"
    },
    "analyzer_metadata": {
      "sentiment": "positive"
    }
  }
}
```

**Fields:**
- `id` - Unique controller request ID
- `created_at` - Request timestamp
- `source_type` - Either "url" or "text"
- `source_url` - Original URL (if source_type is "url")
- `scraper_uuid` - UUID from scraper service
- `textanalyzer_uuid` - UUID from textanalyzer service
- `tags` - AI-generated tags from textanalyzer
- `metadata` - Combined metadata from both services

## Development

### Make Commands

```bash
make build          # Build binary
make test           # Run tests
make test-coverage  # Generate coverage
make run            # Start server
make dev            # Development mode
make clean          # Clean artifacts
make fmt            # Format code
make lint           # Run linter
```

### Running Tests

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Using Go directly
go test ./...
go test -v ./...
go test -cover ./...

# Test specific packages
go test ./internal/storage
go test ./internal/handlers
go test ./internal/clients
```

### Project Structure

```
controller/
├── cmd/
│   └── controller/
│       └── main.go              # Application entry point
├── internal/
│   ├── clients/
│   │   ├── scraper.go          # Scraper service client
│   │   └── textanalyzer.go     # TextAnalyzer client
│   ├── config/
│   │   ├── config.go           # Configuration management
│   │   └── config_test.go      # Config tests
│   ├── handlers/
│   │   ├── handlers.go         # HTTP handlers
│   │   └── handlers_test.go    # Handler tests
│   ├── scraper_requests/
│   │   ├── scraper_requests.go # In-memory scrape request manager
│   │   └── scraper_requests_test.go # Manager tests
│   └── storage/
│       ├── migrations.go       # Database migrations
│       ├── storage.go          # Database operations
│       └── storage_test.go     # Storage tests
├── .env.example                # Example configuration
├── README.md                   # This file
└── API.md                      # API reference
```

## Database

### Schema

**requests table:**
- `id` - UUID primary key
- `created_at` - Timestamp
- `source_type` - "url" or "text"
- `source_url` - Original URL (nullable)
- `scraper_uuid` - Scraper service UUID (nullable)
- `textanalyzer_uuid` - TextAnalyzer UUID
- `tags_json` - JSON array of tags
- `metadata_json` - JSON metadata object

**tags table:**
- `id` - Auto-increment primary key
- `request_id` - Foreign key to requests.id
- `tag` - Individual tag value

### Migrations

The database schema is managed through a migration system in `internal/storage/migrations.go`.

To add a new migration:
1. Add a new Migration struct to the migrations slice
2. Increment the version number
3. Provide SQL statement
4. Restart the service

### Switching to PostgreSQL

To use PostgreSQL instead of SQLite:

1. Update `internal/storage/storage.go`:
   ```go
   import _ "github.com/lib/pq"

   func New(connectionString string) (*Storage, error) {
       db, err := sql.Open("postgres", connectionString)
       // ... rest remains the same
   }
   ```

2. Update migrations:
   - `AUTOINCREMENT` → `SERIAL`
   - `DATETIME` → `TIMESTAMP`

3. Use PostgreSQL connection string:
   ```bash
   export DATABASE_PATH="postgres://user:pass@localhost:5432/controller?sslmode=disable"
   ./controller
   ```

## Performance Considerations

- HTTP client timeouts configured for service dependencies
- Database connection pooling for concurrent requests
- Tag search uses indexed queries
- Fuzzy tag matching uses LIKE queries

## API Documentation

See [API.md](API.md) for complete API reference including:
- Endpoint specifications
- Request/response formats
- Error handling
- Code examples
- Integration patterns

## License

This project is licensed under the MIT License - see the [LICENSE](../../LICENSE) file for details.
