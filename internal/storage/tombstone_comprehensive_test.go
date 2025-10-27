package storage

import (
	"testing"
	"time"
)

/**
 * Comprehensive Tombstone Test Suite
 *
 * This test suite validates the entire tombstone workflow with configurable settings.
 * It tests all rejection and tombstoning points in the system.
 */

// TestTombstoneConfiguration_CustomTags tests tombstoning with custom tags
func TestTombstoneConfiguration_CustomTags(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "tombstone_custom_tags")
	defer cleanup()

	// Create storage with custom tombstone tags
	customTags := []string{"spam", "malicious"}
	storage, err := New(connStr, customTags, 15, 60, 45)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Create a test request
	req := &Request{
		ID:         "test-custom-1",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{"normal-tag"},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update tags to include custom tombstone tag
	newTags := []string{"spam", "normal-tag"}
	if err := storage.UpdateRequestTags(req.ID, newTags); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Retrieve and verify tombstone was added
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get updated request: %v", err)
	}

	// Verify tombstone_datetime was added
	tombstoneDatetimeRaw, ok := updated.Metadata["tombstone_datetime"]
	if !ok {
		t.Fatal("Expected tombstone_datetime for custom tag 'spam'")
	}

	tombstoneDatetimeStr := tombstoneDatetimeRaw.(string)
	tombstoneTime, err := time.Parse(time.RFC3339, tombstoneDatetimeStr)
	if err != nil {
		t.Fatalf("Failed to parse tombstone_datetime: %v", err)
	}

	// Verify it's ~60 days in the future (custom period)
	expectedTime := time.Now().Add(60 * 24 * time.Hour)
	diff := tombstoneTime.Sub(expectedTime)
	if diff < -24*time.Hour || diff > 24*time.Hour {
		t.Errorf("Expected tombstone ~60 days in future (custom period), got %v (diff: %v)", tombstoneTime, diff)
	}

	// Verify tombstone_reason mentions the custom tag
	reasonRaw, ok := updated.Metadata["tombstone_reason"]
	if !ok {
		t.Error("Expected tombstone_reason in metadata")
	} else {
		reason := reasonRaw.(string)
		if reason != "auto-tombstone: spam tag" {
			t.Errorf("Expected reason to mention 'spam', got: %s", reason)
		}
	}
}

// TestTombstoneConfiguration_CustomPeriods tests different tombstone periods
func TestTombstoneConfiguration_CustomPeriods(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "tombstone_custom_periods")
	defer cleanup()

	// Create storage with custom periods: 7 days (low-score), 14 days (tag-based), 21 days (manual)
	storage, err := New(connStr, []string{"low-quality"}, 7, 14, 21)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Test tag-based tombstone period (14 days)
	req := &Request{
		ID:         "test-period-1",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Trigger tag-based tombstone
	if err := storage.UpdateRequestTags(req.ID, []string{"low-quality"}); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Verify 14-day period
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	tombstoneDatetimeStr := updated.Metadata["tombstone_datetime"].(string)
	tombstoneTime, _ := time.Parse(time.RFC3339, tombstoneDatetimeStr)

	expectedTime := time.Now().Add(14 * 24 * time.Hour)
	diff := tombstoneTime.Sub(expectedTime)
	if diff < -24*time.Hour || diff > 24*time.Hour {
		t.Errorf("Expected ~14 days for tag-based tombstone, got diff: %v", diff)
	}
}

// TestTombstoneConfiguration_MultipleTags tests behavior with multiple tombstone tags
func TestTombstoneConfiguration_MultipleTags(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "tombstone_multiple")
	defer cleanup()

	storage, err := New(connStr, []string{"low-quality", "spam", "malicious"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	req := &Request{
		ID:         "test-multi-1",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update with one of the tombstone tags
	if err := storage.UpdateRequestTags(req.ID, []string{"spam", "other-tag"}); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Verify tombstone was added
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	if _, ok := updated.Metadata["tombstone_datetime"]; !ok {
		t.Error("Expected tombstone_datetime for 'spam' tag")
	}

	// Verify reason mentions the matched tag
	reason := updated.Metadata["tombstone_reason"].(string)
	if reason != "auto-tombstone: spam tag" {
		t.Errorf("Expected reason to mention 'spam', got: %s", reason)
	}
}

// TestTombstoneConfiguration_NoTombstoneForNormalTags ensures normal tags don't trigger tombstones
func TestTombstoneConfiguration_NoTombstoneForNormalTags(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "tombstone_no_trigger")
	defer cleanup()

	storage, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	req := &Request{
		ID:         "test-normal-1",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Update with normal tags (not in tombstone list)
	normalTags := []string{"technology", "programming", "web-development"}
	if err := storage.UpdateRequestTags(req.ID, normalTags); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	// Verify NO tombstone was added
	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	if _, ok := updated.Metadata["tombstone_datetime"]; ok {
		t.Error("Did not expect tombstone_datetime for normal tags")
	}
}

// TestTombstoneConfiguration_EdgeCases tests edge cases
func TestTombstoneConfiguration_EdgeCases(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "tombstone_edge")
	defer cleanup()

	// Test with single-day period
	storage, err := New(connStr, []string{"test-tag"}, 1, 1, 1)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	req := &Request{
		ID:         "test-edge-1",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Trigger tombstone with 1-day period
	if err := storage.UpdateRequestTags(req.ID, []string{"test-tag"}); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	// Verify 1-day tombstone
	tombstoneDatetimeStr := updated.Metadata["tombstone_datetime"].(string)
	tombstoneTime, _ := time.Parse(time.RFC3339, tombstoneDatetimeStr)

	expectedTime := time.Now().Add(24 * time.Hour)
	diff := tombstoneTime.Sub(expectedTime)
	if diff < -1*time.Hour || diff > 1*time.Hour {
		t.Errorf("Expected ~1 day tombstone period, got diff: %v", diff)
	}
}

// TestTombstoneConfiguration_DefaultValues tests default configuration values
func TestTombstoneConfiguration_DefaultValues(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "tombstone_defaults")
	defer cleanup()

	// Create storage with default values (30, 90, 90 days)
	storage, err := New(connStr, []string{"low-quality", "sparse-content"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	req := &Request{
		ID:         "test-default-1",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Trigger default tag-based tombstone (90 days)
	if err := storage.UpdateRequestTags(req.ID, []string{"low-quality"}); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	// Verify default 90-day period
	tombstoneDatetimeStr := updated.Metadata["tombstone_datetime"].(string)
	tombstoneTime, _ := time.Parse(time.RFC3339, tombstoneDatetimeStr)

	expectedTime := time.Now().Add(90 * 24 * time.Hour)
	diff := tombstoneTime.Sub(expectedTime)
	if diff < -24*time.Hour || diff > 24*time.Hour {
		t.Errorf("Expected default ~90 days, got diff: %v", diff)
	}
}

// TestTombstoneConfiguration_CaseSensitivity tests tag matching is case-sensitive
func TestTombstoneConfiguration_CaseSensitivity(t *testing.T) {
	connStr, cleanup := setupTestDB(t, "tombstone_case")
	defer cleanup()

	storage, err := New(connStr, []string{"low-quality"}, 30, 90, 90)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	req := &Request{
		ID:         "test-case-1",
		CreatedAt:  time.Now(),
		SourceType: "url",
		SourceURL:  stringPtr("https://example.com"),
		Tags:       []string{},
		Metadata:   map[string]interface{}{},
	}

	if err := storage.SaveRequest(req); err != nil {
		t.Fatalf("Failed to save request: %v", err)
	}

	// Try with different case - should NOT trigger tombstone (case-sensitive)
	if err := storage.UpdateRequestTags(req.ID, []string{"Low-Quality"}); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	updated, err := storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	// Verify NO tombstone was added (case mismatch)
	if _, ok := updated.Metadata["tombstone_datetime"]; ok {
		t.Error("Did not expect tombstone for case-mismatched tag")
	}

	// Now try with exact case match - SHOULD trigger tombstone
	if err := storage.UpdateRequestTags(req.ID, []string{"low-quality"}); err != nil {
		t.Fatalf("Failed to update tags: %v", err)
	}

	updated, err = storage.GetRequest(req.ID)
	if err != nil {
		t.Fatalf("Failed to get request: %v", err)
	}

	// Verify tombstone WAS added (exact match)
	if _, ok := updated.Metadata["tombstone_datetime"]; !ok {
		t.Error("Expected tombstone for exact case match")
	}
}
