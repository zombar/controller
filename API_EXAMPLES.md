# API Examples

This document provides comprehensive curl command examples for all controller service endpoints.

## Setup

Set your base URL (adjust port if needed):

```bash
export CONTROLLER_URL="http://localhost:8080"
```

## 1. Health Check

Simple health check to verify service is running.

```bash
curl -X GET $CONTROLLER_URL/health
```

**Response (200 OK):**
```json
{
  "status": "healthy"
}
```

---

## 2. Scrape and Analyze URL

Submit a URL for scraping and automatic text analysis.

### Basic Example

```bash
curl -X POST $CONTROLLER_URL/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com"
  }'
```

### With Pretty Printing (requires jq)

```bash
curl -s -X POST $CONTROLLER_URL/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://news.ycombinator.com"
  }' | jq
```

### Save Response to Variable

```bash
RESPONSE=$(curl -s -X POST $CONTROLLER_URL/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://github.com"
  }')

# Extract the controller ID
CONTROLLER_ID=$(echo $RESPONSE | jq -r '.id')
echo "Controller ID: $CONTROLLER_ID"

# Extract tags
TAGS=$(echo $RESPONSE | jq -r '.tags[]')
echo "Tags: $TAGS"
```

**Response (201 Created):**
```json
{
  "id": "c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d",
  "created_at": "2025-10-17T15:30:45.123Z",
  "source_type": "url",
  "source_url": "https://example.com",
  "scraper_uuid": "scraper-abc123",
  "textanalyzer_uuid": "analyzer-def456",
  "tags": ["example", "domain", "website"],
  "metadata": {
    "scraper_metadata": {
      "title": "Example Domain",
      "status_code": 200
    },
    "analyzer_metadata": {
      "language": "en",
      "word_count": 150
    }
  }
}
```

**Error Response (400 Bad Request):**
```json
{
  "error": "URL is required"
}
```

**Error Response (500 Internal Server Error):**
```json
{
  "error": "Failed to scrape URL: connection timeout"
}
```

---

## 3. Analyze Text Directly

Submit text directly for analysis without scraping.

### Basic Example

```bash
curl -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Machine learning is revolutionizing the way we build software applications."
  }'
```

### Multi-line Text

```bash
curl -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Artificial intelligence and machine learning are transforming software development.\n\nNew tools powered by AI are helping developers write code faster and with fewer bugs.\n\nThe future of programming is collaborative, with AI assistants working alongside human developers."
  }'
```

### Text from File

```bash
curl -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d "{\"text\": \"$(cat article.txt)\"}"
```

### With Error Handling

```bash
RESPONSE=$(curl -s -w "\n%{http_code}" -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Kubernetes orchestrates containerized applications across clusters."
  }')

HTTP_CODE=$(echo "$RESPONSE" | tail -n1)
BODY=$(echo "$RESPONSE" | sed '$d')

if [ $HTTP_CODE -eq 201 ]; then
  echo "Success!"
  echo $BODY | jq
else
  echo "Error (HTTP $HTTP_CODE):"
  echo $BODY | jq
fi
```

**Response (201 Created):**
```json
{
  "id": "f9e8d7c6-b5a4-4c3b-9a8b-7c6d5e4f3g2h",
  "created_at": "2025-10-17T15:35:12.456Z",
  "source_type": "text",
  "textanalyzer_uuid": "analyzer-ghi789",
  "tags": ["machine-learning", "software", "development"],
  "metadata": {
    "analyzer_metadata": {
      "language": "en",
      "sentiment": "positive",
      "word_count": 12
    }
  }
}
```

---

## 4. Search by Tags

Search for requests by tags with exact or fuzzy matching.

### Exact Match (Single Tag)

```bash
curl -X POST $CONTROLLER_URL/search/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["programming"],
    "fuzzy": false
  }'
```

### Exact Match (Multiple Tags - OR Logic)

```bash
curl -X POST $CONTROLLER_URL/search/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["programming", "software", "development"],
    "fuzzy": false
  }' | jq
```

### Fuzzy Match

```bash
curl -X POST $CONTROLLER_URL/search/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["mach"],
    "fuzzy": true
  }' | jq
```

This will match tags like "machine", "machine-learning", etc.

### Get Full Details for Search Results

```bash
# First, search by tag
REQUEST_IDS=$(curl -s -X POST $CONTROLLER_URL/search/tags \
  -H "Content-Type: application/json" \
  -d '{
    "tags": ["web"],
    "fuzzy": true
  }' | jq -r '.request_ids[]')

# Then, fetch details for each ID
for id in $REQUEST_IDS; do
  echo "Details for $id:"
  curl -s $CONTROLLER_URL/requests/$id | jq
  echo ""
done
```

**Response (200 OK):**
```json
{
  "request_ids": [
    "c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d",
    "f9e8d7c6-b5a4-4c3b-9a8b-7c6d5e4f3g2h"
  ],
  "count": 2
}
```

**Empty Results:**
```json
{
  "request_ids": [],
  "count": 0
}
```

---

## 5. Get Request by ID

Retrieve detailed information about a specific request.

### Basic Example

```bash
curl -X GET $CONTROLLER_URL/requests/c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d
```

### With Pretty Printing

```bash
curl -s $CONTROLLER_URL/requests/c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d | jq
```

### Extract Specific Fields

```bash
# Get only tags
curl -s $CONTROLLER_URL/requests/c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d | jq '.tags'

# Get creation timestamp
curl -s $CONTROLLER_URL/requests/c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d | jq '.created_at'

# Get source URL (if exists)
curl -s $CONTROLLER_URL/requests/c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d | jq '.source_url'
```

**Response (200 OK):**
```json
{
  "id": "c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d",
  "created_at": "2025-10-17T15:30:45.123Z",
  "source_type": "url",
  "source_url": "https://example.com",
  "scraper_uuid": "scraper-abc123",
  "textanalyzer_uuid": "analyzer-def456",
  "tags": ["example", "domain", "website"],
  "metadata": {
    "scraper_metadata": {
      "title": "Example Domain"
    },
    "analyzer_metadata": {
      "language": "en"
    }
  }
}
```

**Error Response (404 Not Found):**
```json
{
  "error": "Request not found"
}
```

---

## 6. List All Requests

List all requests with pagination support.

### Default Pagination

```bash
curl -X GET $CONTROLLER_URL/requests | jq
```

Returns up to 50 requests, offset 0.

### Custom Pagination

```bash
# Get first 10 requests
curl -X GET "$CONTROLLER_URL/requests?limit=10&offset=0" | jq

# Get next 10 requests
curl -X GET "$CONTROLLER_URL/requests?limit=10&offset=10" | jq

# Get 5 requests starting from position 20
curl -X GET "$CONTROLLER_URL/requests?limit=5&offset=20" | jq
```

### List Only IDs and Tags

```bash
curl -s $CONTROLLER_URL/requests | jq '.requests[] | {id: .id, tags: .tags}'
```

### List with Timestamps

```bash
curl -s $CONTROLLER_URL/requests | jq '.requests[] | {id: .id, created_at: .created_at, source_type: .source_type}'
```

### Filter by Source Type (client-side)

```bash
# Get only URL-based requests
curl -s $CONTROLLER_URL/requests | jq '.requests[] | select(.source_type == "url")'

# Get only text-based requests
curl -s $CONTROLLER_URL/requests | jq '.requests[] | select(.source_type == "text")'
```

### Count Total Requests

```bash
curl -s "$CONTROLLER_URL/requests?limit=1000&offset=0" | jq '.count'
```

**Response (200 OK):**
```json
{
  "requests": [
    {
      "id": "f9e8d7c6-b5a4-4c3b-9a8b-7c6d5e4f3g2h",
      "created_at": "2025-10-17T15:35:12.456Z",
      "source_type": "text",
      "textanalyzer_uuid": "analyzer-ghi789",
      "tags": ["machine-learning", "software"],
      "metadata": {}
    },
    {
      "id": "c7b3d8e0-5e0b-4b0f-8b3a-3b9f4b3d3b3d",
      "created_at": "2025-10-17T15:30:45.123Z",
      "source_type": "url",
      "source_url": "https://example.com",
      "scraper_uuid": "scraper-abc123",
      "textanalyzer_uuid": "analyzer-def456",
      "tags": ["example", "domain"],
      "metadata": {}
    }
  ],
  "count": 2,
  "limit": 50,
  "offset": 0
}
```

---

## Advanced Examples

### Complete Workflow Script

```bash
#!/bin/bash

CONTROLLER_URL="http://localhost:8080"

echo "=== Controller Service Demo ==="
echo ""

# 1. Health check
echo "1. Checking service health..."
curl -s $CONTROLLER_URL/health | jq
echo ""

# 2. Scrape a URL
echo "2. Scraping URL..."
SCRAPE_RESULT=$(curl -s -X POST $CONTROLLER_URL/scrape \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com"
  }')

SCRAPE_ID=$(echo $SCRAPE_RESULT | jq -r '.id')
SCRAPE_TAGS=$(echo $SCRAPE_RESULT | jq -r '.tags[]')

echo "Created request: $SCRAPE_ID"
echo "Tags: $SCRAPE_TAGS"
echo ""

# 3. Analyze text directly
echo "3. Analyzing text..."
ANALYZE_RESULT=$(curl -s -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Go is a statically typed, compiled programming language designed at Google."
  }')

ANALYZE_ID=$(echo $ANALYZE_RESULT | jq -r '.id')
echo "Created request: $ANALYZE_ID"
echo ""

# 4. Search by tag
echo "4. Searching by tag..."
FIRST_TAG=$(echo $SCRAPE_TAGS | head -n1)
SEARCH_RESULT=$(curl -s -X POST $CONTROLLER_URL/search/tags \
  -H "Content-Type: application/json" \
  -d "{
    \"tags\": [\"$FIRST_TAG\"],
    \"fuzzy\": false
  }")

echo "Search results for tag '$FIRST_TAG':"
echo $SEARCH_RESULT | jq
echo ""

# 5. Get request details
echo "5. Getting request details..."
curl -s $CONTROLLER_URL/requests/$SCRAPE_ID | jq
echo ""

# 6. List all requests
echo "6. Listing all requests..."
curl -s "$CONTROLLER_URL/requests?limit=5" | jq '.requests[] | {id: .id, source_type: .source_type}'
```

### Bulk Operations

```bash
# Analyze multiple texts in sequence
TEXTS=(
  "Python is a high-level programming language"
  "Docker containers simplify application deployment"
  "React is a JavaScript library for building user interfaces"
)

for text in "${TEXTS[@]}"; do
  echo "Analyzing: $text"
  curl -s -X POST $CONTROLLER_URL/analyze \
    -H "Content-Type: application/json" \
    -d "{\"text\": \"$text\"}" | jq -r '.id'
done
```

### Export to CSV

```bash
# Export all requests to CSV
echo "id,created_at,source_type,tags" > requests.csv

curl -s "$CONTROLLER_URL/requests?limit=1000" | \
  jq -r '.requests[] | [.id, .created_at, .source_type, (.tags | join(";"))] | @csv' \
  >> requests.csv

echo "Exported to requests.csv"
```

### Monitor Recent Activity

```bash
# Watch for new requests (runs every 5 seconds)
watch -n 5 'curl -s "http://localhost:8080/requests?limit=5" | jq ".requests[] | {id: .id, created_at: .created_at}"'
```

---

## Error Handling Examples

### Check HTTP Status Codes

```bash
# Capture both body and status code
HTTP_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d '{"text": "test"}')

HTTP_BODY=$(echo "$HTTP_RESPONSE" | sed '$d')
HTTP_CODE=$(echo "$HTTP_RESPONSE" | tail -n1)

if [ $HTTP_CODE -eq 201 ]; then
  echo "Success: $HTTP_BODY"
elif [ $HTTP_CODE -eq 400 ]; then
  echo "Bad Request: $HTTP_BODY"
elif [ $HTTP_CODE -eq 404 ]; then
  echo "Not Found: $HTTP_BODY"
elif [ $HTTP_CODE -eq 500 ]; then
  echo "Server Error: $HTTP_BODY"
fi
```

### Retry Logic

```bash
MAX_RETRIES=3
RETRY_COUNT=0

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
  RESPONSE=$(curl -s -w "%{http_code}" -X POST $CONTROLLER_URL/scrape \
    -H "Content-Type: application/json" \
    -d '{"url": "https://example.com"}')

  HTTP_CODE="${RESPONSE: -3}"

  if [ $HTTP_CODE -eq 201 ]; then
    echo "Success!"
    break
  fi

  RETRY_COUNT=$((RETRY_COUNT + 1))
  echo "Attempt $RETRY_COUNT failed, retrying..."
  sleep 2
done
```

---

## Testing with Different Data

### Test Invalid Inputs

```bash
# Empty URL
curl -X POST $CONTROLLER_URL/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": ""}'

# Empty text
curl -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d '{"text": ""}'

# Empty tags array
curl -X POST $CONTROLLER_URL/search/tags \
  -H "Content-Type: application/json" \
  -d '{"tags": [], "fuzzy": false}'

# Non-existent ID
curl $CONTROLLER_URL/requests/non-existent-id
```

### Test Edge Cases

```bash
# Very long text
LONG_TEXT=$(python3 -c "print('word ' * 10000)")
curl -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d "{\"text\": \"$LONG_TEXT\"}"

# Special characters in text
curl -X POST $CONTROLLER_URL/analyze \
  -H "Content-Type: application/json" \
  -d '{
    "text": "Test with \"quotes\", new\nlines, and special chars: <>&"
  }'
```

---

## Performance Testing

### Simple Load Test

```bash
# Send 100 requests
for i in {1..100}; do
  curl -s -X POST $CONTROLLER_URL/analyze \
    -H "Content-Type: application/json" \
    -d "{\"text\": \"Test message $i\"}" > /dev/null &
done

wait
echo "Completed 100 requests"
```

### Measure Response Time

```bash
time curl -s -X POST $CONTROLLER_URL/scrape \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}' > /dev/null
```

---

## Notes

- All timestamps are in ISO 8601 format (UTC)
- UUIDs are v4 (random)
- Tag searches use OR logic (match any of the provided tags)
- Fuzzy search uses SQL LIKE with wildcards (`%tag%`)
- Pagination is 0-indexed
- Default limit is 50, max recommended is 1000

For more information, see the [README.md](README.md) documentation.
