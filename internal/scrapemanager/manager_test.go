package scrapemanager

import (
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	ttl := 15 * time.Minute
	manager := New(ttl)

	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.ttl != ttl {
		t.Errorf("Expected TTL %v, got %v", ttl, manager.ttl)
	}

	if manager.requests == nil {
		t.Error("Expected requests map to be initialized")
	}

	if manager.urlIndex == nil {
		t.Error("Expected urlIndex map to be initialized")
	}
}

func TestCreate(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	req := manager.Create(url)

	if req == nil {
		t.Fatal("Expected non-nil request")
	}

	if req.ID == "" {
		t.Error("Expected non-empty ID")
	}

	if req.URL != url {
		t.Errorf("Expected URL '%s', got '%s'", url, req.URL)
	}

	if req.Status != StatusPending {
		t.Errorf("Expected status '%s', got '%s'", StatusPending, req.Status)
	}

	if req.Progress != 0 {
		t.Errorf("Expected progress 0, got %d", req.Progress)
	}

	if req.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestCreateDuplicate(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	req1 := manager.Create(url)
	req2 := manager.Create(url)

	if req1.ID != req2.ID {
		t.Errorf("Expected same ID for duplicate URL, got '%s' and '%s'", req1.ID, req2.ID)
	}

	// Check that only one request exists
	requests := manager.List()
	if len(requests) != 1 {
		t.Errorf("Expected 1 request, got %d", len(requests))
	}
}

func TestCreateDuplicateAfterCompletion(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	req1 := manager.Create(url)

	// Mark as completed
	manager.UpdateStatus(req1.ID, StatusCompleted)

	// Create again with same URL
	req2 := manager.Create(url)

	// Should create a new request since the first one is completed
	if req1.ID == req2.ID {
		t.Error("Expected different ID for new request after completion")
	}
}

func TestGet(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	created := manager.Create(url)

	retrieved, exists := manager.Get(created.ID)
	if !exists {
		t.Error("Expected request to exist")
	}

	if retrieved.ID != created.ID {
		t.Errorf("Expected ID '%s', got '%s'", created.ID, retrieved.ID)
	}
}

func TestGetNotFound(t *testing.T) {
	manager := New(15 * time.Minute)

	_, exists := manager.Get("non-existent-id")
	if exists {
		t.Error("Expected request not to exist")
	}
}

func TestList(t *testing.T) {
	manager := New(15 * time.Minute)

	// Create multiple requests
	urls := []string{
		"https://example.com",
		"https://example.org",
		"https://example.net",
	}

	for _, url := range urls {
		manager.Create(url)
	}

	requests := manager.List()
	if len(requests) != len(urls) {
		t.Errorf("Expected %d requests, got %d", len(urls), len(requests))
	}
}

func TestUpdateStatus(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	req := manager.Create(url)

	// Update to processing
	success := manager.UpdateStatus(req.ID, StatusProcessing)
	if !success {
		t.Error("Expected UpdateStatus to succeed")
	}

	updated, _ := manager.Get(req.ID)
	if updated.Status != StatusProcessing {
		t.Errorf("Expected status '%s', got '%s'", StatusProcessing, updated.Status)
	}

	if updated.StartedAt == nil {
		t.Error("Expected StartedAt to be set when status changes to processing")
	}

	// Update to completed
	manager.UpdateStatus(req.ID, StatusCompleted)
	updated, _ = manager.Get(req.ID)

	if updated.Status != StatusCompleted {
		t.Errorf("Expected status '%s', got '%s'", StatusCompleted, updated.Status)
	}

	if updated.CompletedAt == nil {
		t.Error("Expected CompletedAt to be set when status changes to completed")
	}
}

func TestUpdateStatusNotFound(t *testing.T) {
	manager := New(15 * time.Minute)

	success := manager.UpdateStatus("non-existent-id", StatusCompleted)
	if success {
		t.Error("Expected UpdateStatus to fail for non-existent request")
	}
}

func TestUpdateProgress(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	req := manager.Create(url)

	success := manager.UpdateProgress(req.ID, 50)
	if !success {
		t.Error("Expected UpdateProgress to succeed")
	}

	updated, _ := manager.Get(req.ID)
	if updated.Progress != 50 {
		t.Errorf("Expected progress 50, got %d", updated.Progress)
	}

	// Test clamping to 0
	manager.UpdateProgress(req.ID, -10)
	updated, _ = manager.Get(req.ID)
	if updated.Progress != 0 {
		t.Errorf("Expected progress to be clamped to 0, got %d", updated.Progress)
	}

	// Test clamping to 100
	manager.UpdateProgress(req.ID, 150)
	updated, _ = manager.Get(req.ID)
	if updated.Progress != 100 {
		t.Errorf("Expected progress to be clamped to 100, got %d", updated.Progress)
	}
}

func TestUpdateProgressNotFound(t *testing.T) {
	manager := New(15 * time.Minute)

	success := manager.UpdateProgress("non-existent-id", 50)
	if success {
		t.Error("Expected UpdateProgress to fail for non-existent request")
	}
}

func TestSetResult(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	req := manager.Create(url)

	resultID := "result-123"
	success := manager.SetResult(req.ID, resultID)
	if !success {
		t.Error("Expected SetResult to succeed")
	}

	updated, _ := manager.Get(req.ID)
	if updated.ResultRequestID == nil {
		t.Error("Expected ResultRequestID to be set")
	}

	if *updated.ResultRequestID != resultID {
		t.Errorf("Expected ResultRequestID '%s', got '%s'", resultID, *updated.ResultRequestID)
	}
}

func TestSetResultNotFound(t *testing.T) {
	manager := New(15 * time.Minute)

	success := manager.SetResult("non-existent-id", "result-123")
	if success {
		t.Error("Expected SetResult to fail for non-existent request")
	}
}

func TestSetError(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	req := manager.Create(url)

	errorMsg := "Failed to scrape"
	success := manager.SetError(req.ID, errorMsg)
	if !success {
		t.Error("Expected SetError to succeed")
	}

	updated, _ := manager.Get(req.ID)
	if updated.ErrorMessage == nil {
		t.Error("Expected ErrorMessage to be set")
	}

	if *updated.ErrorMessage != errorMsg {
		t.Errorf("Expected ErrorMessage '%s', got '%s'", errorMsg, *updated.ErrorMessage)
	}
}

func TestSetErrorNotFound(t *testing.T) {
	manager := New(15 * time.Minute)

	success := manager.SetError("non-existent-id", "error")
	if success {
		t.Error("Expected SetError to fail for non-existent request")
	}
}

func TestDelete(t *testing.T) {
	manager := New(15 * time.Minute)

	url := "https://example.com"
	req := manager.Create(url)

	success := manager.Delete(req.ID)
	if !success {
		t.Error("Expected Delete to succeed")
	}

	_, exists := manager.Get(req.ID)
	if exists {
		t.Error("Expected request to be deleted")
	}

	// Verify URL index is also cleaned up
	newReq := manager.Create(url)
	if newReq.ID == req.ID {
		t.Error("Expected new request to have different ID after deletion")
	}
}

func TestDeleteNotFound(t *testing.T) {
	manager := New(15 * time.Minute)

	success := manager.Delete("non-existent-id")
	if success {
		t.Error("Expected Delete to fail for non-existent request")
	}
}

func TestCleanup(t *testing.T) {
	// Use a very short TTL for testing
	manager := New(100 * time.Millisecond)

	// Create completed request
	url1 := "https://example.com"
	req1 := manager.Create(url1)
	manager.UpdateStatus(req1.ID, StatusCompleted)

	// Create failed request
	url2 := "https://example.org"
	req2 := manager.Create(url2)
	manager.UpdateStatus(req2.ID, StatusFailed)

	// Create pending request (should not be cleaned up)
	url3 := "https://example.net"
	req3 := manager.Create(url3)

	// Wait for TTL to expire
	time.Sleep(150 * time.Millisecond)

	// Run cleanup
	removed := manager.Cleanup()

	// Should remove completed and failed requests
	if removed != 2 {
		t.Errorf("Expected 2 requests to be removed, got %d", removed)
	}

	// Verify pending request still exists
	_, exists := manager.Get(req3.ID)
	if !exists {
		t.Error("Expected pending request to still exist after cleanup")
	}

	// Verify completed and failed requests are removed
	_, exists = manager.Get(req1.ID)
	if exists {
		t.Error("Expected completed request to be removed")
	}

	_, exists = manager.Get(req2.ID)
	if exists {
		t.Error("Expected failed request to be removed")
	}
}

func TestCleanupBeforeTTL(t *testing.T) {
	manager := New(10 * time.Minute)

	url := "https://example.com"
	req := manager.Create(url)
	manager.UpdateStatus(req.ID, StatusCompleted)

	// Run cleanup immediately (before TTL expires)
	removed := manager.Cleanup()

	// Should not remove anything
	if removed != 0 {
		t.Errorf("Expected 0 requests to be removed, got %d", removed)
	}

	// Verify request still exists
	_, exists := manager.Get(req.ID)
	if !exists {
		t.Error("Expected request to still exist before TTL expires")
	}
}

func TestStartCleanupLoop(t *testing.T) {
	manager := New(50 * time.Millisecond)

	// Create and complete a request
	url := "https://example.com"
	req := manager.Create(url)
	manager.UpdateStatus(req.ID, StatusCompleted)

	// Start cleanup loop with short interval
	stop := manager.StartCleanupLoop(100 * time.Millisecond)
	defer close(stop)

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Verify request is removed by cleanup loop
	_, exists := manager.Get(req.ID)
	if exists {
		t.Error("Expected request to be removed by cleanup loop")
	}
}

func TestConcurrency(t *testing.T) {
	manager := New(15 * time.Minute)

	// Test concurrent Creates
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			url := "https://example.com/page-" + string(rune('0'+index))
			req := manager.Create(url)
			if req == nil {
				t.Error("Expected non-nil request")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all requests were created
	requests := manager.List()
	if len(requests) != 10 {
		t.Errorf("Expected 10 requests, got %d", len(requests))
	}
}
