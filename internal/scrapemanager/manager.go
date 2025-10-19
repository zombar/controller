package scrapemanager

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// ScrapeStatus represents the current status of a scrape request
type ScrapeStatus string

const (
	StatusPending    ScrapeStatus = "pending"
	StatusProcessing ScrapeStatus = "processing"
	StatusCompleted  ScrapeStatus = "completed"
	StatusFailed     ScrapeStatus = "failed"
)

// ScrapeRequest represents an in-memory scrape request
type ScrapeRequest struct {
	ID               string                 `json:"id"`
	URL              string                 `json:"url"`
	Status           ScrapeStatus           `json:"status"`
	CreatedAt        time.Time              `json:"created_at"`
	StartedAt        *time.Time             `json:"started_at,omitempty"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty"`
	ResultRequestID  *string                `json:"result_request_id,omitempty"` // ID of completed request in storage
	ErrorMessage     *string                `json:"error_message,omitempty"`
	Progress         int                    `json:"progress"` // 0-100
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// Manager handles in-memory scrape request tracking
type Manager struct {
	mu       sync.RWMutex
	requests map[string]*ScrapeRequest
	urlIndex map[string]string // URL -> Request ID mapping for duplicate detection
	ttl      time.Duration
}

// New creates a new scrape request manager
func New(ttl time.Duration) *Manager {
	return &Manager{
		requests: make(map[string]*ScrapeRequest),
		urlIndex: make(map[string]string),
		ttl:      ttl,
	}
}

// Create creates a new scrape request
func (m *Manager) Create(url string) *ScrapeRequest {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if URL is already being scraped
	if existingID, exists := m.urlIndex[url]; exists {
		if req, found := m.requests[existingID]; found {
			// Only return existing if it's still pending or processing
			if req.Status == StatusPending || req.Status == StatusProcessing {
				return req
			}
		}
	}

	// Create new request
	id := uuid.New().String()
	req := &ScrapeRequest{
		ID:        id,
		URL:       url,
		Status:    StatusPending,
		CreatedAt: time.Now().UTC(),
		Progress:  0,
		Metadata:  make(map[string]interface{}),
	}

	m.requests[id] = req
	m.urlIndex[url] = id

	return req
}

// Get retrieves a scrape request by ID
func (m *Manager) Get(id string) (*ScrapeRequest, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	req, exists := m.requests[id]
	return req, exists
}

// List returns all scrape requests
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
func (m *Manager) UpdateStatus(id string, status ScrapeStatus) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, exists := m.requests[id]
	if !exists {
		return false
	}

	req.Status = status

	now := time.Now().UTC()
	switch status {
	case StatusProcessing:
		if req.StartedAt == nil {
			req.StartedAt = &now
		}
	case StatusCompleted, StatusFailed:
		req.CompletedAt = &now
	}

	return true
}

// UpdateProgress updates the progress of a scrape request
func (m *Manager) UpdateProgress(id string, progress int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, exists := m.requests[id]
	if !exists {
		return false
	}

	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}

	req.Progress = progress
	return true
}

// SetResult sets the result request ID for a completed scrape
func (m *Manager) SetResult(id string, resultRequestID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, exists := m.requests[id]
	if !exists {
		return false
	}

	req.ResultRequestID = &resultRequestID
	return true
}

// SetError sets the error message for a failed scrape
func (m *Manager) SetError(id string, errorMsg string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, exists := m.requests[id]
	if !exists {
		return false
	}

	req.ErrorMessage = &errorMsg
	return true
}

// Delete removes a scrape request
func (m *Manager) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	req, exists := m.requests[id]
	if !exists {
		return false
	}

	// Remove from URL index
	delete(m.urlIndex, req.URL)

	// Remove request
	delete(m.requests, id)

	return true
}

// Cleanup removes completed/failed requests older than TTL
func (m *Manager) Cleanup() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	removed := 0

	for id, req := range m.requests {
		// Only cleanup completed or failed requests
		if req.Status != StatusCompleted && req.Status != StatusFailed {
			continue
		}

		// Check if completed time exists and is older than TTL
		if req.CompletedAt != nil && now.Sub(*req.CompletedAt) > m.ttl {
			delete(m.urlIndex, req.URL)
			delete(m.requests, id)
			removed++
		}
	}

	return removed
}

// StartCleanupLoop starts a background goroutine that periodically cleans up old requests
func (m *Manager) StartCleanupLoop(interval time.Duration) chan struct{} {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				removed := m.Cleanup()
				if removed > 0 {
					// Could add logging here if needed
				}
			case <-stop:
				return
			}
		}
	}()

	return stop
}
