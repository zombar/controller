package scraper_requests

import (
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	// Verify initial state
	requests := manager.List()
	if len(requests) != 0 {
		t.Errorf("Expected empty request list, got %d requests", len(requests))
	}
}

func TestCreate(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	req, isNew := manager.Create(url)

	if !isNew {
		t.Error("Expected isNew to be true for first request")
	}

	if req.ID == "" {
		t.Error("Expected non-empty ID")
	}

	if req.URL != url {
		t.Errorf("Expected URL '%s', got '%s'", url, req.URL)
	}

	if req.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, req.Status)
	}

	if req.Progress != 0 {
		t.Errorf("Expected progress 0, got %d", req.Progress)
	}

	if req.ExpiresAt.Before(time.Now()) {
		t.Error("Expected expiration in the future")
	}

	expectedExpiration := time.Now().Add(15 * time.Minute)
	if req.ExpiresAt.Before(expectedExpiration.Add(-1*time.Second)) ||
		req.ExpiresAt.After(expectedExpiration.Add(1*time.Second)) {
		t.Errorf("Expected expiration around %v, got %v", expectedExpiration, req.ExpiresAt)
	}
}

func TestCreateDuplicate(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	req1, isNew1 := manager.Create(url)
	if !isNew1 {
		t.Error("Expected first request to be new")
	}

	req2, isNew2 := manager.Create(url)
	if isNew2 {
		t.Error("Expected duplicate request to not be new")
	}

	if req1.ID != req2.ID {
		t.Errorf("Expected same ID for duplicate URL: %s != %s", req1.ID, req2.ID)
	}
}

func TestGet(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	created, _ := manager.Create(url)

	retrieved, exists := manager.Get(created.ID)
	if !exists {
		t.Fatal("Expected request to exist")
	}

	if retrieved.ID != created.ID {
		t.Errorf("Expected ID '%s', got '%s'", created.ID, retrieved.ID)
	}

	if retrieved.URL != url {
		t.Errorf("Expected URL '%s', got '%s'", url, retrieved.URL)
	}
}

func TestGetNonExistent(t *testing.T) {
	manager := NewManager()

	_, exists := manager.Get("non-existent-id")
	if exists {
		t.Error("Expected non-existent request to return false")
	}
}

func TestUpdateStatus(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	req, _ := manager.Create(url)
	originalUpdatedAt := req.UpdatedAt
	time.Sleep(10 * time.Millisecond) // Small delay to ensure time difference

	manager.UpdateStatus(req.ID, StatusProcessing, 50)

	updated, exists := manager.Get(req.ID)
	if !exists {
		t.Fatal("Expected request to exist")
	}

	if updated.Status != StatusProcessing {
		t.Errorf("Expected status %s, got %s", StatusProcessing, updated.Status)
	}

	if updated.Progress != 50 {
		t.Errorf("Expected progress 50, got %d", updated.Progress)
	}

	if updated.UpdatedAt.Before(originalUpdatedAt) || updated.UpdatedAt.Equal(originalUpdatedAt) {
		t.Error("Expected UpdatedAt to be updated")
	}
}

func TestSetCompleted(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	req, _ := manager.Create(url)
	resultID := "result-test-uuid"

	manager.SetCompleted(req.ID, resultID)

	updated, exists := manager.Get(req.ID)
	if !exists {
		t.Fatal("Expected request to exist")
	}

	if updated.Status != StatusCompleted {
		t.Errorf("Expected status %s, got %s", StatusCompleted, updated.Status)
	}

	if updated.Progress != 100 {
		t.Errorf("Expected progress 100, got %d", updated.Progress)
	}

	if updated.ResultRequestID != resultID {
		t.Errorf("Expected ResultRequestID '%s', got '%s'", resultID, updated.ResultRequestID)
	}
}

func TestSetFailed(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	req, _ := manager.Create(url)
	errorMsg := "Test error message"

	manager.SetFailed(req.ID, errorMsg)

	updated, exists := manager.Get(req.ID)
	if !exists {
		t.Fatal("Expected request to exist")
	}

	if updated.Status != StatusFailed {
		t.Errorf("Expected status %s, got %s", StatusFailed, updated.Status)
	}

	if updated.ErrorMessage != errorMsg {
		t.Errorf("Expected error message '%s', got '%s'", errorMsg, updated.ErrorMessage)
	}
}

func TestRetry(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	req, _ := manager.Create(url)
	manager.SetFailed(req.ID, "Original error")

	success := manager.Retry(req.ID)
	if !success {
		t.Fatal("Expected retry to succeed")
	}

	updated, exists := manager.Get(req.ID)
	if !exists {
		t.Fatal("Expected request to exist")
	}

	if updated.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, updated.Status)
	}

	if updated.Progress != 0 {
		t.Errorf("Expected progress 0, got %d", updated.Progress)
	}

	if updated.ErrorMessage != "" {
		t.Error("Expected error message to be cleared")
	}
}

func TestRetryNonExistent(t *testing.T) {
	manager := NewManager()

	success := manager.Retry("non-existent-id")
	if success {
		t.Error("Expected retry of non-existent request to fail")
	}
}

func TestDelete(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	req, _ := manager.Create(url)

	manager.Delete(req.ID)

	_, exists := manager.Get(req.ID)
	if exists {
		t.Error("Expected request to be deleted")
	}

	// Verify URL mapping is also removed
	duplicate, isNew := manager.Create(url)
	if !isNew {
		t.Error("Expected to create new request after deletion")
	}

	if duplicate.ID == req.ID {
		t.Error("Expected new request to have different ID")
	}
}

func TestList(t *testing.T) {
	manager := NewManager()

	// Create multiple requests
	urls := []string{
		"https://example1.com",
		"https://example2.com",
		"https://example3.com",
	}

	for _, url := range urls {
		manager.Create(url)
	}

	requests := manager.List()
	if len(requests) != len(urls) {
		t.Errorf("Expected %d requests, got %d", len(urls), len(requests))
	}

	// Verify all URLs are present (ordering is not guaranteed at Manager level)
	foundURLs := make(map[string]bool)
	for _, req := range requests {
		foundURLs[req.URL] = true
	}

	for _, url := range urls {
		if !foundURLs[url] {
			t.Errorf("Expected to find URL '%s' in list", url)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	// Create initial request
	req, _ := manager.Create(url)

	// Simulate concurrent updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(progress int) {
			manager.UpdateStatus(req.ID, StatusProcessing, progress*10)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify the request still exists and has valid state
	updated, exists := manager.Get(req.ID)
	if !exists {
		t.Fatal("Expected request to exist after concurrent updates")
	}

	if updated.ID != req.ID {
		t.Error("Request ID should not change")
	}
}

func TestExpirationTime(t *testing.T) {
	manager := NewManager()
	url := "https://example.com"

	before := time.Now().Add(15 * time.Minute)
	req, _ := manager.Create(url)
	after := time.Now().Add(15 * time.Minute)

	if req.ExpiresAt.Before(before.Add(-1*time.Second)) {
		t.Error("Expiration time too early")
	}

	if req.ExpiresAt.After(after.Add(1*time.Second)) {
		t.Error("Expiration time too late")
	}
}

func TestDeletePreservesOtherRequests(t *testing.T) {
	manager := NewManager()

	req1, _ := manager.Create("https://example1.com")
	req2, _ := manager.Create("https://example2.com")

	manager.Delete(req1.ID)

	// Verify req2 still exists
	_, exists := manager.Get(req2.ID)
	if !exists {
		t.Error("Expected other request to still exist after delete")
	}

	requests := manager.List()
	if len(requests) != 1 {
		t.Errorf("Expected 1 request, got %d", len(requests))
	}
}

// Text analysis request tests

func TestCreateText(t *testing.T) {
	manager := NewManager()
	text := "This is some text to analyze"

	req, isNew := manager.CreateText(text)

	if !isNew {
		t.Error("Expected isNew to be true for text request")
	}

	if req.ID == "" {
		t.Error("Expected non-empty ID")
	}

	if req.SourceType != "text" {
		t.Errorf("Expected SourceType 'text', got '%s'", req.SourceType)
	}

	if req.Text != text {
		t.Errorf("Expected Text '%s', got '%s'", text, req.Text)
	}

	if req.URL != "" {
		t.Errorf("Expected empty URL for text request, got '%s'", req.URL)
	}

	if req.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, req.Status)
	}

	if req.Progress != 0 {
		t.Errorf("Expected progress 0, got %d", req.Progress)
	}

	if req.ExpiresAt.Before(time.Now()) {
		t.Error("Expected expiration in the future")
	}
}

func TestCreateTextAlwaysCreatesNew(t *testing.T) {
	manager := NewManager()
	text := "Same text for both requests"

	req1, isNew1 := manager.CreateText(text)
	if !isNew1 {
		t.Error("Expected first text request to be new")
	}

	req2, isNew2 := manager.CreateText(text)
	if !isNew2 {
		t.Error("Expected second text request to also be new (no deduplication)")
	}

	if req1.ID == req2.ID {
		t.Error("Expected different IDs for duplicate text requests")
	}

	// Verify both exist in the manager
	requests := manager.List()
	if len(requests) != 2 {
		t.Errorf("Expected 2 requests, got %d", len(requests))
	}
}

func TestDeleteTextRequest(t *testing.T) {
	manager := NewManager()
	text := "Test text"

	req, _ := manager.CreateText(text)

	success := manager.Delete(req.ID)
	if !success {
		t.Error("Expected delete to succeed")
	}

	_, exists := manager.Get(req.ID)
	if exists {
		t.Error("Expected text request to be deleted")
	}

	// Verify we can create a new text request after deletion
	newReq, isNew := manager.CreateText(text)
	if !isNew {
		t.Error("Expected to create new text request after deletion")
	}

	if newReq.ID == req.ID {
		t.Error("Expected new text request to have different ID")
	}
}

func TestMixedURLAndTextRequests(t *testing.T) {
	manager := NewManager()

	urlReq, _ := manager.Create("https://example.com")
	textReq, _ := manager.CreateText("Some text")

	requests := manager.List()
	if len(requests) != 2 {
		t.Errorf("Expected 2 requests, got %d", len(requests))
	}

	// Verify URL request
	url, exists := manager.Get(urlReq.ID)
	if !exists {
		t.Fatal("Expected URL request to exist")
	}
	if url.SourceType != "url" {
		t.Errorf("Expected SourceType 'url', got '%s'", url.SourceType)
	}

	// Verify text request
	text, exists := manager.Get(textReq.ID)
	if !exists {
		t.Fatal("Expected text request to exist")
	}
	if text.SourceType != "text" {
		t.Errorf("Expected SourceType 'text', got '%s'", text.SourceType)
	}

	// Delete text request and verify URL request still exists
	manager.Delete(textReq.ID)

	_, exists = manager.Get(urlReq.ID)
	if !exists {
		t.Error("Expected URL request to still exist after deleting text request")
	}
}

func TestTextRequestOperations(t *testing.T) {
	manager := NewManager()
	text := "Test text for operations"

	req, _ := manager.CreateText(text)

	// Test update status
	manager.UpdateStatus(req.ID, StatusProcessing, 50)
	updated, _ := manager.Get(req.ID)
	if updated.Status != StatusProcessing {
		t.Errorf("Expected status %s, got %s", StatusProcessing, updated.Status)
	}
	if updated.Progress != 50 {
		t.Errorf("Expected progress 50, got %d", updated.Progress)
	}

	// Test set completed
	resultID := "test-result-id"
	manager.SetCompleted(req.ID, resultID)
	completed, _ := manager.Get(req.ID)
	if completed.Status != StatusCompleted {
		t.Errorf("Expected status %s, got %s", StatusCompleted, completed.Status)
	}
	if completed.ResultRequestID != resultID {
		t.Errorf("Expected ResultRequestID '%s', got '%s'", resultID, completed.ResultRequestID)
	}
}

func TestTextRequestRetry(t *testing.T) {
	manager := NewManager()
	text := "Test text for retry"

	req, _ := manager.CreateText(text)
	manager.SetFailed(req.ID, "Test error")

	success := manager.Retry(req.ID)
	if !success {
		t.Fatal("Expected retry to succeed for text request")
	}

	retried, exists := manager.Get(req.ID)
	if !exists {
		t.Fatal("Expected text request to exist after retry")
	}

	if retried.Status != StatusPending {
		t.Errorf("Expected status %s, got %s", StatusPending, retried.Status)
	}

	if retried.ErrorMessage != "" {
		t.Error("Expected error message to be cleared after retry")
	}
}
