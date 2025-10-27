package storage

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

/**
 * Regression Test Suite: Auto-Tombstone Functionality
 *
 * Purpose: Verify dual-strategy auto-tombstone system
 *
 * Strategy 1: Low-score tombstone (30 days)
 * - Documents with score < threshold get 30-day tombstone
 * - Changed from 3 days to 30 days
 *
 * Strategy 2: Tag-based tombstone (90 days)
 * - Documents with 'low-quality' OR 'sparse-content' tags get 90-day tombstone
 * - Triggers when UpdateRequestTags is called
 *
 * These tests verify the tombstone logic works correctly after implementation.
 */

func TestUpdateRequestTags_AutoTombstone_LowQuality(t *testing.T) {
	dbPath := "test_tombstone_lowquality.db"
	defer os.Remove(dbPath)

	storage, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create a test request without tombstone
	req := &Request{
		ID:         "test-req-1",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{"initial-tag"},
		Metadata: map[string]interface{}{
			"some_field": "some_value",
		},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update tags to include 'low-quality'
	newTags := []string{"initial-tag", "low-quality", "other-tag"}
	if err := storage.UpdateRequestTags(req.ID, newTags); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Retrieve the request and verify tombstone was added
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get updated request: %v", err)
	}

	// Verify tags were updated
	if len(updated.Tags) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(updated.Tags))
	}

	// Verify tombstone_datetime was added
	tombstoneDatetimeRaw, ok := updated.Metadata["tombstone_datetime"]
	if !ok {
		t.Fatal("Expected tombstone_datetime in metadata, but it was not found")
	}

	tombstoneDatetimeStr, ok := tombstoneDatetimeRaw.(string)
	if !ok {
		t.Fatalf("Expected tombstone_datetime to be string, got %T", tombstoneDatetimeRaw)
	}

	tombstoneTime, err := time.Parse(time.RFC3339, tombstoneDatetimeStr)
	if err != nil {
		t.Fatalf("Failed to parse tombstone_datetime: %v", err)
	}

	// Verify tombstone is ~90 days in the future (with 1 day tolerance)
	expectedTime := time.Now().Add(90 * 24 * time.Hour)
	diff := tombstoneTime.Sub(expectedTime)
	if diff < -24*time.Hour || diff > 24*time.Hour {
		t.Errorf("Expected tombstone ~90 days in future, got %v (diff: %v)", tombstoneTime, diff)
	}

	// Verify tombstone_reason was set
	reasonRaw, ok := updated.Metadata["tombstone_reason"]
	if !ok {
		t.Error("Expected tombstone_reason in metadata")
	} else {
		reason, ok := reasonRaw.(string)
		if !ok {
			t.Errorf("Expected tombstone_reason to be string, got %T", reasonRaw)
		} else if reason != "auto-tombstone: low-quality or sparse-content tag" {
			t.Errorf("Expected specific tombstone_reason, got: %s", reason)
		}
	}
}

func TestUpdateRequestTags_AutoTombstone_SparseContent(t *testing.T) {
	dbPath := "test_tombstone_sparse.db"
	defer os.Remove(dbPath)

	storage, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create a test request
	req := &Request{
		ID:         "test-req-2",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{"some-tag"},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update tags to include 'sparse-content'
	newTags := []string{"sparse-content"}
	if err := storage.UpdateRequestTags(req.ID, newTags); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Retrieve and verify tombstone was added
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get updated request: %v", err)
	}

	tombstoneDatetimeRaw, ok := updated.Metadata["tombstone_datetime"]
	if !ok {
		t.Fatal("Expected tombstone_datetime in metadata for sparse-content tag")
	}

	tombstoneDatetimeStr := tombstoneDatetimeRaw.(string)
	tombstoneTime, err := time.Parse(time.RFC3339, tombstoneDatetimeStr)
	if err != nil {
		t.Fatalf("Failed to parse tombstone_datetime: %v", err)
	}

	// Verify it's ~90 days in the future
	expectedTime := time.Now().Add(90 * 24 * time.Hour)
	diff := tombstoneTime.Sub(expectedTime)
	if diff < -24*time.Hour || diff > 24*time.Hour {
		t.Errorf("Expected tombstone ~90 days in future, got %v (diff: %v)", tombstoneTime, diff)
	}
}

func TestUpdateRequestTags_AutoTombstone_BothTags(t *testing.T) {
	dbPath := "test_tombstone_both.db"
	defer os.Remove(dbPath)

	storage, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create a test request
	req := &Request{
		ID:         "test-req-3",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update tags to include both 'low-quality' AND 'sparse-content'
	newTags := []string{"low-quality", "sparse-content", "other"}
	if err := storage.UpdateRequestTags(req.ID, newTags); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Retrieve and verify tombstone was added (only once, not twice)
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get updated request: %v", err)
	}

	_, ok := updated.Metadata["tombstone_datetime"]
	if !ok {
		t.Fatal("Expected tombstone_datetime in metadata")
	}

	// Verify metadata is valid JSON (no duplication issues)
	metadataJSON, err := json.Marshal(updated.Metadata)
	if err != nil {
		t.Fatalf("Metadata is not valid JSON: %v", err)
	}

	var testUnmarshal map[string]interface{}
	if err := json.Unmarshal(metadataJSON, &testUnmarshal); err != nil {
		t.Fatalf("Failed to unmarshal metadata: %v", err)
	}
}

func TestUpdateRequestTags_NoAutoTombstone_NormalTags(t *testing.T) {
	dbPath := "test_tombstone_normal.db"
	defer os.Remove(dbPath)

	storage, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create a test request
	req := &Request{
		ID:         "test-req-4",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update tags with normal tags (no 'low-quality' or 'sparse-content')
	newTags := []string{"javascript", "react", "tutorial"}
	if err := storage.UpdateRequestTags(req.ID, newTags); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Retrieve and verify NO tombstone was added
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get updated request: %v", err)
	}

	_, hasTombstone := updated.Metadata["tombstone_datetime"]
	if hasTombstone {
		t.Error("Did not expect tombstone_datetime for normal tags")
	}
}

func TestUpdateRequestTags_PreservesExistingMetadata(t *testing.T) {
	dbPath := "test_tombstone_preserve.db"
	defer os.Remove(dbPath)

	storage, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create a test request with existing metadata
	req := &Request{
		ID:         "test-req-5",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata: map[string]interface{}{
			"existing_field":  "important_value",
			"another_field":   123,
			"nested_field":    map[string]interface{}{"key": "value"},
		},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update tags to trigger tombstone
	newTags := []string{"low-quality"}
	if err := storage.UpdateRequestTags(req.ID, newTags); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Retrieve and verify existing metadata is preserved
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get updated request: %v", err)
	}

	// Check tombstone was added
	_, hasTombstone := updated.Metadata["tombstone_datetime"]
	if !hasTombstone {
		t.Fatal("Expected tombstone_datetime to be added")
	}

	// Check existing metadata is preserved
	if updated.Metadata["existing_field"] != "important_value" {
		t.Error("Existing metadata field was not preserved")
	}
	if updated.Metadata["another_field"] != float64(123) { // JSON unmarshals numbers as float64
		t.Error("Existing numeric field was not preserved")
	}

	nestedField, ok := updated.Metadata["nested_field"].(map[string]interface{})
	if !ok {
		t.Error("Nested field was not preserved as map")
	} else if nestedField["key"] != "value" {
		t.Error("Nested field value was not preserved")
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
