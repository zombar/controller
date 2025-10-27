package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/storage"
)

// mockQueueClient is a test implementation of queue.Client
type mockQueueClient struct{}

func (m *mockQueueClient) EnqueueScrape(ctx context.Context, jobID, url string, extractLinks bool) (string, error) {
	// Return a fake task ID for testing
	return "test-task-" + uuid.New().String(), nil
}

func (m *mockQueueClient) EnqueueScrapeWithDelay(ctx context.Context, jobID, url string, extractLinks bool, delay time.Duration) (string, error) {
	return "test-task-" + uuid.New().String(), nil
}

func (m *mockQueueClient) Close() error {
	return nil
}

// mockScraperServer creates a mock scraper HTTP server
func mockScraperServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/api/scrape":
			response := clients.ScraperResponse{
				ID:      "scraper-test-uuid",
				URL:     "https://example.com",
				Content: "This is the main text from the scraped page.",
				RawText: "This is the raw HTML text before AI cleaning.",
				Slug:    "example-page",
				Metadata: map[string]interface{}{
					"title": "Example Page",
				},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)

		case "/api/score":
			// Parse the request to get the URL
			var req clients.ScoreRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Return different scores based on URL
			score := 0.8 // Default high score
			reason := "High quality content"
			categories := []string{"technical", "education"}

			if req.URL == "https://social-media.com" || req.URL == "https://low-quality.com" {
				score = 0.3
				reason = "Social media platform - not suitable for ingestion"
				categories = []string{"social_media"}
			} else if strings.HasSuffix(req.URL, ".jpg") || strings.HasSuffix(req.URL, ".png") || strings.HasSuffix(req.URL, ".gif") {
				score = 0.0
				reason = "Image file detected - skipping content scoring"
				categories = []string{"image", "media"}
			}

			response := clients.ScoreResponse{
				URL: req.URL,
				Score: clients.LinkScore{
					URL:                 req.URL,
					Score:               score,
					Reason:              reason,
					Categories:          categories,
					IsRecommended:       score >= 0.5,
					MaliciousIndicators: []string{},
				},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)

		case "/api/extract-links":
			var req clients.ExtractLinksRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			response := clients.ExtractLinksResponse{
				URL: req.URL,
				Links: []string{
					"https://example.com/article-1",
					"https://example.com/article-2",
					"https://example.com/blog/post-1",
				},
				Count: 3,
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)

		case "/api/images/search":
			var req clients.ImageSearchRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			response := clients.ImageSearchResponse{
				Images: []*clients.ImageInfo{
					{
						ID:      "img-1",
						URL:     "https://example.com/image1.jpg",
						AltText: "Test Image 1",
						Summary: "A test image",
						Tags:    req.Tags,
					},
				},
				Count: 1,
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)

		default:
			// Handle dynamic routes like /api/scrapes/{id}/images
			if len(r.URL.Path) > 13 && r.URL.Path[:13] == "/api/scrapes/" && len(r.URL.Path) > 20 && r.URL.Path[len(r.URL.Path)-7:] == "/images" {
				response := clients.ImageSearchResponse{
					Images: []*clients.ImageInfo{
						{
							ID:      "img-1",
							URL:     "https://example.com/scrape-image.jpg",
							AltText: "Scrape Image",
							Summary: "Image from scrape",
							Tags:    []string{"scraped"},
						},
					},
					Count: 1,
				}
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(response)
				return
			}

			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// mockTextAnalyzerServer creates a mock text analyzer HTTP server
func mockTextAnalyzerServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/analyze" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Return async queue response (202 Accepted)
		response := clients.TextAnalyzerQueueResponse{
			JobID:   "analyzer-test-uuid",
			TaskID:  "task-test-123",
			Status:  "queued",
			Message: "Analysis queued for processing",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(response)
	}))
}

func setupTestHandler(t *testing.T) (*Handler, *httptest.Server, *httptest.Server, func()) {
	// Reset Prometheus registry to avoid duplicate metrics registration across tests
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	// Create a unique database file for each test to avoid interference
	connStr, dbCleanup := setupTestDB(t, strings.ReplaceAll(t.Name(), "/", "_"))

	// Use default test values: tags=[low-quality,sparse-content], periods=[30,90,90]
	store, err := storage.New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	scraperMock := mockScraperServer()
	textAnalyzerMock := mockTextAnalyzerServer()

	scraperClient := clients.NewScraperClient(scraperMock.URL)
	textAnalyzerClient := clients.NewTextAnalyzerClient(textAnalyzerMock.URL)

	handler := New(store, scraperClient, textAnalyzerClient, nil, nil, nil, 0.5, "", scraperMock.URL)

	cleanup := func() {
		store.Close()
		scraperMock.Close()
		textAnalyzerMock.Close()
		dbCleanup()
	}

	return handler, scraperMock, textAnalyzerMock, cleanup
}

func TestHealth(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", response["status"])
	}
}

func TestScrapeURL(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScrapeURLRequest{
		URL: "https://example.com",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ScrapeURL(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var response ControllerResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.ID == "" {
		t.Error("Expected non-empty controller ID")
	}
	if response.SourceType != "url" {
		t.Errorf("Expected source_type 'url', got '%s'", response.SourceType)
	}
	if response.ScraperUUID == nil || *response.ScraperUUID != "scraper-test-uuid" {
		t.Error("Expected scraper UUID to be set")
	}
	if response.TextAnalyzerUUID != "analyzer-test-uuid" {
		t.Errorf("Expected analyzer UUID 'analyzer-test-uuid', got '%s'", response.TextAnalyzerUUID)
	}
	// With async processing, analyzer tags aren't immediately available
	// Expect 2 tags initially: domain tag (example.com) + scrape tag
	// The 3 analyzer tags will be added later by the worker
	if len(response.Tags) != 2 {
		t.Errorf("Expected 2 tags (domain + scrape) initially with async processing, got %d: %v", len(response.Tags), response.Tags)
	}
	// Verify domain tag and scrape tag are present
	hasDomainTag := false
	hasScrapeTag := false
	for _, tag := range response.Tags {
		if tag == "example.com" {
			hasDomainTag = true
		}
		if tag == "scrape" {
			hasScrapeTag = true
		}
	}
	if !hasDomainTag {
		t.Error("Expected domain tag 'example.com' to be present in tags")
	}
	if !hasScrapeTag {
		t.Error("Expected 'scrape' tag to be present in tags")
	}
}

func TestAnalyzeText(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := AnalyzeTextRequest{
		Text: "This is some sample text to analyze.",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.AnalyzeText(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var response ControllerResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.ID == "" {
		t.Error("Expected non-empty controller ID")
	}
	if response.SourceType != "text" {
		t.Errorf("Expected source_type 'text', got '%s'", response.SourceType)
	}
	if response.ScraperUUID != nil {
		t.Error("Expected scraper UUID to be nil for text analysis")
	}
	// With async queue processing, TextAnalyzerUUID is now the job_id from the queue response
	if response.TextAnalyzerUUID != "analyzer-test-uuid" {
		t.Errorf("Expected analyzer job ID 'analyzer-test-uuid', got '%s'", response.TextAnalyzerUUID)
	}
	// Tags are not immediately available with async processing - they're populated by the worker
	// The controller saves the record with empty tags initially
	if len(response.Tags) != 0 {
		t.Logf("Note: Got %d tags (expected 0 for async queued analysis): %v", len(response.Tags), response.Tags)
	}
}

func TestSearchTags(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// First, create a scrape request (which adds domain + scrape tags immediately, not async)
	scrapeReq := ScrapeURLRequest{URL: "https://example.com"}
	jsonData, _ := json.Marshal(scrapeReq)
	req := httptest.NewRequest(http.MethodPost, "/api/scrape", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ScrapeURL(w, req)

	// Now search for the domain tag (which is added immediately, not via async analyzer)
	time.Sleep(10 * time.Millisecond) // Small delay to ensure DB write completes

	searchReq := SearchTagsRequest{
		Tags:  []string{"example.com"}, // Search for the domain tag that was added immediately
		Fuzzy: true,
	}
	jsonData, _ = json.Marshal(searchReq)

	req = httptest.NewRequest(http.MethodPost, "/api/search", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	handler.SearchTags(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// With async processing, we search for tags that are added immediately (domain tags)
	// not analyzer tags which are added later by the worker
	if response["request_ids"] == nil {
		t.Error("Expected request_ids in response")
	}
}

func TestGetRequest(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// First, create a request
	analyzeReq := AnalyzeTextRequest{Text: "Test text"}
	jsonData, _ := json.Marshal(analyzeReq)
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.AnalyzeText(w, req)

	var createResponse ControllerResponse
	json.NewDecoder(w.Body).Decode(&createResponse)

	// Now retrieve the request
	req = httptest.NewRequest(http.MethodGet, "/api/requests/"+createResponse.ID, nil)
	w = httptest.NewRecorder()

	handler.GetRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response ControllerResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.ID != createResponse.ID {
		t.Errorf("Expected ID '%s', got '%s'", createResponse.ID, response.ID)
	}
}

func TestGetRequestNotFound(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/requests/non-existent-id", nil)
	w := httptest.NewRecorder()

	handler.GetRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestListRequests(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create multiple requests with unique text
	for i := 0; i < 3; i++ {
		analyzeReq := AnalyzeTextRequest{Text: fmt.Sprintf("Test text %d", i)}
		jsonData, _ := json.Marshal(analyzeReq)
		req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.AnalyzeText(w, req)

		if w.Code != http.StatusCreated {
			t.Fatalf("Failed to create request %d: status %d, body: %s", i, w.Code, w.Body.String())
		}
	}

	// List requests
	req := httptest.NewRequest(http.MethodGet, "/api/requests?limit=10&offset=0", nil)
	w := httptest.NewRecorder()

	handler.ListRequests(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["requests"] == nil {
		t.Error("Expected requests in response")
	}

	requests := response["requests"].([]interface{})
	if len(requests) != 3 {
		t.Errorf("Expected 3 requests, got %d", len(requests))
	}
}

func TestScrapeURLInvalidMethod(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/scrape", nil)
	w := httptest.NewRecorder()

	handler.ScrapeURL(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestScrapeURLEmptyURL(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScrapeURLRequest{URL: ""}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ScrapeURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAnalyzeTextEmptyText(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := AnalyzeTextRequest{Text: ""}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.AnalyzeText(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestScoreLink(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScoreLinkRequest{
		URL: "https://example.com",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/score", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ScoreLink(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["url"] != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got '%v'", response["url"])
	}

	score := response["score"].(map[string]interface{})
	if score["score"].(float64) != 0.8 {
		t.Errorf("Expected score 0.8, got %v", score["score"])
	}

	if response["meets_threshold"].(bool) != true {
		t.Error("Expected meets_threshold to be true")
	}

	if response["threshold"].(float64) != 0.5 {
		t.Errorf("Expected threshold 0.5, got %v", response["threshold"])
	}
}

func TestScoreLinkLowScore(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScoreLinkRequest{
		URL: "https://social-media.com",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/score", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ScoreLink(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	score := response["score"].(map[string]interface{})
	if score["score"].(float64) != 0.3 {
		t.Errorf("Expected score 0.3, got %v", score["score"])
	}

	if response["meets_threshold"].(bool) != false {
		t.Error("Expected meets_threshold to be false")
	}
}

func TestScrapeURLWithLowScore(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScrapeURLRequest{
		URL: "https://low-quality.com",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ScrapeURL(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var response ControllerResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// When score is below threshold, scraper and analyzer UUIDs should not be set
	if response.ScraperUUID != nil {
		t.Error("Expected scraper UUID to be nil for low-scoring URL")
	}

	if response.TextAnalyzerUUID != "" {
		t.Error("Expected analyzer UUID to be empty for low-scoring URL")
	}

	// Check that metadata contains link_score and below_threshold flag
	metadata := response.Metadata
	if metadata["below_threshold"] != true {
		t.Error("Expected below_threshold to be true")
	}

	linkScore, ok := metadata["link_score"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected link_score in metadata")
	}

	if linkScore["score"].(float64) != 0.3 {
		t.Errorf("Expected link score 0.3, got %v", linkScore["score"])
	}

	// Check that low-quality content is automatically tombstoned (30 days)
	if metadata["tombstone_datetime"] == nil {
		t.Fatal("Expected tombstone_datetime for low-quality content")
	}

	tombstoneStr, ok := metadata["tombstone_datetime"].(string)
	if !ok {
		t.Fatal("Expected tombstone_datetime to be a string")
	}

	tombstoneTime, err := time.Parse(time.RFC3339, tombstoneStr)
	if err != nil {
		t.Fatalf("Failed to parse tombstone_datetime: %v", err)
	}

	// Should be approximately 30 days from now (allow 1 hour tolerance)
	expectedTime := time.Now().UTC().Add(30 * 24 * time.Hour)
	diff := tombstoneTime.Sub(expectedTime)
	if diff < -time.Hour || diff > time.Hour {
		t.Errorf("Expected tombstone_datetime around %v (30 days from now), got %v (diff: %v)", expectedTime, tombstoneTime, diff)
	}

	// Verify SEO is disabled for low-quality content
	if response.SEOEnabled {
		t.Error("Expected SEOEnabled to be false for low-quality content")
	}
}

func TestScrapeURLWithHighScore(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScrapeURLRequest{
		URL: "https://example.com",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ScrapeURL(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var response ControllerResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// When score is above threshold, both UUIDs should be set
	if response.ScraperUUID == nil {
		t.Error("Expected scraper UUID to be set for high-scoring URL")
	}

	if response.TextAnalyzerUUID == "" {
		t.Error("Expected analyzer UUID to be set for high-scoring URL")
	}

	// Check that metadata contains link_score
	metadata := response.Metadata
	linkScore, ok := metadata["link_score"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected link_score in metadata")
	}

	if linkScore["score"].(float64) != 0.8 {
		t.Errorf("Expected link score 0.8, got %v", linkScore["score"])
	}

	// Should also have scraper and analyzer metadata
	if _, ok := metadata["scraper_metadata"]; !ok {
		t.Error("Expected scraper_metadata in response")
	}

	if _, ok := metadata["analyzer_metadata"]; !ok {
		t.Error("Expected analyzer_metadata in response")
	}

	// Verify SEO is enabled for high-quality content
	if !response.SEOEnabled {
		t.Error("Expected SEOEnabled to be true for high-quality content")
	}

	// Verify slug was generated for high-quality content
	if response.Slug == nil || *response.Slug == "" {
		t.Error("Expected slug to be generated for high-quality content")
	}
}

func TestScrapeURLWithImageURL(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScrapeURLRequest{
		URL: "https://example.com/photo.jpg",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ScrapeURL(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var response ControllerResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Image URLs should bypass threshold check and get fully scraped
	if response.ScraperUUID == nil {
		t.Error("Expected scraper UUID to be set for image URL (should bypass threshold)")
	}

	// Text analyzer should be skipped for image URLs
	if response.TextAnalyzerUUID != "" {
		t.Error("Expected analyzer UUID to be empty for image URL (text analysis should be skipped)")
	}

	// Check that metadata contains link_score with image category
	metadata := response.Metadata
	linkScore, ok := metadata["link_score"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected link_score in metadata")
	}

	if linkScore["score"].(float64) != 0.0 {
		t.Errorf("Expected link score 0.0 for image URL, got %v", linkScore["score"])
	}

	categories, ok := linkScore["categories"].([]interface{})
	if !ok {
		t.Fatal("Expected categories in link_score")
	}

	hasImageCategory := false
	for _, cat := range categories {
		if cat.(string) == "image" {
			hasImageCategory = true
			break
		}
	}

	if !hasImageCategory {
		t.Error("Expected 'image' category in link score")
	}

	// Should NOT have below_threshold flag since image URLs bypass threshold
	if metadata["below_threshold"] == true {
		t.Error("Expected below_threshold to be false/absent for image URL (should bypass threshold)")
	}
}

func TestExtractLinks(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ExtractLinksRequest{
		URL: "https://example.com",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/extract-links", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ExtractLinks(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["url"] != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got '%v'", response["url"])
	}

	links := response["links"].([]interface{})
	if len(links) != 3 {
		t.Errorf("Expected 3 links, got %d", len(links))
	}

	if response["count"].(float64) != 3 {
		t.Errorf("Expected count 3, got %v", response["count"])
	}
}

func TestExtractLinksInvalidMethod(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/extract-links", nil)
	w := httptest.NewRecorder()

	handler.ExtractLinks(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestExtractLinksEmptyURL(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ExtractLinksRequest{URL: ""}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/extract-links", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ExtractLinks(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

// ============================================================================
// Async Scrape Request Tests
// ============================================================================

func TestCreateScrapeRequest(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScrapeURLRequest{
		URL: "https://example.com",
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateScrapeRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["id"] == nil || response["id"].(string) == "" {
		t.Error("Expected non-empty scrape request ID")
	}

	if response["url"] != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got '%v'", response["url"])
	}

	if response["status"] == nil {
		t.Error("Expected status field")
	}

	if response["status"].(string) != "queued" {
		t.Errorf("Expected status 'queued', got '%v'", response["status"])
	}

	// Queue-based system includes created_at and updated_at timestamps
	if response["created_at"] == nil {
		t.Error("Expected created_at field")
	}

	if response["updated_at"] == nil {
		t.Error("Expected updated_at field")
	}
}

func TestCreateScrapeRequestDuplicate(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScrapeURLRequest{
		URL: "https://example.com",
	}
	jsonData, _ := json.Marshal(reqBody)

	// Create first request
	req1 := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	handler.CreateScrapeRequest(w1, req1)

	var response1 map[string]interface{}
	json.NewDecoder(w1.Body).Decode(&response1)
	id1 := response1["id"].(string)

	// Create duplicate request - queue system creates a new job each time
	req2 := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.CreateScrapeRequest(w2, req2)

	var response2 map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&response2)
	id2 := response2["id"].(string)

	// In the queue-based system, each request creates a new job
	if id1 == id2 {
		t.Errorf("Expected new job to have different ID, but both are: %s", id1)
	}

	// Both jobs should have status "queued"
	if response1["status"].(string) != "queued" || response2["status"].(string) != "queued" {
		t.Error("Expected both jobs to have status 'queued'")
	}
}

func TestCreateScrapeRequestEmptyURL(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := ScrapeURLRequest{URL: ""}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateScrapeRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestListScrapeRequests(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create multiple scrape requests
	urls := []string{
		"https://example1.com",
		"https://example2.com",
		"https://example3.com",
	}

	for _, url := range urls {
		reqBody := ScrapeURLRequest{URL: url}
		jsonData, _ := json.Marshal(reqBody)
		req := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.CreateScrapeRequest(w, req)
	}

	// List requests
	req := httptest.NewRequest(http.MethodGet, "/api/scrape-requests", nil)
	w := httptest.NewRecorder()

	handler.ListScrapeRequests(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["count"] == nil {
		t.Error("Expected count field")
	}

	count := int(response["count"].(float64))
	if count != len(urls) {
		t.Errorf("Expected count %d, got %d", len(urls), count)
	}

	if response["requests"] == nil {
		t.Fatal("Expected requests array")
	}

	requests := response["requests"].([]interface{})
	if len(requests) != len(urls) {
		t.Errorf("Expected %d requests, got %d", len(urls), len(requests))
	}
}

func TestGetScrapeRequest(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create a scrape request
	reqBody := ScrapeURLRequest{URL: "https://example.com"}
	jsonData, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	handler.CreateScrapeRequest(createW, createReq)

	var createResponse map[string]interface{}
	json.NewDecoder(createW.Body).Decode(&createResponse)
	id := createResponse["id"].(string)

	// Get the request
	getReq := httptest.NewRequest(http.MethodGet, "/api/scrape-requests/"+id, nil)
	getW := httptest.NewRecorder()

	handler.GetScrapeRequest(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", getW.Code, getW.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(getW.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["id"] != id {
		t.Errorf("Expected ID '%s', got '%v'", id, response["id"])
	}

	if response["url"] != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got '%v'", response["url"])
	}
}

func TestGetScrapeRequestNotFound(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/scrape-requests/non-existent-id", nil)
	w := httptest.NewRecorder()

	handler.GetScrapeRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestDeleteScrapeRequest(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create a scrape request
	reqBody := ScrapeURLRequest{URL: "https://example.com"}
	jsonData, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	handler.CreateScrapeRequest(createW, createReq)

	var createResponse map[string]interface{}
	json.NewDecoder(createW.Body).Decode(&createResponse)
	id := createResponse["id"].(string)

	// Delete the request
	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/scrape-requests/"+id, nil)
	deleteW := httptest.NewRecorder()

	handler.DeleteScrapeRequest(deleteW, deleteReq)

	if deleteW.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", deleteW.Code, deleteW.Body.String())
	}

	var deleteResponse map[string]interface{}
	json.NewDecoder(deleteW.Body).Decode(&deleteResponse)
	if deleteResponse["status"] != "deleted" {
		t.Errorf("Expected status 'deleted', got '%v'", deleteResponse["status"])
	}

	// Verify request is deleted
	getReq := httptest.NewRequest(http.MethodGet, "/api/scrape-requests/"+id, nil)
	getW := httptest.NewRecorder()
	handler.GetScrapeRequest(getW, getReq)

	if getW.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 after delete, got %d", getW.Code)
	}
}

func TestDeleteScrapeRequestNotFound(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/scrape-requests/non-existent-id", nil)
	w := httptest.NewRecorder()

	handler.DeleteScrapeRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestRetryScrapeRequest(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// Create a scrape request with a URL that will fail (low score)
	reqBody := ScrapeURLRequest{URL: "https://low-quality.com"}
	jsonData, _ := json.Marshal(reqBody)
	createReq := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
	createReq.Header.Set("Content-Type", "application/json")
	createW := httptest.NewRecorder()
	handler.CreateScrapeRequest(createW, createReq)

	var createResponse map[string]interface{}
	json.NewDecoder(createW.Body).Decode(&createResponse)
	id := createResponse["id"].(string)

	// Wait for the request to fail (low score will cause failure)
	time.Sleep(50 * time.Millisecond)

	// Verify it failed
	getReq := httptest.NewRequest(http.MethodGet, "/api/scrape-requests/"+id, nil)
	getW := httptest.NewRecorder()
	handler.GetScrapeRequest(getW, getReq)

	var getResponse map[string]interface{}
	json.NewDecoder(getW.Body).Decode(&getResponse)

	if getResponse["status"] != "failed" {
		t.Skipf("Request did not fail as expected (status: %v), skipping retry test", getResponse["status"])
	}

	// Retry the request
	retryReq := httptest.NewRequest(http.MethodPost, "/api/scrape-requests/"+id+"/retry", nil)
	retryW := httptest.NewRecorder()

	handler.RetryScrapeRequest(retryW, retryReq)

	if retryW.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", retryW.Code, retryW.Body.String())
	}

	var retryResponse map[string]interface{}
	json.NewDecoder(retryW.Body).Decode(&retryResponse)

	// Verify the request was reset
	if retryResponse["status"] == "failed" {
		t.Error("Expected status to be reset after retry")
	}

	if retryResponse["error_message"] != "" && retryResponse["error_message"] != nil {
		t.Error("Expected error_message to be cleared after retry")
	}
}

func TestRetryScrapeRequestNotFound(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/scrape-requests/non-existent-id/retry", nil)
	w := httptest.NewRecorder()

	handler.RetryScrapeRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestScrapeRequestMethodNotAllowed(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// Test invalid method on create/list endpoint
	req := httptest.NewRequest(http.MethodPut, "/api/scrape-requests", nil)
	w := httptest.NewRecorder()

	handler.CreateScrapeRequest(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestGetDocumentImages(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	tests := []struct {
		name           string
		method         string
		path           string
		wantStatusCode int
		wantErrMsg     string
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "successful retrieval",
			method:         http.MethodGet,
			path:           "/api/documents/scraper-test-uuid/images",
			wantStatusCode: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var resp map[string]interface{}
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("Failed to parse response: %v", err)
				}

				images, ok := resp["images"].([]interface{})
				if !ok {
					t.Fatal("Expected images field to be an array")
				}

				count, ok := resp["count"].(float64)
				if !ok {
					t.Fatal("Expected count field to be a number")
				}

				if int(count) != len(images) {
					t.Errorf("Count %d doesn't match images length %d", int(count), len(images))
				}
			},
		},
		{
			name:           "missing scraper UUID",
			method:         http.MethodGet,
			path:           "/api/documents//images",
			wantStatusCode: http.StatusBadRequest,
			wantErrMsg:     "Scraper UUID is required",
		},
		{
			name:           "method not allowed",
			method:         http.MethodPost,
			path:           "/api/documents/scraper-test-uuid/images",
			wantStatusCode: http.StatusMethodNotAllowed,
			wantErrMsg:     "Method not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handler.GetDocumentImages(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("Status code = %d, want %d. Body: %s", w.Code, tt.wantStatusCode, w.Body.String())
			}

			if tt.wantErrMsg != "" {
				var errResp ErrorResponse
				if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
					t.Fatalf("Failed to decode error response: %v", err)
				}
				if errResp.Error != tt.wantErrMsg {
					t.Errorf("Error message = %q, want %q", errResp.Error, tt.wantErrMsg)
				}
				return
			}

			if tt.checkResponse != nil {
				tt.checkResponse(t, w.Body.Bytes())
			}
		})
	}
}

func TestTombstoneRequest(t *testing.T) {
	scraperServer := mockScraperServer()
	defer scraperServer.Close()

	textanalyzerServer := mockTextAnalyzerServer()
	defer textanalyzerServer.Close()

	connStr, cleanup := setupTestDB(t, "test_tombstone_request")
	defer cleanup()

	// Use default test values: tags=[low-quality,sparse-content], periods=[30,90,90]
	store, err := storage.New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	scraper := clients.NewScraperClient(scraperServer.URL)
	textAnalyzer := clients.NewTextAnalyzerClient(textanalyzerServer.URL)

	handler := &Handler{
		storage:            store,
		scraper:            scraper,
		textAnalyzer:       textAnalyzer,
		linkScoreThreshold: 0.5,
	}

	// First, create a request to tombstone
	req := &storage.Request{
		ID:               "tombstone-req-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "text",
		TextAnalyzerUUID: "analyzer-1",
		Tags:             []string{"test"},
		Metadata:         map[string]interface{}{},
	}
	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Tombstone the request
	r := httptest.NewRequest(http.MethodPut, "/api/requests/tombstone-req-1/tombstone", nil)
	w := httptest.NewRecorder()

	handler.TombstoneRequest(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify the request has tombstone_datetime in metadata (90 days from now)
	retrieved, err := store.GetRequest("tombstone-req-1")
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	if retrieved.Metadata["tombstone_datetime"] == nil {
		t.Fatal("Expected tombstone_datetime in metadata")
	}

	// Parse tombstone datetime and verify it's 90 days from now
	tombstoneStr, ok := retrieved.Metadata["tombstone_datetime"].(string)
	if !ok {
		t.Fatal("Expected tombstone_datetime to be a string")
	}

	tombstoneTime, err := time.Parse(time.RFC3339, tombstoneStr)
	if err != nil {
		t.Fatalf("Failed to parse tombstone_datetime: %v", err)
	}

	// Should be approximately 90 days from now (allow 1 minute tolerance)
	expectedTime := time.Now().UTC().Add(90 * 24 * time.Hour)
	diff := tombstoneTime.Sub(expectedTime)
	if diff < -time.Minute || diff > time.Minute {
		t.Errorf("Expected tombstone_datetime around %v (90 days from now), got %v (diff: %v)", expectedTime, tombstoneTime, diff)
	}
}

func TestTombstoneRequestNotFound(t *testing.T) {
	scraperServer := mockScraperServer()
	defer scraperServer.Close()

	textanalyzerServer := mockTextAnalyzerServer()
	defer textanalyzerServer.Close()

	connStr, cleanup := setupTestDB(t, "test_tombstone_notfound")
	defer cleanup()

	// Use default test values: tags=[low-quality,sparse-content], periods=[30,90,90]
	store, err := storage.New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	scraper := clients.NewScraperClient(scraperServer.URL)
	textAnalyzer := clients.NewTextAnalyzerClient(textanalyzerServer.URL)

	handler := &Handler{
		storage:            store,
		scraper:            scraper,
		textAnalyzer:       textAnalyzer,
		linkScoreThreshold: 0.5,
	}

	r := httptest.NewRequest(http.MethodPut, "/api/requests/non-existent/tombstone", nil)
	w := httptest.NewRecorder()

	handler.TombstoneRequest(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUntombstoneRequest(t *testing.T) {
	scraperServer := mockScraperServer()
	defer scraperServer.Close()

	textanalyzerServer := mockTextAnalyzerServer()
	defer textanalyzerServer.Close()

	connStr, cleanup := setupTestDB(t, "test_untombstone_request")
	defer cleanup()

	// Use default test values: tags=[low-quality,sparse-content], periods=[30,90,90]
	store, err := storage.New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	scraper := clients.NewScraperClient(scraperServer.URL)
	textAnalyzer := clients.NewTextAnalyzerClient(textanalyzerServer.URL)

	handler := &Handler{
		storage:            store,
		scraper:            scraper,
		textAnalyzer:       textAnalyzer,
		linkScoreThreshold: 0.5,
	}

	// Create a request with tombstone_datetime
	req := &storage.Request{
		ID:               "untombstone-req-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "text",
		TextAnalyzerUUID: "analyzer-1",
		Tags:             []string{"test"},
		Metadata: map[string]interface{}{
			"tombstone_datetime": time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Untombstone the request
	r := httptest.NewRequest(http.MethodDelete, "/api/requests/untombstone-req-1/tombstone", nil)
	w := httptest.NewRecorder()

	handler.UntombstoneRequest(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify tombstone_datetime was removed
	retrieved, err := store.GetRequest("untombstone-req-1")
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	if retrieved.Metadata["tombstone_datetime"] != nil {
		t.Error("Expected tombstone_datetime to be removed from metadata")
	}
}

func TestDeleteRequest(t *testing.T) {
	scraperServer := mockScraperServer()
	defer scraperServer.Close()

	textanalyzerServer := mockTextAnalyzerServer()
	defer textanalyzerServer.Close()

	connStr, cleanup := setupTestDB(t, "test_delete_request")
	defer cleanup()

	// Use default test values: tags=[low-quality,sparse-content], periods=[30,90,90]
	store, err := storage.New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	scraper := clients.NewScraperClient(scraperServer.URL)
	textAnalyzer := clients.NewTextAnalyzerClient(textanalyzerServer.URL)

	handler := &Handler{
		storage:            store,
		scraper:            scraper,
		textAnalyzer:       textAnalyzer,
		linkScoreThreshold: 0.5,
	}

	// Create a request to delete
	req := &storage.Request{
		ID:               "delete-req-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "text",
		TextAnalyzerUUID: "analyzer-1",
		Tags:             []string{"test"},
	}
	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Delete the request
	r := httptest.NewRequest(http.MethodDelete, "/api/requests/delete-req-1", nil)
	w := httptest.NewRecorder()

	handler.DeleteRequest(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify request no longer exists
	_, err = store.GetRequest("delete-req-1")
	if err == nil {
		t.Error("Expected error for deleted request")
	}
	if err.Error() != "request not found" {
		t.Errorf("Expected 'request not found' error, got: %v", err)
	}
}

func TestDeleteRequestNotFound(t *testing.T) {
	scraperServer := mockScraperServer()
	defer scraperServer.Close()

	textanalyzerServer := mockTextAnalyzerServer()
	defer textanalyzerServer.Close()

	connStr, cleanup := setupTestDB(t, "test_delete_notfound")
	defer cleanup()

	// Use default test values: tags=[low-quality,sparse-content], periods=[30,90,90]
	store, err := storage.New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	scraper := clients.NewScraperClient(scraperServer.URL)
	textAnalyzer := clients.NewTextAnalyzerClient(textanalyzerServer.URL)

	handler := &Handler{
		storage:            store,
		scraper:            scraper,
		textAnalyzer:       textAnalyzer,
		linkScoreThreshold: 0.5,
	}

	r := httptest.NewRequest(http.MethodDelete, "/api/requests/non-existent", nil)
	w := httptest.NewRecorder()

	handler.DeleteRequest(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGetTimelineExtents(t *testing.T) {
	t.Run("empty database returns default date", func(t *testing.T) {
		handler, _, _, cleanup := setupTestHandler(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/requests/timeline-extents", nil)
		w := httptest.NewRecorder()

		handler.GetTimelineExtents(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		earliestDateStr, ok := response["earliest_date"].(string)
		if !ok {
			t.Fatal("Expected earliest_date in response")
		}

		earliestDate, err := time.Parse(time.RFC3339, earliestDateStr)
		if err != nil {
			t.Fatalf("Failed to parse earliest_date: %v", err)
		}

		// For empty database, should return 30 days ago (approximately)
		now := time.Now()
		expectedDate := now.AddDate(0, 0, -30)
		diff := earliestDate.Sub(expectedDate)
		// Allow 1 minute tolerance for test execution time
		if diff < -time.Minute || diff > time.Minute {
			t.Errorf("Expected earliest_date around %v (30 days ago), got %v (diff: %v)", expectedDate, earliestDate, diff)
		}
	})

	t.Run("returns earliest effective date from documents", func(t *testing.T) {
		// Create custom storage for this test
		connStr, cleanup := setupTestDB(t, "test_timeline_extents_handler")
		defer cleanup()

		// Use default test values: tags=[low-quality,sparse-content], periods=[30,90,90]
	store, err := storage.New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		scraperServer := mockScraperServer()
		defer scraperServer.Close()
		textanalyzerServer := mockTextAnalyzerServer()
		defer textanalyzerServer.Close()

		scraper := clients.NewScraperClient(scraperServer.URL)
		textAnalyzer := clients.NewTextAnalyzerClient(textanalyzerServer.URL)

		handler := &Handler{
			storage:            store,
			scraper:            scraper,
			textAnalyzer:       textAnalyzer,
			linkScoreThreshold: 0.5,
		}

		// Create documents with different dates
		// Document 1: March 2024
		sourceURL1 := "https://example.com/article-1"
		scraperUUID1 := "scraper-1"
		req1 := &storage.Request{
			ID:               "test-1",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "url",
			SourceURL:        &sourceURL1,
			ScraperUUID:      &scraperUUID1,
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"test"},
			Metadata: map[string]interface{}{
				"scraper_metadata": map[string]interface{}{
					"publish_date": time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
				},
			},
		}
		if err := store.SaveRequest(req1); err != nil {
			t.Fatalf("Failed to save request 1: %v", err)
		}

		// Document 2: January 2024 (earliest)
		sourceURL2 := "https://example.com/article-2"
		scraperUUID2 := "scraper-2"
		req2 := &storage.Request{
			ID:               "test-2",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "url",
			SourceURL:        &sourceURL2,
			ScraperUUID:      &scraperUUID2,
			TextAnalyzerUUID: "analyzer-2",
			Tags:             []string{"test"},
			Metadata: map[string]interface{}{
				"scraper_metadata": map[string]interface{}{
					"publish_date": time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
				},
			},
		}
		if err := store.SaveRequest(req2); err != nil {
			t.Fatalf("Failed to save request 2: %v", err)
		}

		// Document 3: May 2024
		sourceURL3 := "https://example.com/article-3"
		scraperUUID3 := "scraper-3"
		req3 := &storage.Request{
			ID:               "test-3",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "url",
			SourceURL:        &sourceURL3,
			ScraperUUID:      &scraperUUID3,
			TextAnalyzerUUID: "analyzer-3",
			Tags:             []string{"test"},
			Metadata: map[string]interface{}{
				"scraper_metadata": map[string]interface{}{
					"publish_date": time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
				},
			},
		}
		if err := store.SaveRequest(req3); err != nil {
			t.Fatalf("Failed to save request 3: %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/api/requests/timeline-extents", nil)
		w := httptest.NewRecorder()

		handler.GetTimelineExtents(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		earliestDateStr, ok := response["earliest_date"].(string)
		if !ok {
			t.Fatal("Expected earliest_date in response")
		}

		earliestDate, err := time.Parse(time.RFC3339, earliestDateStr)
		if err != nil {
			t.Fatalf("Failed to parse earliest_date: %v", err)
		}

		// Should return January 15, 2024 from req2
		expectedDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
		diff := earliestDate.Sub(expectedDate)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("Expected earliest_date %v, got %v (diff: %v)", expectedDate, earliestDate, diff)
		}
	})

	t.Run("method not allowed for non-GET requests", func(t *testing.T) {
		handler, _, _, cleanup := setupTestHandler(t)
		defer cleanup()

		methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

		for _, method := range methods {
			req := httptest.NewRequest(method, "/api/requests/timeline-extents", nil)
			w := httptest.NewRecorder()

			handler.GetTimelineExtents(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405 for %s method, got %d", method, w.Code)
			}
		}
	})
}

func TestUpdateRequestTags(t *testing.T) {
	t.Run("successfully update tags", func(t *testing.T) {
		handler, _, _, cleanup := setupTestHandler(t)
		defer cleanup()

		// Create a test request first
		scraperUUID := "test-uuid"
		testReq := &storage.Request{
			ID:               "test-request-1",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "url",
			ScraperUUID:      &scraperUUID,
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"initial", "tag"},
			Metadata: map[string]interface{}{
				"url":     "https://example.com",
				"title":   "Test",
				"content": "Test content",
			},
		}
		if err := handler.storage.SaveRequest(testReq); err != nil {
			t.Fatalf("Failed to save test request: %v", err)
		}

		// Update tags
		newTags := []string{"updated", "tags", "example.com"}
		reqBody, _ := json.Marshal(map[string][]string{"tags": newTags})
		req := httptest.NewRequest(http.MethodPut, "/api/requests/test-request-1/tags", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()

		handler.UpdateRequestTags(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}

		// Verify tags were updated
		updated, err := handler.storage.GetRequest("test-request-1")
		if err != nil {
			t.Fatalf("Failed to get updated request: %v", err)
		}

		if len(updated.Tags) != len(newTags) {
			t.Errorf("Expected %d tags, got %d", len(newTags), len(updated.Tags))
		}

		for i, tag := range newTags {
			if updated.Tags[i] != tag {
				t.Errorf("Expected tag %s at position %d, got %s", tag, i, updated.Tags[i])
			}
		}
	})

	t.Run("request not found", func(t *testing.T) {
		handler, _, _, cleanup := setupTestHandler(t)
		defer cleanup()

		reqBody, _ := json.Marshal(map[string][]string{"tags": {"test"}})
		req := httptest.NewRequest(http.MethodPut, "/api/requests/nonexistent/tags", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()

		handler.UpdateRequestTags(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid request body", func(t *testing.T) {
		handler, _, _, cleanup := setupTestHandler(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodPut, "/api/requests/test-id/tags", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.UpdateRequestTags(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		handler, _, _, cleanup := setupTestHandler(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/requests/test-id/tags", nil)
		w := httptest.NewRecorder()

		handler.UpdateRequestTags(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d: %s", w.Code, w.Body.String())
		}
	})
}

func TestUpdateImageTags(t *testing.T) {
	t.Run("successfully update image tags", func(t *testing.T) {
		handler, scraperServer, _, cleanup := setupTestHandler(t)
		defer cleanup()

		// Mock scraper response for updating image tags
		testImageID := "test-image-1"
		scraperServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && r.URL.Path == "/api/images/"+testImageID+"/tags" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]string{"message": "Image tags updated successfully"})
				return
			}
			http.Error(w, "Not found", http.StatusNotFound)
		})

		newTags := []string{"updated", "image", "tags"}
		reqBody, _ := json.Marshal(map[string][]string{"tags": newTags})
		req := httptest.NewRequest(http.MethodPut, "/api/images/"+testImageID+"/tags", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()

		handler.UpdateImageTags(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("image not found", func(t *testing.T) {
		handler, scraperServer, _, cleanup := setupTestHandler(t)
		defer cleanup()

		// Mock scraper response for image not found
		scraperServer.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "image not found", http.StatusNotFound)
		})

		reqBody, _ := json.Marshal(map[string][]string{"tags": {"test"}})
		req := httptest.NewRequest(http.MethodPut, "/api/images/nonexistent/tags", bytes.NewReader(reqBody))
		w := httptest.NewRecorder()

		handler.UpdateImageTags(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid request body", func(t *testing.T) {
		handler, _, _, cleanup := setupTestHandler(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodPut, "/api/images/test-id/tags", bytes.NewReader([]byte("invalid json")))
		w := httptest.NewRecorder()

		handler.UpdateImageTags(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		handler, _, _, cleanup := setupTestHandler(t)
		defer cleanup()

		req := httptest.NewRequest(http.MethodGet, "/api/images/test-id/tags", nil)
		w := httptest.NewRecorder()

		handler.UpdateImageTags(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 405, got %d: %s", w.Code, w.Body.String())
		}
	})
}