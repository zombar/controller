package clients

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestScraperClient_Scrape(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		mockResponse   ScraperResponse
		mockStatusCode int
		expectError    bool
	}{
		{
			name: "successful scrape",
			url:  "https://example.com",
			mockResponse: ScraperResponse{
				ID:      "test-id-123",
				URL:     "https://example.com",
				Content: "This is test content from the page",
				Metadata: map[string]interface{}{
					"title": "Test Page",
				},
			},
			mockStatusCode: http.StatusOK,
			expectError:    false,
		},
		{
			name: "successful scrape with 201",
			url:  "https://example.com/new",
			mockResponse: ScraperResponse{
				ID:      "test-id-456",
				URL:     "https://example.com/new",
				Content: "New content",
			},
			mockStatusCode: http.StatusCreated,
			expectError:    false,
		},
		{
			name:           "server error",
			url:            "https://example.com/error",
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:           "bad request",
			url:            "",
			mockStatusCode: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.URL.Path != "/api/scrape" {
					t.Errorf("Expected path /api/scrape, got %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST method, got %s", r.Method)
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockStatusCode == http.StatusOK || tt.mockStatusCode == http.StatusCreated {
					json.NewEncoder(w).Encode(tt.mockResponse)
				} else {
					json.NewEncoder(w).Encode(map[string]string{"error": "mock error"})
				}
			}))
			defer server.Close()

			// Create client
			client := NewScraperClient(server.URL)

			// Execute
			result, err := client.Scrape(tt.url)

			// Verify
			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("Expected result but got nil")
				}
				if result.ID != tt.mockResponse.ID {
					t.Errorf("Expected ID %s, got %s", tt.mockResponse.ID, result.ID)
				}
				if result.Content != tt.mockResponse.Content {
					t.Errorf("Expected Content %s, got %s", tt.mockResponse.Content, result.Content)
				}
			}
		})
	}
}

func TestScraperClient_InvalidJSON(t *testing.T) {
	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json{"))
	}))
	defer server.Close()

	client := NewScraperClient(server.URL)
	_, err := client.Scrape("https://example.com")

	if err == nil {
		t.Error("Expected error for invalid JSON but got none")
	}
}

func TestScraperClient_NetworkError(t *testing.T) {
	// Use an invalid URL that will cause network error
	client := NewScraperClient("http://localhost:99999")
	_, err := client.Scrape("https://example.com")

	if err == nil {
		t.Error("Expected network error but got none")
	}
}
