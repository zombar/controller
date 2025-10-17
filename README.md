# Controller Service

A central coordinating service that orchestrates the `scraper` and `textanalyzer` microservices. The controller service manages workflows for URL scraping, text analysis, and provides search capabilities across processed content.

## Features

- **URL Scraping Workflow**: Send a URL to be scraped, with the main text automatically passed to the text analyzer
- **Direct Text Analysis**: Send text directly for analysis without scraping
- **Tag Search**: Search for content by AI-generated tags with fuzzy matching support
- **Metadata Tracking**: All processing UUIDs and timestamps are stored for audit trails
- **RESTful API**: Simple HTTP JSON API for all operations

## Architecture

The controller service acts as an orchestrator:

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

### Technology Stack

- **Language**: Go 1.21+
- **Database**: SQLite (with PostgreSQL migration path)
- **Dependencies**:
  - `github.com/mattn/go-sqlite3` - SQLite driver
  - `github.com/google/uuid` - UUID generation

## Installation

### Prerequisites

- Go 1.21 or higher
- SQLite3
- Running instances of `scraper` and `textanalyzer` services

### Build

```bash
# Install dependencies
go mod download

# Build the binary
go build -o controller ./cmd/controller

# Or run directly
go run ./cmd/controller/main.go
```

## Configuration

Configuration is managed through environment variables. Copy `.env.example` to `.env` and adjust as needed:

```bash
cp .env.example .env
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SCRAPER_BASE_URL` | `http://localhost:8081` | Base URL for the scraper service |
| `TEXTANALYZER_BASE_URL` | `http://localhost:8082` | Base URL for the text analyzer service |
| `CONTROLLER_PORT` | `8080` | Port for the controller service to listen on |
| `DATABASE_PATH` | `./controller.db` | Path to SQLite database file |

## API Documentation

### Base URL

All endpoints are relative to `http://localhost:8080` (or your configured port).

### Endpoints

#### 1. Health Check

Check if the service is running.

**Request:**
```bash
curl -X GET http://localhost:8080/health
```

**Response:**
```json
{
  "status": "healthy"
}
```

---

#### 2. Scrape URL and Analyze

Scrape a URL and automatically analyze the extracted text.

**Request:**
```bash
curl -X POST http://localhost:8080/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/article"
  }'
```

**Response:**
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
      "language": "en"
    }
  }
}
```

---

#### 3. Analyze Text Directly

Send text directly for analysis without scraping.

**Request:**
```bash
curl -X POST http://localhost:8080/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "text": "This is some text that needs to be analyzed for AI tagging."
  }'
```

**Response:**
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "created_at": "2025-10-17T12:35:10.123Z",
  "source_type": "text",
  "textanalyzer_uuid": "ghi789-analyzer-uuid",
  "tags": ["analysis", "ai", "tagging"],
  "metadata": {
    "analyzer_metadata": {
      "language": "en",
      "confidence": 0.95
    }
  }
}
```

---

#### 4. Search by Tags

Search for requests by tags with optional fuzzy matching.

**Request (Exact Match):**
```bash
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["programming", "web"],
    "fuzzy": false
  }'
```

**Request (Fuzzy Match):**
```bash
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["prog"],
    "fuzzy": true
  }'
```

**Response:**
```json
{
  "request_ids": [
    "550e8400-e29b-41d4-a716-446655440000",
    "660e8400-e29b-41d4-a716-446655440001"
  ],
  "count": 2
}
```

---

#### 5. Get Request by ID

Retrieve detailed information about a specific request.

**Request:**
```bash
curl -X GET http://localhost:8080/requests/550e8400-e29b-41d4-a716-446655440000
```

**Response:**
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
      "language": "en"
    }
  }
}
```

---

#### 6. List All Requests

List all requests with pagination support.

**Request:**
```bash
# Default pagination (50 items, offset 0)
curl -X GET http://localhost:8080/requests

# Custom pagination
curl -X GET "http://localhost:8080/requests?limit=10&offset=20"
```

**Response:**
```json
{
  "requests": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "created_at": "2025-10-17T12:35:10.123Z",
      "source_type": "text",
      "textanalyzer_uuid": "ghi789-analyzer-uuid",
      "tags": ["analysis", "ai"],
      "metadata": {}
    },
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "created_at": "2025-10-17T12:34:56.789Z",
      "source_type": "url",
      "source_url": "https://example.com/article",
      "scraper_uuid": "abc123-scraper-uuid",
      "textanalyzer_uuid": "def456-analyzer-uuid",
      "tags": ["technology", "programming"],
      "metadata": {}
    }
  ],
  "count": 2,
  "limit": 50,
  "offset": 0
}
```

---

### Error Responses

All endpoints return errors in the following format:

```json
{
  "error": "Error message describing what went wrong"
}
```

Common HTTP status codes:
- `400 Bad Request` - Invalid input data
- `404 Not Found` - Resource not found
- `405 Method Not Allowed` - Wrong HTTP method
- `500 Internal Server Error` - Server-side error

## Database Schema

The service uses SQLite with a migration system for easy schema evolution.

### Tables

#### `requests`
Stores all controller request records.

| Column | Type | Description |
|--------|------|-------------|
| `id` | TEXT | Primary key (UUID) |
| `created_at` | TIMESTAMP | When the request was created |
| `source_type` | TEXT | Either "url" or "text" |
| `source_url` | TEXT | Original URL (if source_type is "url") |
| `scraper_uuid` | TEXT | UUID from scraper service |
| `textanalyzer_uuid` | TEXT | UUID from text analyzer service |
| `tags_json` | TEXT | JSON array of tags |
| `metadata_json` | TEXT | JSON object with additional metadata |

#### `tags`
Individual tags for efficient searching.

| Column | Type | Description |
|--------|------|-------------|
| `id` | INTEGER | Auto-increment primary key |
| `request_id` | TEXT | Foreign key to requests.id |
| `tag` | TEXT | Individual tag value |

### Migrations

The database schema is managed through a migration system located in `internal/storage/migrations.go`. To add a new migration:

1. Add a new `Migration` struct to the `migrations` slice
2. Increment the version number
3. Provide descriptive name and SQL
4. Restart the application (migrations run automatically on startup)

Example:
```go
{
    Version: 3,
    Name:    "add_user_column",
    SQL: `ALTER TABLE requests ADD COLUMN user_id TEXT;`,
}
```

## Testing

### Run Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...

# Run specific package tests
go test ./internal/storage
go test ./internal/handlers
go test ./internal/config
```

### Test Coverage

The test suite covers:
- ✅ Configuration loading and validation
- ✅ Database operations (CRUD)
- ✅ Tag searching (exact and fuzzy)
- ✅ HTTP handlers with mock services
- ✅ Error handling and edge cases
- ✅ Pagination

## Development

### Project Structure

```
controller/
├── cmd/
│   └── controller/
│       └── main.go              # Application entry point
├── internal/
│   ├── clients/
│   │   ├── scraper.go          # Scraper service client
│   │   └── textanalyzer.go     # Text analyzer client
│   ├── config/
│   │   ├── config.go           # Configuration management
│   │   └── config_test.go      # Config tests
│   ├── handlers/
│   │   ├── handlers.go         # HTTP handlers
│   │   └── handlers_test.go    # Handler tests
│   └── storage/
│       ├── migrations.go       # Database migrations
│       ├── storage.go          # Database operations
│       └── storage_test.go     # Storage tests
├── .env.example                # Example environment config
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

### Adding New Features

1. **New Endpoint**: Add handler in `internal/handlers/handlers.go` and register route in `cmd/controller/main.go`
2. **New Client Method**: Extend `internal/clients/*.go` with new API calls
3. **Database Changes**: Add migration in `internal/storage/migrations.go`
4. **Tests**: Add corresponding tests in `*_test.go` files

## Deployment

### Docker Example

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o controller ./cmd/controller

FROM alpine:latest
RUN apk --no-cache add ca-certificates sqlite
WORKDIR /root/
COPY --from=builder /app/controller .
EXPOSE 8080
CMD ["./controller"]
```

Build and run:
```bash
docker build -t controller:latest .
docker run -p 8080:8080 \
  -e SCRAPER_BASE_URL=http://scraper:8081 \
  -e TEXTANALYZER_BASE_URL=http://analyzer:8082 \
  controller:latest
```

### Production Considerations

1. **Database**: For production, migrate to PostgreSQL by:
   - Updating connection string in `storage.go`
   - Adjusting SQL queries (most should work as-is)
   - Testing migrations thoroughly

2. **Logging**: Add structured logging (e.g., `log/slog` from stdlib)

3. **Metrics**: Add Prometheus metrics for monitoring

4. **Service Discovery**: Use environment variables or consul for service URLs

5. **Security**: Add authentication/authorization middleware

## Migration to PostgreSQL

The codebase is designed for easy migration to PostgreSQL:

1. Update `internal/storage/storage.go`:
```go
import _ "github.com/lib/pq"  // Instead of sqlite3

func New(connectionString string) (*Storage, error) {
    db, err := sql.Open("postgres", connectionString)
    // ... rest remains the same
}
```

2. Update connection string format in configuration:
```
DATABASE_URL=postgres://user:pass@localhost:5432/controller?sslmode=disable
```

3. Test migrations - most SQL should be compatible, but check:
   - `AUTOINCREMENT` → `SERIAL` or `IDENTITY`
   - JSON storage (PostgreSQL has native JSON types)
   - Foreign key syntax

## License

MIT

## Support

For issues, questions, or contributions, please open an issue in the repository.
