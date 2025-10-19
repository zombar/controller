package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/storage"
)

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

		case "/api/scrape/batch":
			var req clients.BatchScrapeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			results := []clients.BatchResult{}
			successCount := 0
			cachedCount := 0

			for _, url := range req.URLs {
				if url == "https://invalid-url.com" {
					results = append(results, clients.BatchResult{
						URL:     url,
						Success: false,
						Error:   "failed to fetch page",
						Cached:  false,
					})
				} else {
					cached := !req.Force && url == "https://cached.com"
					if cached {
						cachedCount++
					}
					results = append(results, clients.BatchResult{
						URL:     url,
						Success: true,
						Data: &clients.ScrapedData{
							ID:      "batch-test-uuid-" + url,
							URL:     url,
							Title:   "Test Page",
							Content: "Test content",
							Cached:  cached,
						},
						Cached: cached,
					})
					successCount++
				}
			}

			response := clients.BatchScrapeResponse{
				Results: results,
				Summary: clients.BatchSummary{
					Total:   len(req.URLs),
					Success: successCount,
					Failed:  len(req.URLs) - successCount,
					Cached:  cachedCount,
					Scraped: successCount - cachedCount,
				},
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(response)

		default:
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

		response := clients.TextAnalyzerResponse{
			ID: "analyzer-test-uuid",
			Metadata: map[string]interface{}{
				"language": "en",
				"tags":     []interface{}{"example", "test", "content"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
}

func setupTestHandler(t *testing.T) (*Handler, *httptest.Server, *httptest.Server, func()) {
	dbPath := "test_handlers.db"

	store, err := storage.New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}

	scraperMock := mockScraperServer()
	textAnalyzerMock := mockTextAnalyzerServer()

	scraperClient := clients.NewScraperClient(scraperMock.URL)
	textAnalyzerClient := clients.NewTextAnalyzerClient(textAnalyzerMock.URL)

	handler := New(store, scraperClient, textAnalyzerClient, 0.5)

	cleanup := func() {
		store.Close()
		scraperMock.Close()
		textAnalyzerMock.Close()
		os.Remove(dbPath)
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
	if len(response.Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(response.Tags))
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
	if response.TextAnalyzerUUID != "analyzer-test-uuid" {
		t.Errorf("Expected analyzer UUID 'analyzer-test-uuid', got '%s'", response.TextAnalyzerUUID)
	}
	if len(response.Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(response.Tags))
	}
}

func TestSearchTags(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	// First, create some requests with tags
	analyzeReq := AnalyzeTextRequest{Text: "Test text"}
	jsonData, _ := json.Marshal(analyzeReq)
	req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.AnalyzeText(w, req)

	// Now search for tags
	time.Sleep(10 * time.Millisecond) // Small delay to ensure DB write completes

	searchReq := SearchTagsRequest{
		Tags:  []string{"test"},
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

	// Create multiple requests
	for i := 0; i < 3; i++ {
		analyzeReq := AnalyzeTextRequest{Text: "Test text"}
		jsonData, _ := json.Marshal(analyzeReq)
		req := httptest.NewRequest(http.MethodPost, "/api/analyze", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.AnalyzeText(w, req)
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

func TestBatchScrape(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := BatchScrapeRequest{
		URLs:  []string{"https://example.com", "https://example.org"},
		Force: false,
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape/batch", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.BatchScrape(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	results := response["results"].([]interface{})
	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	summary := response["summary"].(map[string]interface{})
	if summary["total"].(float64) != 2 {
		t.Errorf("Expected total 2, got %v", summary["total"])
	}

	if summary["success"].(float64) != 2 {
		t.Errorf("Expected success 2, got %v", summary["success"])
	}
}

func TestBatchScrapeWithFailures(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := BatchScrapeRequest{
		URLs:  []string{"https://example.com", "https://invalid-url.com"},
		Force: false,
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape/batch", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.BatchScrape(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	summary := response["summary"].(map[string]interface{})
	if summary["success"].(float64) != 1 {
		t.Errorf("Expected success 1, got %v", summary["success"])
	}

	if summary["failed"].(float64) != 1 {
		t.Errorf("Expected failed 1, got %v", summary["failed"])
	}
}

func TestBatchScrapeWithCache(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := BatchScrapeRequest{
		URLs:  []string{"https://cached.com", "https://example.com"},
		Force: false,
	}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape/batch", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.BatchScrape(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	summary := response["summary"].(map[string]interface{})
	if summary["cached"].(float64) != 1 {
		t.Errorf("Expected cached 1, got %v", summary["cached"])
	}

	if summary["scraped"].(float64) != 1 {
		t.Errorf("Expected scraped 1, got %v", summary["scraped"])
	}
}

func TestBatchScrapeInvalidMethod(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/scrape/batch", nil)
	w := httptest.NewRecorder()

	handler.BatchScrape(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestBatchScrapeEmptyURLs(t *testing.T) {
	handler, _, _, cleanup := setupTestHandler(t)
	defer cleanup()

	reqBody := BatchScrapeRequest{URLs: []string{}}
	jsonData, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/scrape/batch", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.BatchScrape(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}
