package storage

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	dbPath := "test_new.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	if store.db == nil {
		t.Fatal("Database connection is nil")
	}
}

func TestSaveAndGetRequest(t *testing.T) {
	dbPath := "test_save_get.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Test saving a URL-based request
	sourceURL := "https://example.com"
	scraperUUID := "scraper-123"
	req := &Request{
		ID:               "test-id-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &sourceURL,
		ScraperUUID:      &scraperUUID,
		TextAnalyzerUUID: "analyzer-123",
		Tags:             []string{"tag1", "tag2", "tag3"},
		Metadata: map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		},
	}

	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Retrieve the request
	retrieved, err := store.GetRequest("test-id-1")
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	// Verify fields
	if retrieved.ID != req.ID {
		t.Errorf("Expected ID %s, got %s", req.ID, retrieved.ID)
	}
	if retrieved.SourceType != req.SourceType {
		t.Errorf("Expected SourceType %s, got %s", req.SourceType, retrieved.SourceType)
	}
	if retrieved.TextAnalyzerUUID != req.TextAnalyzerUUID {
		t.Errorf("Expected TextAnalyzerUUID %s, got %s", req.TextAnalyzerUUID, retrieved.TextAnalyzerUUID)
	}
	if len(retrieved.Tags) != len(req.Tags) {
		t.Errorf("Expected %d tags, got %d", len(req.Tags), len(retrieved.Tags))
	}
}

func TestSaveTextRequest(t *testing.T) {
	dbPath := "test_text_request.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Test saving a text-based request (no scraper)
	req := &Request{
		ID:               "test-id-2",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "text",
		TextAnalyzerUUID: "analyzer-456",
		Tags:             []string{"text-tag1", "text-tag2"},
	}

	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save text request: %v", err)
	}

	retrieved, err := store.GetRequest("test-id-2")
	if err != nil {
		t.Fatalf("Failed to get text request: %v", err)
	}

	if retrieved.SourceType != "text" {
		t.Errorf("Expected SourceType text, got %s", retrieved.SourceType)
	}
	if retrieved.SourceURL != nil {
		t.Error("Expected SourceURL to be nil for text request")
	}
	if retrieved.ScraperUUID != nil {
		t.Error("Expected ScraperUUID to be nil for text request")
	}
}

func TestSearchByTags(t *testing.T) {
	dbPath := "test_search_tags.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create multiple requests with different tags
	requests := []*Request{
		{
			ID:               "req-1",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "text",
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"golang", "programming", "backend"},
		},
		{
			ID:               "req-2",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "text",
			TextAnalyzerUUID: "analyzer-2",
			Tags:             []string{"python", "programming", "data-science"},
		},
		{
			ID:               "req-3",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "text",
			TextAnalyzerUUID: "analyzer-3",
			Tags:             []string{"javascript", "frontend", "web"},
		},
	}

	for _, req := range requests {
		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request: %v", err)
		}
	}

	// Test exact search
	results, err := store.SearchByTags([]string{"programming"}, false)
	if err != nil {
		t.Fatalf("Failed to search tags: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'programming', got %d", len(results))
	}

	// Test fuzzy search
	results, err = store.SearchByTags([]string{"prog"}, true)
	if err != nil {
		t.Fatalf("Failed to fuzzy search tags: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results for fuzzy 'prog', got %d", len(results))
	}

	// Test multiple tags (OR search)
	results, err = store.SearchByTags([]string{"golang", "python"}, false)
	if err != nil {
		t.Fatalf("Failed to search multiple tags: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Expected 2 results for golang OR python, got %d", len(results))
	}

	// Test non-existent tag
	results, err = store.SearchByTags([]string{"nonexistent"}, false)
	if err != nil {
		t.Fatalf("Failed to search non-existent tag: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent tag, got %d", len(results))
	}
}

func TestListRequests(t *testing.T) {
	dbPath := "test_list_requests.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create multiple requests
	for i := 0; i < 10; i++ {
		req := &Request{
			ID:               fmt.Sprintf("req-%d", i),
			CreatedAt:        time.Now().UTC().Add(time.Duration(i) * time.Second),
			SourceType:       "text",
			TextAnalyzerUUID: fmt.Sprintf("analyzer-%d", i),
			Tags:             []string{"tag1"},
		}
		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request %d: %v", i, err)
		}
	}

	// Test pagination
	results, err := store.ListRequests(5, 0)
	if err != nil {
		t.Fatalf("Failed to list requests: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Expected 5 results, got %d", len(results))
	}

	// Test offset
	results, err = store.ListRequests(5, 5)
	if err != nil {
		t.Fatalf("Failed to list requests with offset: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("Expected 5 results with offset, got %d", len(results))
	}

	// Verify ordering (most recent first)
	if results[0].CreatedAt.Before(results[1].CreatedAt) {
		t.Error("Results are not ordered by created_at DESC")
	}
}

func TestGetRequestNotFound(t *testing.T) {
	dbPath := "test_not_found.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	_, err = store.GetRequest("non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent request")
	}
	if err.Error() != "request not found" {
		t.Errorf("Expected 'request not found' error, got: %v", err)
	}
}

func TestUpdateRequestMetadata(t *testing.T) {
	dbPath := "test_update_metadata.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create a request
	req := &Request{
		ID:               "test-update-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "text",
		TextAnalyzerUUID: "analyzer-1",
		Tags:             []string{"tag1"},
		Metadata: map[string]interface{}{
			"initial_key": "initial_value",
		},
	}

	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update metadata
	newMetadata := map[string]interface{}{
		"initial_key":  "updated_value",
		"new_key":      "new_value",
		"tombstone_datetime": "2025-10-19T12:34:56.789Z",
	}

	if err := store.UpdateRequestMetadata("test-update-1", newMetadata); err != nil {
		t.Fatalf("Failed to update metadata: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetRequest("test-update-1")
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	if retrieved.Metadata["initial_key"] != "updated_value" {
		t.Errorf("Expected updated_value, got %v", retrieved.Metadata["initial_key"])
	}
	if retrieved.Metadata["new_key"] != "new_value" {
		t.Errorf("Expected new_value, got %v", retrieved.Metadata["new_key"])
	}
	if retrieved.Metadata["tombstone_datetime"] != "2025-10-19T12:34:56.789Z" {
		t.Errorf("Expected tombstone_datetime, got %v", retrieved.Metadata["tombstone_datetime"])
	}
}

func TestUpdateRequestMetadataNotFound(t *testing.T) {
	dbPath := "test_update_metadata_notfound.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	err = store.UpdateRequestMetadata("non-existent-id", map[string]interface{}{
		"key": "value",
	})

	if err == nil {
		t.Error("Expected error for non-existent request")
	}
	if err.Error() != "request not found" {
		t.Errorf("Expected 'request not found' error, got: %v", err)
	}
}

func TestDeleteRequest(t *testing.T) {
	dbPath := "test_delete_request.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create a request with tags
	req := &Request{
		ID:               "test-delete-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "text",
		TextAnalyzerUUID: "analyzer-1",
		Tags:             []string{"tag1", "tag2", "tag3"},
	}

	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Verify request exists
	_, err = store.GetRequest("test-delete-1")
	if err != nil {
		t.Fatalf("Request should exist before deletion: %v", err)
	}

	// Delete the request
	if err := store.DeleteRequest("test-delete-1"); err != nil {
		t.Fatalf("Failed to delete request: %v", err)
	}

	// Verify request no longer exists
	_, err = store.GetRequest("test-delete-1")
	if err == nil {
		t.Error("Expected error after deletion")
	}
	if err.Error() != "request not found" {
		t.Errorf("Expected 'request not found' error, got: %v", err)
	}

	// Verify tags were also deleted (cascade)
	results, err := store.SearchByTags([]string{"tag1"}, false)
	if err != nil {
		t.Fatalf("Failed to search tags: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results after deletion, got %d", len(results))
	}
}

func TestDeleteRequestNotFound(t *testing.T) {
	dbPath := "test_delete_notfound.db"
	defer os.Remove(dbPath)

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	err = store.DeleteRequest("non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent request")
	}
	if err.Error() != "request not found" {
		t.Errorf("Expected 'request not found' error, got: %v", err)
	}
}
