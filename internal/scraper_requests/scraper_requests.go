package scraper_requests

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// ScrapeRequestStatus represents the current state of a scrape request
type ScrapeRequestStatus string

const (
	StatusPending    ScrapeRequestStatus = "pending"
	StatusProcessing ScrapeRequestStatus = "processing"
	StatusCompleted  ScrapeRequestStatus = "completed"
	StatusFailed     ScrapeRequestStatus = "failed"
)

// ScrapeRequest represents an in-memory scrape request
type ScrapeRequest struct {
	ID               string              `json:"id"`
	SourceType       string              `json:"source_type"`         // "url" or "text"
	URL              string              `json:"url,omitempty"`       // For URL requests
	Text             string              `json:"text,omitempty"`      // For text requests
	Status           ScrapeRequestStatus `json:"status"`
	Progress         int                 `json:"progress"` // 0-100
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
	ResultRequestID  string              `json:"result_request_id,omitempty"` // Controller request ID when completed
	ErrorMessage     string              `json:"error_message,omitempty"`
	ExpiresAt        time.Time           `json:"expires_at"` // Auto-cleanup after 15 minutes
}

// Manager handles in-memory scrape request tracking
type Manager struct {
	requests map[string]*ScrapeRequest // keyed by request ID
	urlMap   map[string]string         // URL -> request ID mapping for duplicate detection
	mu       sync.RWMutex
}

// NewManager creates a new scrape request manager
func NewManager() *Manager {
	m := &Manager{
		requests: make(map[string]*ScrapeRequest),
		urlMap:   make(map[string]string),
	}

	// Start cleanup goroutine
	go m.cleanupExpired()

	return m
}

// Create creates a new scrape request or returns existing one for the same URL
func (m *Manager) Create(url string) (*ScrapeRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if URL already has an active request
	if existingID, exists := m.urlMap[url]; exists {
		if req, ok := m.requests[existingID]; ok {
			// Return existing request
			return req, false
		}
		// Clean up stale urlMap entry
		delete(m.urlMap, url)
	}

	// Create new request
	now := time.Now()
	req := &ScrapeRequest{
		ID:         uuid.New().String(),
		SourceType: "url",
		URL:        url,
		Status:     StatusPending,
		Progress:   0,
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  now.Add(15 * time.Minute),
	}

	m.requests[req.ID] = req
	m.urlMap[url] = req.ID

	return req, true
}

// CreateText creates a new text analysis request
// Text requests are always created fresh (no duplicate detection)
func (m *Manager) CreateText(text string) (*ScrapeRequest, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create new request
	now := time.Now()
	req := &ScrapeRequest{
		ID:         uuid.New().String(),
		SourceType: "text",
		Text:       text,
		Status:     StatusPending,
		Progress:   0,
		CreatedAt:  now,
		UpdatedAt:  now,
		ExpiresAt:  now.Add(15 * time.Minute),
	}

	m.requests[req.ID] = req

	return req, true
}

// Get retrieves a scrape request by ID
func (m *Manager) Get(id string) (*ScrapeRequest, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	req, ok := m.requests[id]
	return req, ok
}

// List returns all active scrape requests
func (m *Manager) List() []*ScrapeRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()

	requests := make([]*ScrapeRequest, 0, len(m.requests))
	for _, req := range m.requests {
		requests = append(requests, req)
	}

	return requests
}

// UpdateStatus updates the status of a scrape request
func (m *Manager) UpdateStatus(id string, status ScrapeRequestStatus, progress int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return false
	}

	req.Status = status
	req.Progress = progress
	req.UpdatedAt = time.Now()

	return true
}

// SetCompleted marks a request as completed with the result request ID
func (m *Manager) SetCompleted(id string, resultRequestID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return false
	}

	req.Status = StatusCompleted
	req.Progress = 100
	req.ResultRequestID = resultRequestID
	req.UpdatedAt = time.Now()

	return true
}

// SetFailed marks a request as failed with an error message
func (m *Manager) SetFailed(id string, errorMsg string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return false
	}

	req.Status = StatusFailed
	req.ErrorMessage = errorMsg
	req.UpdatedAt = time.Now()

	return true
}

// Delete removes a scrape request
func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return false
	}

	// Remove from URL map if it's a URL request
	if req.SourceType == "url" && req.URL != "" {
		delete(m.urlMap, req.URL)
	}

	// Remove from requests map
	delete(m.requests, id)

	return true
}

// Retry resets a failed request to pending state
func (m *Manager) Retry(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, ok := m.requests[id]
	if !ok {
		return false
	}

	req.Status = StatusPending
	req.Progress = 0
	req.ErrorMessage = ""
	req.UpdatedAt = time.Now()
	return true
}

// cleanupExpired removes expired scrape requests
func (m *Manager) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()

		for id, req := range m.requests {
			if now.After(req.ExpiresAt) {
				// Remove from URL map if it's a URL request
				if req.SourceType == "url" && req.URL != "" {
					delete(m.urlMap, req.URL)
				}
				delete(m.requests, id)
			}
		}

		m.mu.Unlock()
	}
}
