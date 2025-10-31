package storage

import (
	"fmt"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_new")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	if store.db == nil {
		t.Fatal("Database connection is nil")
	}
}

func TestSaveAndGetRequest(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_save_get")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Test saving a URL-based request
	sourceURL := "https://example.com"
	scraperUUID := "scraper-123"
	slug := "example-article"
	req := &Request{
		ID:               "test-id-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &sourceURL,
		ScraperUUID:      &scraperUUID,
		TextAnalyzerUUID: "analyzer-123",
		Tags:             []string{"tag1", "tag2", "tag3"},
		Slug:             &slug,
		SEOEnabled:       true,
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
	if retrieved.SEOEnabled != req.SEOEnabled {
		t.Errorf("Expected SEOEnabled %v, got %v", req.SEOEnabled, retrieved.SEOEnabled)
	}
	if retrieved.Slug == nil || *retrieved.Slug != *req.Slug {
		t.Errorf("Expected Slug %s, got %v", *req.Slug, retrieved.Slug)
	}
}

func TestSaveTextRequest(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_text_request")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
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
		SEOEnabled:       false, // SEO typically disabled for text-based requests
		Metadata:         map[string]interface{}{},
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
	connStr, cleanup := setupTestDB(t, "test_search_tags")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
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
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		{
			ID:               "req-2",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "text",
			TextAnalyzerUUID: "analyzer-2",
			Tags:             []string{"python", "programming", "data-science"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		{
			ID:               "req-3",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "text",
			TextAnalyzerUUID: "analyzer-3",
			Tags:             []string{"javascript", "frontend", "web"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
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
	connStr, cleanup := setupTestDB(t, "test_list_requests")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
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
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
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
	connStr, cleanup := setupTestDB(t, "test_not_found")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
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
	connStr, cleanup := setupTestDB(t, "test_update_metadata")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
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
	connStr, cleanup := setupTestDB(t, "test_update_metadata_notfound")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
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
	connStr, cleanup := setupTestDB(t, "test_delete_request")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
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
		Metadata:         map[string]interface{}{},
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
	connStr, cleanup := setupTestDB(t, "test_delete_notfound")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
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

func TestGetTimelineExtents(t *testing.T) {
	t.Run("empty database", func(t *testing.T) {
		connStr, cleanup := setupTestDB(t, "test_timeline_extents_empty")
		defer cleanup()

		store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		earliestDate, err := store.GetTimelineExtents()
		if err != nil {
			t.Fatalf("Failed to get timeline extents: %v", err)
		}

		if earliestDate != nil {
			t.Errorf("Expected nil for empty database, got %v", earliestDate)
		}
	})

	t.Run("single document with publish_date", func(t *testing.T) {
		connStr, cleanup := setupTestDB(t, "test_timeline_extents_single")
		defer cleanup()

		store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		expectedDate := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
		sourceURL := "https://example.com/article"
		scraperUUID := "scraper-1"

		req := &Request{
			ID:               "test-1",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "url",
			SourceURL:        &sourceURL,
			ScraperUUID:      &scraperUUID,
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"test"},
			Metadata: map[string]interface{}{
				"scraper_metadata": map[string]interface{}{
					"publish_date": expectedDate.Format(time.RFC3339),
					"title":        "Test Article",
				},
			},
		}

		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request: %v", err)
		}

		earliestDate, err := store.GetTimelineExtents()
		if err != nil {
			t.Fatalf("Failed to get timeline extents: %v", err)
		}

		if earliestDate == nil {
			t.Fatal("Expected date, got nil")
		}

		// Compare dates with some tolerance for precision
		diff := earliestDate.Sub(expectedDate)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("Expected date %v, got %v (diff: %v)", expectedDate, earliestDate, diff)
		}
	})

	t.Run("multiple documents - earliest is in scraper_metadata", func(t *testing.T) {
		connStr, cleanup := setupTestDB(t, "test_timeline_extents_multiple")
		defer cleanup()

		store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		// Three documents with different dates
		dates := []time.Time{
			time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),  // Newest
			time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC), // Middle
			time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC), // Oldest (expected)
		}

		for i, date := range dates {
			sourceURL := fmt.Sprintf("https://example.com/article-%d", i)
			scraperUUID := fmt.Sprintf("scraper-%d", i)

			req := &Request{
				ID:               fmt.Sprintf("test-%d", i),
				CreatedAt:        time.Now().UTC(),
				SourceType:       "url",
				SourceURL:        &sourceURL,
				ScraperUUID:      &scraperUUID,
				TextAnalyzerUUID: fmt.Sprintf("analyzer-%d", i),
				Tags:             []string{"test"},
				Metadata: map[string]interface{}{
					"scraper_metadata": map[string]interface{}{
						"publish_date": date.Format(time.RFC3339),
					},
				},
			}

			if err := store.SaveRequest(req); err != nil {
				t.Fatalf("Failed to save request %d: %v", i, err)
			}
		}

		earliestDate, err := store.GetTimelineExtents()
		if err != nil {
			t.Fatalf("Failed to get timeline extents: %v", err)
		}

		if earliestDate == nil {
			t.Fatal("Expected date, got nil")
		}

		expectedDate := dates[2] // Oldest
		diff := earliestDate.Sub(expectedDate)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("Expected earliest date %v, got %v", expectedDate, earliestDate)
		}
	})

	t.Run("date precedence - scraper_metadata.publish_date takes priority", func(t *testing.T) {
		connStr, cleanup := setupTestDB(t, "test_timeline_extents_precedence")
		defer cleanup()

		store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		expectedDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		laterDate := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
		sourceURL := "https://example.com/article"
		scraperUUID := "scraper-1"

		req := &Request{
			ID:               "test-precedence",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "url",
			SourceURL:        &sourceURL,
			ScraperUUID:      &scraperUUID,
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"test"},
			Metadata: map[string]interface{}{
				"scraper_metadata": map[string]interface{}{
					"publish_date":   expectedDate.Format(time.RFC3339), // Should take priority
					"published_date": laterDate.Format(time.RFC3339),
				},
				"additional_metadata": map[string]interface{}{
					"publish_date": laterDate.Format(time.RFC3339),
					"date":         laterDate.Format(time.RFC3339),
				},
			},
		}

		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request: %v", err)
		}

		earliestDate, err := store.GetTimelineExtents()
		if err != nil {
			t.Fatalf("Failed to get timeline extents: %v", err)
		}

		if earliestDate == nil {
			t.Fatal("Expected date, got nil")
		}

		diff := earliestDate.Sub(expectedDate)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("Expected date %v (from scraper_metadata.publish_date), got %v", expectedDate, earliestDate)
		}
	})

	t.Run("date precedence - falls back to additional_metadata.date", func(t *testing.T) {
		connStr, cleanup := setupTestDB(t, "test_timeline_extents_fallback")
		defer cleanup()

		store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		expectedDate := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
		sourceURL := "https://example.com/article"
		scraperUUID := "scraper-1"

		req := &Request{
			ID:               "test-fallback",
			CreatedAt:        time.Now().UTC(),
			SourceType:       "url",
			SourceURL:        &sourceURL,
			ScraperUUID:      &scraperUUID,
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"test"},
			Metadata: map[string]interface{}{
				"additional_metadata": map[string]interface{}{
					"date": expectedDate.Format(time.RFC3339), // Only this field exists
				},
			},
		}

		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request: %v", err)
		}

		earliestDate, err := store.GetTimelineExtents()
		if err != nil {
			t.Fatalf("Failed to get timeline extents: %v", err)
		}

		if earliestDate == nil {
			t.Fatal("Expected date, got nil")
		}

		diff := earliestDate.Sub(expectedDate)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("Expected date %v (from additional_metadata.date), got %v", expectedDate, earliestDate)
		}
	})

	t.Run("date precedence - falls back to created_at", func(t *testing.T) {
		connStr, cleanup := setupTestDB(t, "test_timeline_extents_created_at")
		defer cleanup()

		store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		expectedDate := time.Date(2024, 2, 1, 10, 30, 0, 0, time.UTC)
		sourceURL := "https://example.com/article"
		scraperUUID := "scraper-1"

		req := &Request{
			ID:               "test-created-at",
			CreatedAt:        expectedDate,
			SourceType:       "url",
			SourceURL:        &sourceURL,
			ScraperUUID:      &scraperUUID,
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"test"},
			Metadata:         map[string]interface{}{}, // No date fields in metadata
		}

		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request: %v", err)
		}

		earliestDate, err := store.GetTimelineExtents()
		if err != nil {
			t.Fatalf("Failed to get timeline extents: %v", err)
		}

		if earliestDate == nil {
			t.Fatal("Expected date, got nil")
		}

		diff := earliestDate.Sub(expectedDate)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("Expected date %v (from created_at), got %v", expectedDate, earliestDate)
		}
	})

	t.Run("mixed metadata structures", func(t *testing.T) {
		connStr, cleanup := setupTestDB(t, "test_timeline_extents_mixed")
		defer cleanup()

		store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
		if err != nil {
			t.Fatalf("Failed to create storage: %v", err)
		}
		defer store.Close()

		// Create requests with different metadata structures
		// Document 1: Has scraper_metadata.publish_date (March 1)
		sourceURL1 := "https://example.com/article-1"
		scraperUUID1 := "scraper-1"
		req1 := &Request{
			ID:               "test-mixed-1",
			CreatedAt:        time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
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

		// Document 2: Only has additional_metadata.date (January 15 - should be earliest)
		sourceURL2 := "https://example.com/article-2"
		scraperUUID2 := "scraper-2"
		req2 := &Request{
			ID:               "test-mixed-2",
			CreatedAt:        time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			SourceType:       "url",
			SourceURL:        &sourceURL2,
			ScraperUUID:      &scraperUUID2,
			TextAnalyzerUUID: "analyzer-2",
			Tags:             []string{"test"},
			Metadata: map[string]interface{}{
				"additional_metadata": map[string]interface{}{
					"date": time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
				},
			},
		}

		// Document 3: No metadata dates, only created_at (April 1)
		sourceURL3 := "https://example.com/article-3"
		scraperUUID3 := "scraper-3"
		req3 := &Request{
			ID:               "test-mixed-3",
			CreatedAt:        time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC),
			SourceType:       "url",
			SourceURL:        &sourceURL3,
			ScraperUUID:      &scraperUUID3,
			TextAnalyzerUUID: "analyzer-3",
			Tags:             []string{"test"},
			Metadata:         map[string]interface{}{},
		}

		for _, req := range []*Request{req1, req2, req3} {
			if err := store.SaveRequest(req); err != nil {
				t.Fatalf("Failed to save request %s: %v", req.ID, err)
			}
		}

		earliestDate, err := store.GetTimelineExtents()
		if err != nil {
			t.Fatalf("Failed to get timeline extents: %v", err)
		}

		if earliestDate == nil {
			t.Fatal("Expected date, got nil")
		}

		// Should be January 15 from req2's additional_metadata.date
		expectedDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
		diff := earliestDate.Sub(expectedDate)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("Expected earliest date %v (from req2 additional_metadata.date), got %v", expectedDate, earliestDate)
		}
	})
}

func TestUpdateSEOEnabled(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_update_seo")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create a request with SEO disabled
	sourceURL := "https://example.com/test"
	scraperUUID := "scraper-seo-test"
	slug := "test-seo-article"
	req := &Request{
		ID:               "test-seo-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &sourceURL,
		ScraperUUID:      &scraperUUID,
		TextAnalyzerUUID: "analyzer-seo-1",
		Tags:             []string{"test"},
		Slug:             &slug,
		SEOEnabled:       false,
		Metadata:         map[string]interface{}{},
	}

	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Verify SEO is disabled
	retrieved, err := store.GetRequest("test-seo-1")
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}
	if retrieved.SEOEnabled {
		t.Error("Expected SEOEnabled to be false")
	}

	// Enable SEO
	if err := store.UpdateSEOEnabled("test-seo-1", true); err != nil {
		t.Fatalf("Failed to update SEO enabled: %v", err)
	}

	// Verify SEO is now enabled
	retrieved, err = store.GetRequest("test-seo-1")
	if err != nil {
		t.Fatalf("Failed to get request after update: %v", err)
	}
	if !retrieved.SEOEnabled {
		t.Error("Expected SEOEnabled to be true after update")
	}

	// Disable SEO again
	if err := store.UpdateSEOEnabled("test-seo-1", false); err != nil {
		t.Fatalf("Failed to disable SEO: %v", err)
	}

	// Verify SEO is disabled
	retrieved, err = store.GetRequest("test-seo-1")
	if err != nil {
		t.Fatalf("Failed to get request after second update: %v", err)
	}
	if retrieved.SEOEnabled {
		t.Error("Expected SEOEnabled to be false after disabling")
	}
}

func TestUpdateSEOEnabledNotFound(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_update_seo_notfound")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	err = store.UpdateSEOEnabled("non-existent-id", true)
	if err == nil {
		t.Error("Expected error for non-existent request")
	}
	if err.Error() != "request not found" {
		t.Errorf("Expected 'request not found' error, got: %v", err)
	}
}

func TestGetRequestBySlug(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_get_by_slug")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create a request with a slug
	sourceURL := "https://example.com/article"
	scraperUUID := "scraper-slug-test"
	slug := "my-awesome-article"
	req := &Request{
		ID:               "test-slug-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &sourceURL,
		ScraperUUID:      &scraperUUID,
		TextAnalyzerUUID: "analyzer-slug-1",
		Tags:             []string{"test", "slug"},
		Slug:             &slug,
		SEOEnabled:       true,
		Metadata: map[string]interface{}{
			"scraper_metadata": map[string]interface{}{
				"title": "My Awesome Article",
			},
		},
	}

	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Retrieve by slug
	retrieved, err := store.GetRequestBySlug("my-awesome-article")
	if err != nil {
		t.Fatalf("Failed to get request by slug: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Expected request, got nil")
	}

	if retrieved.ID != req.ID {
		t.Errorf("Expected ID %s, got %s", req.ID, retrieved.ID)
	}
	if retrieved.Slug == nil || *retrieved.Slug != slug {
		t.Errorf("Expected slug %s, got %v", slug, retrieved.Slug)
	}
	if !retrieved.SEOEnabled {
		t.Error("Expected SEOEnabled to be true")
	}

	// Try to retrieve non-existent slug
	nonExistent, err := store.GetRequestBySlug("non-existent-slug")
	if err != nil {
		t.Fatalf("Expected no error for non-existent slug, got: %v", err)
	}
	if nonExistent != nil {
		t.Error("Expected nil for non-existent slug")
	}
}

func TestSlugUniqueness(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_slug_uniqueness")
	defer cleanup()

	store, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	// Create first request with a slug
	sourceURL1 := "https://example.com/article-1"
	scraperUUID1 := "scraper-1"
	slug := "duplicate-slug"
	req1 := &Request{
		ID:               "test-dup-1",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &sourceURL1,
		ScraperUUID:      &scraperUUID1,
		TextAnalyzerUUID: "analyzer-1",
		Tags:             []string{"test"},
		Slug:             &slug,
		SEOEnabled:       true,
		Metadata:         map[string]interface{}{},
	}

	if err := store.SaveRequest(req1); err != nil {
		t.Fatalf("Failed to save first request: %v", err)
	}

	// Try to create second request with same slug
	sourceURL2 := "https://example.com/article-2"
	scraperUUID2 := "scraper-2"
	req2 := &Request{
		ID:               "test-dup-2",
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &sourceURL2,
		ScraperUUID:      &scraperUUID2,
		TextAnalyzerUUID: "analyzer-2",
		Tags:             []string{"test"},
		Slug:             &slug, // Same slug
		SEOEnabled:       true,
		Metadata:         map[string]interface{}{},
	}

	// This should fail due to unique constraint on slug
	err = store.SaveRequest(req2)
	if err == nil {
		t.Error("Expected error when saving duplicate slug, but got none")
	}
}

// TestGetTagTimeline_EmptyDatabase verifies behavior with no documents
func TestGetTagTimeline_EmptyDatabase(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_tag_timeline_empty")
	defer cleanup()

	store, err := New(connStr, []string{}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	startDate := time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2025, 10, 2, 0, 0, 0, 0, time.UTC)
	bucketDuration := 6 * time.Hour

	timeline, err := store.GetTagTimeline(startDate, endDate, bucketDuration, 20)
	if err != nil {
		t.Fatalf("GetTagTimeline failed: %v", err)
	}

	if timeline.Stats.TotalDocuments != 0 {
		t.Errorf("Expected 0 documents, got %d", timeline.Stats.TotalDocuments)
	}
	if timeline.Stats.TotalUniqueTags != 0 {
		t.Errorf("Expected 0 unique tags, got %d", timeline.Stats.TotalUniqueTags)
	}
	if len(timeline.Buckets) != 4 { // 24 hours / 6 hours = 4 buckets
		t.Errorf("Expected 4 buckets, got %d", len(timeline.Buckets))
	}

	// All buckets should be empty
	for i, bucket := range timeline.Buckets {
		if len(bucket.Tags) != 0 {
			t.Errorf("Bucket %d: expected 0 tags, got %d", i, len(bucket.Tags))
		}
	}
}

// TestGetTagTimeline_SingleBucket verifies tag frequency calculation in a single bucket
func TestGetTagTimeline_SingleBucket(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_tag_timeline_single")
	defer cleanup()

	store, err := New(connStr, []string{}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	baseTime := time.Date(2025, 10, 30, 12, 0, 0, 0, time.UTC)

	// Create documents with different tags
	requests := []*Request{
		{
			ID:               "doc-1",
			CreatedAt:        baseTime,
			EffectiveDate:    baseTime,
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"politics", "news"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		{
			ID:               "doc-2",
			CreatedAt:        baseTime.Add(10 * time.Minute),
			EffectiveDate:    baseTime.Add(10 * time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-2",
			Tags:             []string{"politics", "economy"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		{
			ID:               "doc-3",
			CreatedAt:        baseTime.Add(20 * time.Minute),
			EffectiveDate:    baseTime.Add(20 * time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-3",
			Tags:             []string{"politics", "tech"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
	}

	for _, req := range requests {
		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request: %v", err)
		}
	}

	// Query with a 1-hour bucket covering all documents
	startDate := baseTime.Add(-10 * time.Minute)
	endDate := baseTime.Add(50 * time.Minute)
	bucketDuration := 1 * time.Hour

	timeline, err := store.GetTagTimeline(startDate, endDate, bucketDuration, 20)
	if err != nil {
		t.Fatalf("GetTagTimeline failed: %v", err)
	}

	if timeline.Stats.TotalDocuments != 3 {
		t.Errorf("Expected 3 documents, got %d", timeline.Stats.TotalDocuments)
	}
	if timeline.Stats.TotalUniqueTags != 4 { // politics, news, economy, tech
		t.Errorf("Expected 4 unique tags, got %d", timeline.Stats.TotalUniqueTags)
	}

	if len(timeline.Buckets) != 1 {
		t.Fatalf("Expected 1 bucket, got %d", len(timeline.Buckets))
	}

	bucket := timeline.Buckets[0]
	if len(bucket.Tags) != 4 {
		t.Fatalf("Expected 4 tags in bucket, got %d", len(bucket.Tags))
	}

	// Verify politics is the most popular (appears in 3 docs)
	politicsTag := bucket.Tags[0]
	if politicsTag.Tag != "politics" {
		t.Errorf("Expected first tag to be 'politics', got '%s'", politicsTag.Tag)
	}
	if politicsTag.Count != 3 {
		t.Errorf("Expected politics to have count 3, got %d", politicsTag.Count)
	}
	if politicsTag.PopularityScore != 1.0 {
		t.Errorf("Expected politics to have popularity 1.0, got %f", politicsTag.PopularityScore)
	}

	// Other tags should have count 1 and popularity score 1/3
	for i := 1; i < len(bucket.Tags); i++ {
		tag := bucket.Tags[i]
		if tag.Count != 1 {
			t.Errorf("Expected tag '%s' to have count 1, got %d", tag.Tag, tag.Count)
		}
		expectedPopularity := 1.0 / 3.0
		if abs(tag.PopularityScore-expectedPopularity) > 0.01 {
			t.Errorf("Expected tag '%s' to have popularity ~%.3f, got %.3f", tag.Tag, expectedPopularity, tag.PopularityScore)
		}
	}
}

// TestGetTagTimeline_MultipleBuckets verifies distribution across time buckets
func TestGetTagTimeline_MultipleBuckets(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_tag_timeline_multiple")
	defer cleanup()

	store, err := New(connStr, []string{}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	baseTime := time.Date(2025, 10, 30, 0, 0, 0, 0, time.UTC)

	// Create documents across 3 hours (3 buckets with 1-hour buckets)
	requests := []*Request{
		// Hour 1: 2 docs with "morning" tag
		{
			ID:               "doc-1",
			CreatedAt:        baseTime.Add(10 * time.Minute),
			EffectiveDate:    baseTime.Add(10 * time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"morning", "news"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		{
			ID:               "doc-2",
			CreatedAt:        baseTime.Add(30 * time.Minute),
			EffectiveDate:    baseTime.Add(30 * time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-2",
			Tags:             []string{"morning", "weather"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		// Hour 2: 1 doc with "afternoon" tag
		{
			ID:               "doc-3",
			CreatedAt:        baseTime.Add(1*time.Hour + 15*time.Minute),
			EffectiveDate:    baseTime.Add(1*time.Hour + 15*time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-3",
			Tags:             []string{"afternoon", "news"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		// Hour 3: 3 docs with "evening" tag
		{
			ID:               "doc-4",
			CreatedAt:        baseTime.Add(2*time.Hour + 5*time.Minute),
			EffectiveDate:    baseTime.Add(2*time.Hour + 5*time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-4",
			Tags:             []string{"evening", "politics"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		{
			ID:               "doc-5",
			CreatedAt:        baseTime.Add(2*time.Hour + 25*time.Minute),
			EffectiveDate:    baseTime.Add(2*time.Hour + 25*time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-5",
			Tags:             []string{"evening", "sports"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		{
			ID:               "doc-6",
			CreatedAt:        baseTime.Add(2*time.Hour + 45*time.Minute),
			EffectiveDate:    baseTime.Add(2*time.Hour + 45*time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-6",
			Tags:             []string{"evening", "tech"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
	}

	for _, req := range requests {
		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request: %v", err)
		}
	}

	// Query with 1-hour buckets
	startDate := baseTime
	endDate := baseTime.Add(3 * time.Hour)
	bucketDuration := 1 * time.Hour

	timeline, err := store.GetTagTimeline(startDate, endDate, bucketDuration, 20)
	if err != nil {
		t.Fatalf("GetTagTimeline failed: %v", err)
	}

	if timeline.Stats.TotalDocuments != 6 {
		t.Errorf("Expected 6 documents, got %d", timeline.Stats.TotalDocuments)
	}

	if len(timeline.Buckets) != 3 {
		t.Fatalf("Expected 3 buckets, got %d", len(timeline.Buckets))
	}

	// Verify bucket 1 (morning): 2 docs, "morning" most popular
	bucket1 := timeline.Buckets[0]
	if len(bucket1.Tags) < 1 {
		t.Fatal("Expected at least 1 tag in bucket 1")
	}
	if bucket1.Tags[0].Tag != "morning" {
		t.Errorf("Bucket 1: expected 'morning' tag first, got '%s'", bucket1.Tags[0].Tag)
	}
	if bucket1.Tags[0].Count != 2 {
		t.Errorf("Bucket 1: expected 'morning' count 2, got %d", bucket1.Tags[0].Count)
	}

	// Verify bucket 2 (afternoon): 1 doc
	bucket2 := timeline.Buckets[1]
	if len(bucket2.Tags) < 1 {
		t.Fatal("Expected at least 1 tag in bucket 2")
	}

	// Verify bucket 3 (evening): 3 docs, "evening" most popular
	bucket3 := timeline.Buckets[2]
	if len(bucket3.Tags) < 1 {
		t.Fatal("Expected at least 1 tag in bucket 3")
	}
	if bucket3.Tags[0].Tag != "evening" {
		t.Errorf("Bucket 3: expected 'evening' tag first, got '%s'", bucket3.Tags[0].Tag)
	}
	if bucket3.Tags[0].Count != 3 {
		t.Errorf("Bucket 3: expected 'evening' count 3, got %d", bucket3.Tags[0].Count)
	}
}

// TestGetTagTimeline_MaxTagsPerBucket verifies max_tags limiting
func TestGetTagTimeline_MaxTagsPerBucket(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_tag_timeline_max_tags")
	defer cleanup()

	store, err := New(connStr, []string{}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	baseTime := time.Date(2025, 10, 30, 12, 0, 0, 0, time.UTC)

	// Create a document with 10 different tags
	req := &Request{
		ID:               "doc-many-tags",
		CreatedAt:        baseTime,
		EffectiveDate:    baseTime,
		SourceType:       "url",
		TextAnalyzerUUID: "analyzer-1",
		Tags:             []string{"tag1", "tag2", "tag3", "tag4", "tag5", "tag6", "tag7", "tag8", "tag9", "tag10"},
		SEOEnabled:       true,
		Metadata:         map[string]interface{}{},
	}

	if err := store.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Query with max_tags = 5
	startDate := baseTime.Add(-10 * time.Minute)
	endDate := baseTime.Add(10 * time.Minute)
	bucketDuration := 1 * time.Hour
	maxTags := 5

	timeline, err := store.GetTagTimeline(startDate, endDate, bucketDuration, maxTags)
	if err != nil {
		t.Fatalf("GetTagTimeline failed: %v", err)
	}

	if len(timeline.Buckets) != 1 {
		t.Fatalf("Expected 1 bucket, got %d", len(timeline.Buckets))
	}

	bucket := timeline.Buckets[0]
	if len(bucket.Tags) > maxTags {
		t.Errorf("Expected at most %d tags, got %d", maxTags, len(bucket.Tags))
	}
}

// TestGetTagTimeline_ExcludesTombstonedAndSEODisabled verifies filtering
func TestGetTagTimeline_ExcludesTombstonedAndSEODisabled(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "test_tag_timeline_filtering")
	defer cleanup()

	store, err := New(connStr, []string{}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer store.Close()

	baseTime := time.Date(2025, 10, 30, 12, 0, 0, 0, time.UTC)
	futureTime := baseTime.Add(48 * time.Hour)

	requests := []*Request{
		// Valid document
		{
			ID:               "doc-valid",
			CreatedAt:        baseTime,
			EffectiveDate:    baseTime,
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-1",
			Tags:             []string{"valid"},
			SEOEnabled:       true,
			Metadata:         map[string]interface{}{},
		},
		// SEO disabled
		{
			ID:               "doc-seo-disabled",
			CreatedAt:        baseTime.Add(5 * time.Minute),
			EffectiveDate:    baseTime.Add(5 * time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-2",
			Tags:             []string{"seo-disabled"},
			SEOEnabled:       false,
			Metadata:         map[string]interface{}{},
		},
		// Tombstoned (tombstone in the past)
		{
			ID:               "doc-tombstoned",
			CreatedAt:        baseTime.Add(10 * time.Minute),
			EffectiveDate:    baseTime.Add(10 * time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-3",
			Tags:             []string{"tombstoned"},
			SEOEnabled:       true,
			Metadata: map[string]interface{}{
				"tombstone_datetime": baseTime.Add(-1 * time.Hour).Format(time.RFC3339),
			},
		},
		// Not yet tombstoned (tombstone in future)
		{
			ID:               "doc-future-tombstone",
			CreatedAt:        baseTime.Add(15 * time.Minute),
			EffectiveDate:    baseTime.Add(15 * time.Minute),
			SourceType:       "url",
			TextAnalyzerUUID: "analyzer-4",
			Tags:             []string{"future-tombstone"},
			SEOEnabled:       true,
			Metadata: map[string]interface{}{
				"tombstone_datetime": futureTime.Format(time.RFC3339),
			},
		},
	}

	for _, req := range requests {
		if err := store.SaveRequest(req); err != nil {
			t.Fatalf("Failed to save request: %v", err)
		}
	}

	// Query timeline
	startDate := baseTime.Add(-10 * time.Minute)
	endDate := baseTime.Add(30 * time.Minute)
	bucketDuration := 1 * time.Hour

	timeline, err := store.GetTagTimeline(startDate, endDate, bucketDuration, 20)
	if err != nil {
		t.Fatalf("GetTagTimeline failed: %v", err)
	}

	// Should only include "valid" and "future-tombstone" documents
	if timeline.Stats.TotalDocuments != 2 {
		t.Errorf("Expected 2 documents (valid + future-tombstone), got %d", timeline.Stats.TotalDocuments)
	}

	// Check that excluded tags don't appear
	if len(timeline.Buckets) != 1 {
		t.Fatalf("Expected 1 bucket, got %d", len(timeline.Buckets))
	}

	bucket := timeline.Buckets[0]
	for _, tagEntry := range bucket.Tags {
		if tagEntry.Tag == "seo-disabled" {
			t.Error("SEO-disabled document should not appear in timeline")
		}
		if tagEntry.Tag == "tombstoned" {
			t.Error("Tombstoned document should not appear in timeline")
		}
	}
}

// Helper function for floating point comparison
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
