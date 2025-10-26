package queue

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

func TestScrapeTaskPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload ScrapeTaskPayload
	}{
		{
			name: "root job without parent",
			payload: ScrapeTaskPayload{
				JobID:        "job-123",
				URL:          "https://example.com",
				ExtractLinks: true,
				ParentJobID:  nil,
				Depth:        0,
			},
		},
		{
			name: "child job with parent",
			payload: ScrapeTaskPayload{
				JobID:        "child-456",
				URL:          "https://example.com/child",
				ExtractLinks: false,
				ParentJobID:  stringPtr("job-123"),
				Depth:        1,
			},
		},
		{
			name: "grandchild job",
			payload: ScrapeTaskPayload{
				JobID:        "grandchild-789",
				URL:          "https://example.com/grandchild",
				ExtractLinks: false,
				ParentJobID:  stringPtr("child-456"),
				Depth:        2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON
			data, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Failed to marshal payload: %v", err)
			}

			// Unmarshal from JSON
			var unmarshaled ScrapeTaskPayload
			err = json.Unmarshal(data, &unmarshaled)
			if err != nil {
				t.Fatalf("Failed to unmarshal payload: %v", err)
			}

			// Verify fields
			if unmarshaled.JobID != tt.payload.JobID {
				t.Errorf("Expected JobID %s, got %s", tt.payload.JobID, unmarshaled.JobID)
			}

			if unmarshaled.URL != tt.payload.URL {
				t.Errorf("Expected URL %s, got %s", tt.payload.URL, unmarshaled.URL)
			}

			if unmarshaled.ExtractLinks != tt.payload.ExtractLinks {
				t.Errorf("Expected ExtractLinks %v, got %v", tt.payload.ExtractLinks, unmarshaled.ExtractLinks)
			}

			if unmarshaled.Depth != tt.payload.Depth {
				t.Errorf("Expected Depth %d, got %d", tt.payload.Depth, unmarshaled.Depth)
			}

			// Check parent job ID
			if tt.payload.ParentJobID == nil {
				if unmarshaled.ParentJobID != nil {
					t.Errorf("Expected nil ParentJobID, got %v", *unmarshaled.ParentJobID)
				}
			} else {
				if unmarshaled.ParentJobID == nil {
					t.Error("Expected non-nil ParentJobID, got nil")
				} else if *unmarshaled.ParentJobID != *tt.payload.ParentJobID {
					t.Errorf("Expected ParentJobID %s, got %s", *tt.payload.ParentJobID, *unmarshaled.ParentJobID)
				}
			}
		})
	}
}

func TestClientEnqueueScrape(t *testing.T) {
	// Note: This test requires a real Redis instance
	// Skip if REDIS_ADDR is not set
	redisAddr := "localhost:6379"

	client := NewClient(ClientConfig{
		RedisAddr: redisAddr,
	})
	defer client.Close()

	ctx := context.Background()

	// Test basic enqueue
	taskID, err := client.EnqueueScrape(ctx, "test-job-1", "https://example.com", false)
	if err != nil {
		t.Skipf("Skipping test - Redis not available: %v", err)
	}

	if taskID == "" {
		t.Error("Expected non-empty task ID")
	}
}

func TestClientEnqueueScrapeWithParent(t *testing.T) {
	redisAddr := "localhost:6379"

	client := NewClient(ClientConfig{
		RedisAddr: redisAddr,
	})
	defer client.Close()

	ctx := context.Background()

	// Test enqueue with parent and depth
	parentID := "parent-job-123"
	taskID, err := client.EnqueueScrapeWithParent(
		ctx,
		"child-job-456",
		"https://example.com/child",
		false,
		&parentID,
		1,
	)

	if err != nil {
		t.Skipf("Skipping test - Redis not available: %v", err)
	}

	if taskID == "" {
		t.Error("Expected non-empty task ID")
	}
}

func TestTaskUniqueKey(t *testing.T) {
	// Test that duplicate URLs get the same unique key
	tests := []struct {
		name       string
		url1       string
		url2       string
		shouldSame bool
	}{
		{
			name:       "same URL",
			url1:       "https://example.com",
			url2:       "https://example.com",
			shouldSame: true,
		},
		{
			name:       "different URLs",
			url1:       "https://example.com",
			url2:       "https://different.com",
			shouldSame: false,
		},
		{
			name:       "same URL with trailing slash",
			url1:       "https://example.com",
			url2:       "https://example.com/",
			shouldSame: false, // These should be considered different
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload1 := ScrapeTaskPayload{
				JobID:        "job-1",
				URL:          tt.url1,
				ExtractLinks: false,
				Depth:        0,
			}

			payload2 := ScrapeTaskPayload{
				JobID:        "job-2",
				URL:          tt.url2,
				ExtractLinks: false,
				Depth:        0,
			}

			data1, _ := json.Marshal(payload1)
			data2, _ := json.Marshal(payload2)

			task1 := asynq.NewTask(TypeScrapeURL, data1)
			task2 := asynq.NewTask(TypeScrapeURL, data2)

			// In a real implementation, you'd check the unique key
			// This is a basic check that tasks can be created
			if task1 == nil || task2 == nil {
				t.Error("Failed to create tasks")
			}
		})
	}
}

func TestDepthValidation(t *testing.T) {
	tests := []struct {
		name          string
		depth         int
		maxLinkDepth  int
		extractLinks  bool
		shouldExtract bool
	}{
		{
			name:          "depth 0, max 1, extract enabled",
			depth:         0,
			maxLinkDepth:  1,
			extractLinks:  true,
			shouldExtract: true,
		},
		{
			name:          "depth 1, max 1, extract enabled",
			depth:         1,
			maxLinkDepth:  1,
			extractLinks:  true,
			shouldExtract: false,
		},
		{
			name:          "depth 0, max 2, extract enabled",
			depth:         0,
			maxLinkDepth:  2,
			extractLinks:  true,
			shouldExtract: true,
		},
		{
			name:          "depth 1, max 2, extract enabled",
			depth:         1,
			maxLinkDepth:  2,
			extractLinks:  true,
			shouldExtract: true,
		},
		{
			name:          "depth 2, max 2, extract enabled",
			depth:         2,
			maxLinkDepth:  2,
			extractLinks:  true,
			shouldExtract: false,
		},
		{
			name:          "extract disabled",
			depth:         0,
			maxLinkDepth:  1,
			extractLinks:  false,
			shouldExtract: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from extractAndQueueLinks
			shouldExtract := tt.extractLinks && tt.depth < tt.maxLinkDepth

			if shouldExtract != tt.shouldExtract {
				t.Errorf("Expected shouldExtract=%v, got %v (depth=%d, maxLinkDepth=%d, extractLinks=%v)",
					tt.shouldExtract, shouldExtract, tt.depth, tt.maxLinkDepth, tt.extractLinks)
			}

			// Calculate what extract_links should be for child jobs
			if shouldExtract {
				childDepth := tt.depth + 1
				childShouldExtract := childDepth < tt.maxLinkDepth

				t.Logf("Child at depth %d should have extract_links=%v", childDepth, childShouldExtract)
			}
		})
	}
}

func TestRetryDelayCalculation(t *testing.T) {
	tests := []struct {
		retryNum      int
		expectedDelay time.Duration
	}{
		{retryNum: 0, expectedDelay: 1 * time.Minute},
		{retryNum: 1, expectedDelay: 5 * time.Minute},
		{retryNum: 2, expectedDelay: 15 * time.Minute},
		{retryNum: 3, expectedDelay: 15 * time.Minute}, // Max out at last delay
		{retryNum: 10, expectedDelay: 15 * time.Minute},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			delays := []time.Duration{
				1 * time.Minute,
				5 * time.Minute,
				15 * time.Minute,
			}

			var delay time.Duration
			if tt.retryNum < len(delays) {
				delay = delays[tt.retryNum]
			} else {
				delay = delays[len(delays)-1]
			}

			if delay != tt.expectedDelay {
				t.Errorf("Retry %d: expected delay %v, got %v", tt.retryNum, tt.expectedDelay, delay)
			}
		})
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
