package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTextAnalyzerClient_Analyze(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		mockResponse   map[string]interface{}
		mockStatusCode int
		expectError    bool
	}{
		{
			name: "successful analysis (async queue)",
			text: "This is a test text to analyze",
			mockResponse: map[string]interface{}{
				"job_id":  "analysis-123",
				"task_id": "task-456",
				"status":  "queued",
				"message": "Analysis queued for processing",
			},
			mockStatusCode: http.StatusAccepted,
			expectError:    false,
		},
		{
			name: "analysis with 202 accepted",
			text: "Another test text",
			mockResponse: map[string]interface{}{
				"job_id":  "analysis-456",
				"task_id": "task-789",
				"status":  "queued",
				"message": "Analysis queued for processing",
			},
			mockStatusCode: http.StatusAccepted,
			expectError:    false,
		},
		{
			name:           "server error",
			text:           "Error text",
			mockStatusCode: http.StatusInternalServerError,
			expectError:    true,
		},
		{
			name:           "bad request",
			text:           "",
			mockStatusCode: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				if r.URL.Path != "/api/analyze" {
					t.Errorf("Expected path /api/analyze, got %s", r.URL.Path)
				}
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST method, got %s", r.Method)
				}

				// Send response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.mockStatusCode)
				if tt.mockStatusCode == http.StatusAccepted {
					json.NewEncoder(w).Encode(tt.mockResponse)
				} else {
					json.NewEncoder(w).Encode(map[string]string{"error": "mock error"})
				}
			}))
			defer server.Close()

			// Create client
			client := NewTextAnalyzerClient(server.URL)

			// Execute
			result, err := client.Analyze(context.Background(), tt.text)

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
				// The deprecated Analyze method now returns job_id as ID
				expectedJobID := tt.mockResponse["job_id"].(string)
				if result.ID != expectedJobID {
					t.Errorf("Expected ID %s, got %s", expectedJobID, result.ID)
				}
				if result.Metadata == nil {
					t.Error("Expected metadata but got nil")
				}
				// Verify the queued status is in metadata
				if status, ok := result.Metadata["status"].(string); !ok || status != "queued" {
					t.Errorf("Expected status 'queued' in metadata, got %v", result.Metadata["status"])
				}
			}
		})
	}
}

func TestTextAnalyzerResponse_GetTags(t *testing.T) {
	tests := []struct {
		name     string
		response TextAnalyzerResponse
		expected []string
	}{
		{
			name: "tags present",
			response: TextAnalyzerResponse{
				ID: "test-1",
				Metadata: map[string]interface{}{
					"tags": []interface{}{"tag1", "tag2", "tag3"},
				},
			},
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name: "no tags",
			response: TextAnalyzerResponse{
				ID:       "test-2",
				Metadata: map[string]interface{}{},
			},
			expected: []string{},
		},
		{
			name: "nil metadata",
			response: TextAnalyzerResponse{
				ID:       "test-3",
				Metadata: nil,
			},
			expected: []string{},
		},
		{
			name: "tags with mixed types (filtered)",
			response: TextAnalyzerResponse{
				ID: "test-4",
				Metadata: map[string]interface{}{
					"tags": []interface{}{"tag1", 123, "tag2", nil, "tag3"},
				},
			},
			expected: []string{"tag1", "tag2", "tag3"},
		},
		{
			name: "empty tags array",
			response: TextAnalyzerResponse{
				ID: "test-5",
				Metadata: map[string]interface{}{
					"tags": []interface{}{},
				},
			},
			expected: []string{},
		},
		{
			name: "tags field is not array",
			response: TextAnalyzerResponse{
				ID: "test-6",
				Metadata: map[string]interface{}{
					"tags": "not an array",
				},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.response.GetTags()

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d tags, got %d", len(tt.expected), len(result))
			}

			for i, tag := range result {
				if tag != tt.expected[i] {
					t.Errorf("Expected tag %s at index %d, got %s", tt.expected[i], i, tag)
				}
			}
		})
	}
}

func TestTextAnalyzerClient_InvalidJSON(t *testing.T) {
	// Create mock server that returns invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid json{"))
	}))
	defer server.Close()

	client := NewTextAnalyzerClient(server.URL)
	_, err := client.Analyze(context.Background(), "test text")

	if err == nil {
		t.Error("Expected error for invalid JSON but got none")
	}
}

func TestTextAnalyzerClient_NetworkError(t *testing.T) {
	// Use an invalid URL that will cause network error
	client := NewTextAnalyzerClient("http://localhost:99999")
	_, err := client.Analyze(context.Background(), "test text")

	if err == nil {
		t.Error("Expected network error but got none")
	}
}
