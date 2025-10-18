# Controller API Reference

REST API documentation for the controller orchestration service.

## Base URL

```
http://localhost:8080
```

## Endpoints

### Health Check

Check if the service is running.

**Request:**
```http
GET /health
```

**Response:**
```json
{
  "status": "healthy"
}
```

---

### Scrape URL and Analyze

Scrape a URL and automatically analyze the extracted text.

**Request:**
```http
POST /scrape
Content-Type: application/json

{
  "url": "https://example.com/article"
}
```

**Parameters:**
- `url` (string, required) - URL to scrape

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
      "title": "Example Article",
      "content": "AI-cleaned content...",
      "images": [],
      "links": []
    },
    "analyzer_metadata": {
      "sentiment": "positive",
      "word_count": 500,
      "readability_level": "standard"
    }
  }
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'
```

---

### Analyze Text Directly

Send text directly for analysis without scraping.

**Request:**
```http
POST /analyze
Content-Type: application/json

{
  "text": "Your text to analyze..."
}
```

**Parameters:**
- `text` (string, required) - Text to analyze

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
      "sentiment": "neutral",
      "word_count": 25,
      "language": "english",
      "confidence": 0.95
    }
  }
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/analyze \
  -H "Content-Type: application/json" \
  -d '{"text": "This is some text to analyze."}'
```

---

### Search by Tags

Search for requests by tags with optional fuzzy matching.

**Request (Exact Match):**
```http
POST /search/tags
Content-Type: application/json

{
  "tags": ["programming", "web"],
  "fuzzy": false
}
```

**Request (Fuzzy Match):**
```http
POST /search/tags
Content-Type: application/json

{
  "tags": ["prog"],
  "fuzzy": true
}
```

**Parameters:**
- `tags` (array of strings, required) - Tags to search for
- `fuzzy` (boolean, optional) - Enable fuzzy matching (default: false)

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

**Example:**
```bash
# Exact match
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{"tags": ["programming", "web"], "fuzzy": false}'

# Fuzzy match
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{"tags": ["prog"], "fuzzy": true}'
```

---

### Get Request by ID

Retrieve detailed information about a specific request.

**Request:**
```http
GET /requests/{id}
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
    "scraper_metadata": { ... },
    "analyzer_metadata": { ... }
  }
}
```

**Error Response (404):**
```json
{
  "error": "request not found"
}
```

**Example:**
```bash
curl http://localhost:8080/requests/550e8400-e29b-41d4-a716-446655440000
```

---

### List All Requests

List all requests with pagination support.

**Request:**
```http
GET /requests?limit=50&offset=0
```

**Query Parameters:**
- `limit` (integer, optional) - Results per page (default: 50, max: 100)
- `offset` (integer, optional) - Number to skip (default: 0)

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

**Example:**
```bash
# Default pagination
curl http://localhost:8080/requests

# Custom pagination
curl "http://localhost:8080/requests?limit=10&offset=20"
```

---

## Data Types

### Request

```go
type Request struct {
    ID                  string    `json:"id"`
    CreatedAt           time.Time `json:"created_at"`
    SourceType          string    `json:"source_type"`      // "url" or "text"
    SourceURL           string    `json:"source_url,omitempty"`
    ScraperUUID         string    `json:"scraper_uuid,omitempty"`
    TextAnalyzerUUID    string    `json:"textanalyzer_uuid"`
    Tags                []string  `json:"tags"`
    Metadata            Metadata  `json:"metadata"`
}
```

### Metadata

```go
type Metadata struct {
    ScraperMetadata     interface{} `json:"scraper_metadata,omitempty"`
    AnalyzerMetadata    interface{} `json:"analyzer_metadata,omitempty"`
}
```

---

## Error Responses

All errors return JSON with an `error` field:

```json
{
  "error": "descriptive error message"
}
```

**HTTP Status Codes:**
- `200 OK` - Success
- `400 Bad Request` - Invalid input data
- `404 Not Found` - Resource not found
- `405 Method Not Allowed` - Wrong HTTP method
- `500 Internal Server Error` - Server-side error

---

## Integration Examples

### JavaScript/TypeScript

```typescript
// Scrape and analyze URL
async function scrapeURL(url: string): Promise<Request> {
  const response = await fetch('http://localhost:8080/scrape', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url })
  });
  return response.json();
}

// Analyze text directly
async function analyzeText(text: string): Promise<Request> {
  const response = await fetch('http://localhost:8080/analyze', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text })
  });
  return response.json();
}

// Search by tags
async function searchByTags(tags: string[], fuzzy = false): Promise<SearchResult> {
  const response = await fetch('http://localhost:8080/search/tags', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tags, fuzzy })
  });
  return response.json();
}

// Get request by ID
async function getRequest(id: string): Promise<Request> {
  const response = await fetch(`http://localhost:8080/requests/${id}`);
  return response.json();
}

// List requests
async function listRequests(limit = 50, offset = 0): Promise<RequestList> {
  const response = await fetch(
    `http://localhost:8080/requests?limit=${limit}&offset=${offset}`
  );
  return response.json();
}
```

### Python

```python
import requests

# Scrape and analyze URL
def scrape_url(url: str) -> dict:
    response = requests.post(
        'http://localhost:8080/scrape',
        json={'url': url}
    )
    return response.json()

# Analyze text directly
def analyze_text(text: str) -> dict:
    response = requests.post(
        'http://localhost:8080/analyze',
        json={'text': text}
    )
    return response.json()

# Search by tags
def search_by_tags(tags: list[str], fuzzy: bool = False) -> dict:
    response = requests.post(
        'http://localhost:8080/search/tags',
        json={'tags': tags, 'fuzzy': fuzzy}
    )
    return response.json()

# Get request by ID
def get_request(id: str) -> dict:
    response = requests.get(f'http://localhost:8080/requests/{id}')
    return response.json()

# List requests
def list_requests(limit: int = 50, offset: int = 0) -> dict:
    response = requests.get(
        'http://localhost:8080/requests',
        params={'limit': limit, 'offset': offset}
    )
    return response.json()
```

### cURL

```bash
# Health check
curl http://localhost:8080/health

# Scrape and analyze URL
curl -X POST http://localhost:8080/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'

# Analyze text directly
curl -X POST http://localhost:8080/analyze \
  -H "Content-Type: application/json" \
  -d '{"text": "Your text to analyze..."}'

# Search by tags (exact match)
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{"tags": ["programming", "web"], "fuzzy": false}'

# Search by tags (fuzzy match)
curl -X POST http://localhost:8080/search/tags \
  -H "Content-Type: application/json" \
  -d '{"tags": ["prog"], "fuzzy": true}'

# Get request by ID
curl http://localhost:8080/requests/550e8400-e29b-41d4-a716-446655440000

# List all requests
curl "http://localhost:8080/requests?limit=10&offset=0"
```

---

## Configuration

### Environment Variables

```bash
export SCRAPER_BASE_URL=http://localhost:8081
export TEXTANALYZER_BASE_URL=http://localhost:8082
export CONTROLLER_PORT=8080
export DATABASE_PATH=./controller.db
```

- `SCRAPER_BASE_URL` - Scraper service URL (default: http://localhost:8081)
- `TEXTANALYZER_BASE_URL` - TextAnalyzer service URL (default: http://localhost:8082)
- `CONTROLLER_PORT` - HTTP server port (default: 8080)
- `DATABASE_PATH` - SQLite database path (default: ./controller.db)

---

## Database Schema

### requests Table

```sql
CREATE TABLE requests (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    source_type TEXT NOT NULL,
    source_url TEXT,
    scraper_uuid TEXT,
    textanalyzer_uuid TEXT NOT NULL,
    tags_json TEXT NOT NULL,
    metadata_json TEXT NOT NULL
);
```

### tags Table

```sql
CREATE TABLE tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    request_id TEXT NOT NULL,
    tag TEXT NOT NULL,
    FOREIGN KEY (request_id) REFERENCES requests(id) ON DELETE CASCADE
);
```

### Indexes

- Index on `tags.tag` for efficient tag search
- Index on `requests.created_at` for time-based queries

---

## Performance

### Timeouts

- HTTP client requests to services have configured timeouts
- Database operations use connection pooling
- Tag search uses indexed lookups

### Fuzzy Search

Fuzzy tag matching uses SQL LIKE queries with wildcards:
- Tags are matched using `tag LIKE '%search%'` pattern
- Results are deduplicated across multiple tag matches
