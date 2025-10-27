# Controller Service

[![Go Report Card](https://goreportcard.com/badge/github.com/zombar/purpletab)](https://goreportcard.com/report/github.com/zombar/purpletab)
[![Go Version](https://img.shields.io/github/go-mod/go-version/zombar/purpletab)](go.mod)

A central orchestration service that coordinates the scraper and textanalyzer microservices, providing a unified API for URL scraping, text analysis, and content search.

## Features

- Orchestrates scraper and textanalyzer services
- Unified API for URL scraping with automatic text analysis
- **Persistent task queue with Asynq and Redis**
- **Asynchronous scrape request processing with database persistence**
- **Configurable worker concurrency and retry policies**
- **Queue monitoring UI via Asynqmon**
- Direct text analysis without scraping
- AI-powered link extraction and filtering
- Batch URL scraping with caching support
- Link quality scoring to filter low-quality content
- Tag-based search with fuzzy matching support
- Image search across scraped content
- PostgreSQL storage with audit trails
- UUID tracking for all service calls
- Connection pooling and OpenTelemetry instrumentation

## Requirements

- Go 1.21 or higher
- Running scraper service
- Running textanalyzer service
- **Redis 6.0 or higher (for task queue)**
- PostgreSQL 16 or higher

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
- `SCHEDULER_BASE_URL` - Scheduler service URL (default: http://localhost:8083)
- `CONTROLLER_PORT` - HTTP server port (default: 8080)
- **`REDIS_ADDR` - Redis server address (default: localhost:6379)**
- **`WORKER_CONCURRENCY` - Number of concurrent queue workers (default: 10)**
- `LINK_SCORE_THRESHOLD` - Minimum link quality score 0.0-1.0 (default: 0.5)
- `WEB_INTERFACE_URL` - Web interface URL for SEO links (default: http://localhost:5173)
- `DB_HOST` - PostgreSQL host (default: postgres)
- `DB_PORT` - PostgreSQL port (default: 5432)
- `DB_USER` - Database user (default: docutab)
- `DB_PASSWORD` - Database password
- `DB_NAME` - Database name (default: docutab)

### Tombstone Configuration

- **`TOMBSTONE_TAGS`** - Comma-separated list of tags that trigger auto-tombstoning (default: `low-quality,sparse-content`)
- **`TOMBSTONE_PERIOD_LOW_SCORE`** - Days until deletion for low-score URLs (default: 30)
- **`TOMBSTONE_PERIOD_TAG_BASED`** - Days until deletion for tagged content (default: 90)
- **`TOMBSTONE_PERIOD_MANUAL`** - Days until deletion for manual tombstones (default: 90)

The tombstone system automatically marks low-quality content for deletion:
- **Low-score rejection**: URLs scored below `LINK_SCORE_THRESHOLD` are tombstoned immediately
- **Tag-based tombstoning**: Content tagged with any tag in `TOMBSTONE_TAGS` is tombstoned when tags are updated
- **Manual tombstoning**: Content manually marked via API endpoints

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

The controller acts as an orchestrator for the scraper and textanalyzer services, using Redis-backed queues for async processing:

```
┌──────────┐
│  Client  │
└────┬─────┘
     │
     v
┌────────────┐       ┌──────────┐       ┌──────────────┐
│ Controller │──────>│ Scraper  │       │ Asynq Queue  │
│   API      │       └──────────┘       │   (Redis)    │
└────┬───────┘              ^            └──────┬───────┘
     │                      │                   │
     │              ┌──────────────┐            │
     └─────────────>│TextAnalyzer  │            v
                    └──────────────┘    ┌───────────────┐
                                        │Queue Workers  │
                                        │(Concurrent)   │
                                        └───────────────┘
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

**Async Scrape Request Flow (Queue-based):**
1. Client creates scrape request
2. Controller creates job in database and enqueues to Redis
3. Controller returns job with UUID and "queued" status
4. Asynq worker picks up task from queue
5. Worker processes scrape → analyze → store workflow
6. Job status updates: queued → processing → completed/failed
7. Failed jobs retry with exponential backoff (1min, 5min, 15min)
8. Client polls for status updates via job ID
9. Completed jobs persist in database with result linkage

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
│   │   ├── textanalyzer.go     # TextAnalyzer client
│   │   └── scheduler.go        # Scheduler client
│   ├── config/
│   │   ├── config.go           # Configuration management
│   │   └── config_test.go      # Config tests
│   ├── handlers/
│   │   ├── handlers.go         # HTTP handlers
│   │   ├── handlers_test.go    # Handler tests
│   │   └── seo.go              # SEO-related handlers
│   ├── queue/
│   │   ├── client.go           # Asynq queue client
│   │   ├── worker.go           # Asynq queue worker
│   │   ├── tasks.go            # Task handlers
│   │   └── tasks_test.go       # Task tests
│   ├── scraper_requests/
│   │   ├── scraper_requests.go # In-memory manager (text analysis only)
│   │   └── scraper_requests_test.go # Manager tests
│   ├── storage/
│   │   ├── migrations.go       # Database migrations
│   │   ├── storage.go          # Database operations
│   │   ├── scrape_jobs.go      # Scrape job persistence
│   │   └── storage_test.go     # Storage tests
│   └── slug/
│       └── slug.go             # URL slug generation
├── .env.example                # Example configuration
├── README.md                   # This file
└── API.md                      # API reference
```

## Database

PostgreSQL database with automatic schema migrations via `internal/storage/migrations.go`.

### Schema

**requests table:**
- `id` - UUID primary key
- `created_at` - Timestamp
- `effective_date` - Date used for timeline (from metadata or created_at)
- `source_type` - "url" or "text"
- `source_url` - Original URL (nullable)
- `scraper_uuid` - Scraper service UUID (nullable)
- `textanalyzer_uuid` - TextAnalyzer UUID
- `tags_json` - JSON array of tags
- `metadata_json` - JSON metadata object
- `slug` - SEO-friendly URL slug
- `seo_enabled` - Boolean flag for SEO page generation

**tags table:**
- `id` - Serial primary key
- `request_id` - Foreign key to requests.id
- `tag` - Individual tag value

**scrape_jobs table (queue persistence):**
- `id` - UUID primary key (job ID)
- `url` - URL to scrape
- `extract_links` - Boolean for recursive link extraction
- `status` - Job status: queued, processing, completed, failed
- `retries` - Retry attempt count
- `created_at` - Job creation timestamp
- `updated_at` - Last update timestamp
- `completed_at` - Completion timestamp
- `error_message` - Error details for failed jobs
- `result_request_id` - Foreign key to requests.id (result)
- `asynq_task_id` - Asynq task ID for correlation

The shared database package (`pkg/database`) provides connection pooling, OpenTelemetry instrumentation, and automatic retry logic.

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
