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
		if r.URL.Path != "/api/scrape" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		response := clients.ScraperResponse{
			ID:      "scraper-test-uuid",
			URL:     "https://example.com",
			Content: "This is the main text from the scraped page.",
			Metadata: map[string]interface{}{
				"title": "Example Page",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
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

	handler := New(store, scraperClient, textAnalyzerClient)

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
