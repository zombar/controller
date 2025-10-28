package events

import (
	"encoding/json"
	"sync"
)

// DocumentUpdateEvent represents a document status update
type DocumentUpdateEvent struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

// Subscriber represents a client listening for events
type Subscriber struct {
	ID       string
	Events   chan DocumentUpdateEvent
	RequestID string // Only send events for this request
}

// Broadcaster manages SSE subscriptions and publishes events
type Broadcaster struct {
	subscribers map[string]*Subscriber
	mu          sync.RWMutex
}

// NewBroadcaster creates a new event broadcaster
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		subscribers: make(map[string]*Subscriber),
	}
}

// Subscribe adds a new subscriber for a specific request
func (b *Broadcaster) Subscribe(id string, requestID string) *Subscriber {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &Subscriber{
		ID:        id,
		Events:    make(chan DocumentUpdateEvent, 10),
		RequestID: requestID,
	}
	b.subscribers[id] = sub
	return sub
}

// Unsubscribe removes a subscriber
func (b *Broadcaster) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if sub, ok := b.subscribers[id]; ok {
		close(sub.Events)
		delete(b.subscribers, id)
	}
}

// Publish sends an event to all subscribers watching this request
func (b *Broadcaster) Publish(event DocumentUpdateEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subscribers {
		if sub.RequestID == event.RequestID {
			select {
			case sub.Events <- event:
			default:
				// Channel full, skip this event
			}
		}
	}
}

// MarshalEvent formats an event for SSE
func MarshalEvent(event DocumentUpdateEvent) (string, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	return "data: " + string(data) + "\n\n", nil
}
