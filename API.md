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

Scrape a URL and automatically analyze the extracted text. **All URLs are automatically scored for quality before processing.** If the score is below the configured threshold, only scoring metadata is returned (no scraping or analysis is performed).

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

**Response (High-Quality URL - Score ≥ Threshold):**
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
    },
    "link_score": {
      "score": 0.85,
      "reason": "High quality technical article",
      "categories": ["technical", "education"],
      "is_recommended": true,
      "malicious_indicators": []
    }
  }
}
```

**Response (Low-Quality URL - Score < Threshold):**
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "created_at": "2025-10-17T12:35:10.123Z",
  "source_type": "url",
  "source_url": "https://facebook.com",
  "tags": ["social_media"],
  "metadata": {
    "link_score": {
      "score": 0.2,
      "reason": "Social media platform - not suitable for ingestion",
      "categories": ["social_media"],
      "is_recommended": false,
      "malicious_indicators": []
    },
    "below_threshold": true,
    "threshold": 0.5
  }
}
```

**Note:** When a URL scores below the configured threshold (default: 0.5), the controller skips the expensive scraping and analysis operations and returns only the scoring metadata. This protects the database from irrelevant content while providing transparency about why the URL was rejected.

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

### Score Link

Score a URL to determine if it should be ingested. This endpoint evaluates content quality and identifies potentially inappropriate, malicious, or low-value content without performing a full scrape.

**Request:**
```http
POST /api/score
Content-Type: application/json

{
  "url": "https://example.com/article"
}
```

**Parameters:**
- `url` (string, required) - URL to score

**Response:**
```json
{
  "url": "https://example.com/article",
  "score": {
    "score": 0.85,
    "reason": "High quality technical article with educational content",
    "categories": ["technical", "education"],
    "is_recommended": true,
    "malicious_indicators": []
  },
  "meets_threshold": true,
  "threshold": 0.5
}
```

**Score Field Description:**
- `score` (float) - Quality score from 0.0 to 1.0
- `reason` (string) - Explanation for the assigned score
- `categories` (array) - Detected content categories
- `is_recommended` (boolean) - Whether the link meets the scraper's quality threshold
- `malicious_indicators` (array) - Any detected suspicious patterns
- `meets_threshold` (boolean) - Whether the score meets the controller's configured threshold
- `threshold` (float) - The controller's configured minimum score for ingestion

**Rejected Content Types:**
- Social media platforms (Facebook, Twitter, Instagram, Reddit, etc.)
- Gambling websites
- Adult content / pornography
- Drug marketplaces
- Forums and chatrooms (except high-quality technical forums)
- General marketplaces (eBay, Amazon, etc.)
- Spam and clickbait
- Malicious websites (phishing, malware, scams)

**Accepted Content Types:**
- News articles and journalism
- Educational content and tutorials
- Research papers and academic content
- Technical documentation
- Blog posts with substantive content
- Government and official resources

**Low Score Example:**
```json
{
  "url": "https://facebook.com",
  "score": {
    "score": 0.2,
    "reason": "Social media platform - not suitable for content ingestion",
    "categories": ["social_media"],
    "is_recommended": false,
    "malicious_indicators": []
  },
  "meets_threshold": false,
  "threshold": 0.5
}
```

**Malicious Content Example:**
```json
{
  "url": "https://suspicious-site.com",
  "score": {
    "score": 0.05,
    "reason": "Suspected phishing site with misleading content",
    "categories": ["malicious", "spam"],
    "is_recommended": false,
    "malicious_indicators": ["phishing", "suspicious_url"]
  },
  "meets_threshold": false,
  "threshold": 0.5
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/score \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'
```

**Use Case:** Use this endpoint to pre-screen URLs before submitting them for full scraping. This allows you to filter out low-quality or inappropriate content efficiently.

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

### Filter Requests

Filter requests by multiple criteria including tags, date range, and source type.

**Request:**
```http
POST /api/requests/filter
Content-Type: application/json

{
  "tags": ["programming", "web"],
  "fuzzy": true,
  "date_start": "2024-01-01T00:00:00Z",
  "date_end": "2024-01-31T23:59:59Z",
  "source_type": "url",
  "limit": 100,
  "offset": 0
}
```

**Parameters:**
- `tags` (array of strings, optional) - Tags to filter by
- `fuzzy` (boolean, optional) - Enable fuzzy tag matching (default: false)
- `date_start` (string, optional) - Start date in RFC3339 format
- `date_end` (string, optional) - End date in RFC3339 format
- `source_type` (string, optional) - Filter by source type ("url" or "text")
- `limit` (integer, optional) - Maximum number of results (default: 100)
- `offset` (integer, optional) - Number of results to skip for pagination

**Response:**
```json
{
  "requests": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "created_at": "2024-01-15T14:30:00Z",
      "source_type": "url",
      "source_url": "https://example.com/article",
      "scraper_uuid": "660e8400-e29b-41d4-a716-446655440001",
      "textanalyzer_uuid": "770e8400-e29b-41d4-a716-446655440002",
      "tags": ["programming", "web", "tutorial"],
      "metadata": {
        "scraper_metadata": {
          "title": "Web Programming Tutorial"
        }
      }
    }
  ],
  "count": 1,
  "limit": 100,
  "offset": 0
}
```

**Example:**
```bash
# Filter by date range
curl -X POST http://localhost:8080/api/requests/filter \
  -H "Content-Type: application/json" \
  -d '{
    "date_start": "2024-01-01T00:00:00Z",
    "date_end": "2024-01-31T23:59:59Z",
    "limit": 50
  }'

# Filter by tags and date range
curl -X POST http://localhost:8080/api/requests/filter \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["programming"],
    "fuzzy": true,
    "date_start": "2024-01-01T00:00:00Z",
    "date_end": "2024-01-31T23:59:59Z"
  }'

# Filter by source type
curl -X POST http://localhost:8080/api/requests/filter \
  -H "Content-Type: application/json" \
  -d '{
    "source_type": "url",
    "limit": 100
  }'
```

---

### Search Images by Tags

Search for images across all scraped content using fuzzy tag matching. This endpoint queries the scraper service for images with matching tags.

**Request:**
```http
POST /api/images/search
Content-Type: application/json

{
  "tags": ["cat", "animal"]
}
```

**Parameters:**
- `tags` (array of strings, required) - Tags to search for (fuzzy matching)

**Response:**
```json
{
  "images": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "url": "https://example.com/cat.jpg",
      "alt_text": "A cat photo",
      "summary": "Image shows a domestic cat...",
      "tags": ["cat", "animal", "pet"],
      "base64_data": "iVBORw0KGgoAAAANSUhEUgAAAAEA..."
    },
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "url": "https://example.com/wildlife.jpg",
      "alt_text": "Wildlife scene",
      "summary": "Image depicts various animals in nature...",
      "tags": ["animals", "wildlife", "nature"],
      "base64_data": "iVBORw0KGgoAAAANSUhEUgAAAAEA..."
    }
  ],
  "count": 2
}
```

**Fuzzy Matching:** Searches are case-insensitive and match substrings. For example:
- Searching for "cat" will match images with tags: "cat", "cats", "wildcat", "scatter"
- Searching for "anim" will match images with tags: "animal", "animation", "animals"

**Error Response (400):**
```json
{
  "error": "At least one tag is required"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/images/search \
  -H "Content-Type: application/json" \
  -d '{"tags": ["cat", "dog"]}'
```

**Note:** This endpoint is a pass-through to the scraper service's image search functionality. All images stored in the scraper's database can be searched.

---

### Get Document Images

Retrieve all images associated with a specific document using its scraper UUID.

**Request:**
```http
GET /api/documents/{scraper_uuid}/images
```

**Parameters:**
- `scraper_uuid` (string, required) - Scraper UUID from document metadata

**Response:**
```json
{
  "images": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "url": "https://example.com/article-image.jpg",
      "alt_text": "Article header image",
      "summary": "Image shows a diagram explaining the concept...",
      "tags": ["diagram", "technical", "illustration"],
      "base64_data": "iVBORw0KGgoAAAANSUhEUgAAAAEA..."
    },
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "url": "https://example.com/chart.png",
      "alt_text": "Performance chart",
      "summary": "Graph depicting performance metrics...",
      "tags": ["chart", "data", "visualization"],
      "base64_data": "iVBORw0KGgoAAAANSUhEUgAAAAEA..."
    }
  ],
  "count": 2
}
```

**Empty Response:**
```json
{
  "images": [],
  "count": 0
}
```

**Error Response (400):**
```json
{
  "error": "Scraper UUID is required"
}
```

**Example:**
```bash
curl http://localhost:8080/api/documents/abc123-scraper-uuid/images
```

**Use Case:** Retrieve images that were scraped alongside a document. Use the `scraper_uuid` field from a document's metadata to fetch its associated images.

---

### Extract Links

Extract and filter links from a URL using AI-powered content analysis. This endpoint identifies substantive links (articles, blog posts, research papers) while filtering out navigation, social media buttons, ads, and spam.

**Request:**
```http
POST /api/extract-links
Content-Type: application/json

{
  "url": "https://example.com"
}
```

**Parameters:**
- `url` (string, required) - URL to extract links from

**Response:**
```json
{
  "url": "https://example.com",
  "links": [
    "https://example.com/article-1",
    "https://example.com/article-2",
    "https://example.com/blog/important-post"
  ],
  "count": 3
}
```

**Response Fields:**
- `url` (string) - The URL that was processed
- `links` (array) - Array of filtered, substantive links found on the page
- `count` (integer) - Number of links returned

**AI Filtering:** The endpoint uses Ollama to intelligently filter links, including only:
- Articles and blog posts
- Research papers and publications
- Documentation and guides
- News content
- Educational resources

**Excluded Link Types:**
- Navigation menus and breadcrumbs
- Social media share buttons
- Footer links
- Advertisements
- "Load more" pagination
- Cookie consent and privacy policy links

**Error Response (400):**
```json
{
  "error": "URL is required"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/extract-links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

**Use Case:** Use this endpoint to discover content from a landing page or index page. The AI filtering ensures you only get meaningful content links for further processing.

---

### Create Async Scrape Request

Create an asynchronous scrape request that processes in the background. Returns immediately with a request ID for tracking progress.

**Request:**
```http
POST /api/scrape-requests
Content-Type: application/json

{
  "url": "https://example.com/article"
}
```

**Parameters:**
- `url` (string, required) - URL to scrape asynchronously

**Response:**
```json
{
  "id": "7a8e9f0a-1234-5678-90ab-cdef12345678",
  "url": "https://example.com/article",
  "status": "pending",
  "progress": 0,
  "created_at": "2025-10-19T12:34:56.789Z",
  "updated_at": "2025-10-19T12:34:56.789Z",
  "expires_at": "2025-10-19T12:49:56.789Z"
}
```

**Status Values:**
- `pending` - Request queued, not yet started
- `processing` - Currently being processed
- `completed` - Successfully completed, result available
- `failed` - Processing failed, error message available

**Progress Tracking:**
- `0` - Pending
- `10` - Started processing
- `30` - URL scoring complete
- `50` - Scraping complete
- `70` - Text analysis complete
- `90` - Saving to database
- `100` - Completed

**Notes:**
- Returns existing request if URL is already being processed
- Requests automatically expire and are removed after 15 minutes
- Background processing includes scoring, scraping, and analysis
- URLs below quality threshold will fail with error message

**Example:**
```bash
curl -X POST http://localhost:8080/api/scrape-requests \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/article"}'
```

---

### List Scrape Requests

Get all active scrape requests sorted by creation time (newest first).

**Request:**
```http
GET /api/scrape-requests
```

**Response:**
```json
{
  "count": 2,
  "requests": [
    {
      "id": "7a8e9f0a-1234-5678-90ab-cdef12345678",
      "url": "https://example.com/article-1",
      "status": "completed",
      "progress": 100,
      "created_at": "2025-10-19T12:34:56.789Z",
      "updated_at": "2025-10-19T12:35:12.456Z",
      "result_request_id": "550e8400-e29b-41d4-a716-446655440000",
      "expires_at": "2025-10-19T12:49:56.789Z"
    },
    {
      "id": "8b9f0a1b-2345-6789-01bc-def123456789",
      "url": "https://example.com/article-2",
      "status": "processing",
      "progress": 50,
      "created_at": "2025-10-19T12:35:00.123Z",
      "updated_at": "2025-10-19T12:35:08.789Z",
      "expires_at": "2025-10-19T12:50:00.123Z"
    }
  ]
}
```

**Example:**
```bash
curl http://localhost:8080/api/scrape-requests
```

---

### Get Scrape Request

Get detailed status of a specific scrape request.

**Request:**
```http
GET /api/scrape-requests/{id}
```

**Parameters:**
- `id` (string, required) - Scrape request UUID

**Response (Processing):**
```json
{
  "id": "7a8e9f0a-1234-5678-90ab-cdef12345678",
  "url": "https://example.com/article",
  "status": "processing",
  "progress": 70,
  "created_at": "2025-10-19T12:34:56.789Z",
  "updated_at": "2025-10-19T12:35:10.123Z",
  "expires_at": "2025-10-19T12:49:56.789Z"
}
```

**Response (Completed):**
```json
{
  "id": "7a8e9f0a-1234-5678-90ab-cdef12345678",
  "url": "https://example.com/article",
  "status": "completed",
  "progress": 100,
  "created_at": "2025-10-19T12:34:56.789Z",
  "updated_at": "2025-10-19T12:35:12.456Z",
  "result_request_id": "550e8400-e29b-41d4-a716-446655440000",
  "expires_at": "2025-10-19T12:49:56.789Z"
}
```

**Response (Failed):**
```json
{
  "id": "7a8e9f0a-1234-5678-90ab-cdef12345678",
  "url": "https://invalid-url.com",
  "status": "failed",
  "progress": 30,
  "created_at": "2025-10-19T12:34:56.789Z",
  "updated_at": "2025-10-19T12:35:02.123Z",
  "error_message": "Failed to score link: DNS lookup failed",
  "expires_at": "2025-10-19T12:49:56.789Z"
}
```

**Error Response:**
```json
{
  "error": "Scrape request not found"
}
```

**Example:**
```bash
curl http://localhost:8080/api/scrape-requests/7a8e9f0a-1234-5678-90ab-cdef12345678
```

---

### Retry Scrape Request

Retry a failed scrape request. Resets status to pending and starts processing again.

**Request:**
```http
POST /api/scrape-requests/{id}/retry
```

**Parameters:**
- `id` (string, required) - Scrape request UUID

**Response:**
```json
{
  "id": "7a8e9f0a-1234-5678-90ab-cdef12345678",
  "url": "https://example.com/article",
  "status": "pending",
  "progress": 0,
  "created_at": "2025-10-19T12:34:56.789Z",
  "updated_at": "2025-10-19T12:36:00.000Z",
  "expires_at": "2025-10-19T12:49:56.789Z"
}
```

**Error Response (Not Found):**
```json
{
  "error": "Scrape request not found"
}
```

**Error Response (Invalid State):**
```json
{
  "error": "Can only retry failed requests"
}
```

**Notes:**
- Only failed requests can be retried
- Completed, pending, or processing requests will return an error
- Original creation and expiration times are preserved

**Example:**
```bash
curl -X POST http://localhost:8080/api/scrape-requests/7a8e9f0a-1234-5678-90ab-cdef12345678/retry
```

---

### Delete Scrape Request

Delete a scrape request from tracking. Does not delete the stored result if already completed.

**Request:**
```http
DELETE /api/scrape-requests/{id}
```

**Parameters:**
- `id` (string, required) - Scrape request UUID

**Response:**
```json
{
  "status": "deleted"
}
```

**Error Response:**
```json
{
  "error": "Scrape request not found"
}
```

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/scrape-requests/7a8e9f0a-1234-5678-90ab-cdef12345678
```

**Use Case:** Clean up completed or failed requests from the tracking list. Useful for UI implementations to remove requests after user has viewed the result.

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

### Delete Request

Permanently delete a request and all associated data from the controller, scraper, and textanalyzer services.

**Request:**
```http
DELETE /api/requests/{id}
```

**Parameters:**
- `id` (string, required) - Request UUID

**Response:**
```json
{
  "message": "Request deleted successfully"
}
```

**Error Response (404):**
```json
{
  "error": "Request not found"
}
```

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/requests/550e8400-e29b-41d4-a716-446655440000
```

**Notes:**
- Deletes the request from the controller database
- Deletes associated scraper data if `scraper_uuid` exists
- Deletes associated textanalyzer data
- This operation is permanent and cannot be undone
- Failures in upstream service deletions are logged but don't stop the local deletion

---

### Tombstone Request

Mark a request as scheduled for deletion by adding `tombstone_datetime` to its metadata. This is a soft delete that can be undone.

**Request:**
```http
PUT /api/requests/{id}/tombstone
```

**Parameters:**
- `id` (string, required) - Request UUID

**Response:**
```json
{
  "message": "Request tombstoned successfully",
  "tombstone_datetime": "2025-10-19T12:34:56.789Z"
}
```

**Error Response (404):**
```json
{
  "error": "Request not found"
}
```

**Example:**
```bash
curl -X PUT http://localhost:8080/api/requests/550e8400-e29b-41d4-a716-446655440000/tombstone
```

**Notes:**
- Adds `tombstone_datetime` field to request metadata
- Request remains in database and can be retrieved
- Can be undone using the untombstone endpoint
- UI applications should display tombstoned items with visual indicators

---

### Untombstone Request

Remove tombstone status from a request by deleting the `tombstone_datetime` field from its metadata.

**Request:**
```http
DELETE /api/requests/{id}/tombstone
```

**Parameters:**
- `id` (string, required) - Request UUID

**Response:**
```json
{
  "message": "Request untombstoned successfully"
}
```

**Error Response (404):**
```json
{
  "error": "Request not found"
}
```

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/requests/550e8400-e29b-41d4-a716-446655440000/tombstone
```

**Use Case:** Restore a request that was marked for deletion. The request returns to normal status.

---

### Delete Image

Permanently delete an image from the scraper service.

**Request:**
```http
DELETE /api/images/{id}
```

**Parameters:**
- `id` (string, required) - Image UUID

**Response:**
```json
{
  "message": "Image deleted successfully"
}
```

**Error Response (404):**
```json
{
  "error": "Image not found"
}
```

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/images/550e8400-e29b-41d4-a716-446655440000
```

**Notes:**
- Deletes the image from the scraper database
- This operation is permanent and cannot be undone
- Image must exist in the scraper service

---

### Tombstone Image

Mark an image as scheduled for deletion by adding `tombstone_datetime` to its record. This is a soft delete that can be undone.

**Request:**
```http
PUT /api/images/{id}/tombstone
```

**Parameters:**
- `id` (string, required) - Image UUID

**Response:**
```json
{
  "message": "Image tombstoned successfully"
}
```

**Error Response (404):**
```json
{
  "error": "Image not found"
}
```

**Example:**
```bash
curl -X PUT http://localhost:8080/api/images/550e8400-e29b-41d4-a716-446655440000/tombstone
```

**Notes:**
- Adds `tombstone_datetime` field to image record in scraper database
- Image remains in database and can be retrieved
- Can be undone using the untombstone endpoint
- Image will include `tombstone_datetime` in API responses

---

### Untombstone Image

Remove tombstone status from an image by deleting the `tombstone_datetime` field.

**Request:**
```http
DELETE /api/images/{id}/tombstone
```

**Parameters:**
- `id` (string, required) - Image UUID

**Response:**
```json
{
  "message": "Image untombstoned successfully"
}
```

**Error Response (404):**
```json
{
  "error": "Image not found"
}
```

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/images/550e8400-e29b-41d4-a716-446655440000/tombstone
```

**Use Case:** Restore an image that was marked for deletion. The image returns to normal status.

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

// Extract links from URL
async function extractLinks(url: string): Promise<ExtractLinksResponse> {
  const response = await fetch('http://localhost:8080/api/extract-links', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ url })
  });
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

# Extract links from URL
def extract_links(url: str) -> dict:
    response = requests.post(
        'http://localhost:8080/api/extract-links',
        json={'url': url}
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

# Extract links from URL
curl -X POST http://localhost:8080/api/extract-links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'
```

---

## Configuration

### Environment Variables

```bash
export SCRAPER_BASE_URL=http://localhost:8081
export TEXTANALYZER_BASE_URL=http://localhost:8082
export CONTROLLER_PORT=8080
export DATABASE_PATH=./controller.db
export LINK_SCORE_THRESHOLD=0.5
```

- `SCRAPER_BASE_URL` - Scraper service URL (default: http://localhost:8081)
- `TEXTANALYZER_BASE_URL` - TextAnalyzer service URL (default: http://localhost:8082)
- `CONTROLLER_PORT` - HTTP server port (default: 8080)
- `DATABASE_PATH` - SQLite database path (default: ./controller.db)
- `LINK_SCORE_THRESHOLD` - Minimum quality score (0.0-1.0) for URL ingestion (default: 0.5)
  - URLs scoring below this threshold will not be scraped or analyzed
  - Only scoring metadata will be stored and returned
  - Higher values (e.g., 0.7) increase quality but may filter more content
  - Lower values (e.g., 0.3) allow more content but with lower quality standards

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
