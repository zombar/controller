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

	if response["progress"] == nil {
		t.Error("Expected progress field")
	}

	if response["expires_at"] == nil {
		t.Error("Expected expires_at field")
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

	// Create duplicate request
	req2 := httptest.NewRequest(http.MethodPost, "/api/scrape-requests", bytes.NewBuffer(jsonData))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.CreateScrapeRequest(w2, req2)

	var response2 map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&response2)
	id2 := response2["id"].(string)

	if id1 != id2 {
		t.Errorf("Expected duplicate URL to return same request ID: %s != %s", id1, id2)
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
