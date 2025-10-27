package urlcache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// setupTestCache creates a test cache with an in-memory Redis instance
func setupTestCache(t *testing.T) (*Cache, *miniredis.Miniredis) {
	t.Helper()

	// Create an in-memory Redis instance for testing
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to create miniredis: %v", err)
	}

	// Create cache with miniredis address
	cache := &Cache{
		client: redis.NewClient(&redis.Options{
			Addr: mr.Addr(),
		}),
	}

	return cache, mr
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "basic URL",
			input:    "https://example.com/path",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "URL with UTM parameters",
			input:    "https://example.com/path?utm_source=twitter&utm_campaign=test",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "URL with mixed tracking and real parameters",
			input:    "https://example.com/search?q=test&utm_source=facebook&page=2&fbclid=123",
			expected: "https://example.com/search?page=2&q=test",
			wantErr:  false,
		},
		{
			name:     "URL with fragment",
			input:    "https://example.com/path#section",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "URL with trailing slash",
			input:    "https://example.com/path/",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "URL with uppercase scheme and host",
			input:    "HTTPS://EXAMPLE.COM/Path",
			expected: "https://example.com/Path",
			wantErr:  false,
		},
		{
			name:     "URL with sorted parameters",
			input:    "https://example.com/path?z=1&a=2&m=3",
			expected: "https://example.com/path?a=2&m=3&z=1",
			wantErr:  false,
		},
		{
			name:     "URL with Google click ID",
			input:    "https://example.com/path?gclid=abc123",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "URL with Facebook parameters",
			input:    "https://example.com/path?fbclid=xyz&fb_source=test",
			expected: "https://example.com/path",
			wantErr:  false,
		},
		{
			name:     "root path keeps slash",
			input:    "https://example.com/",
			expected: "https://example.com/",
			wantErr:  false,
		},
		{
			name:     "invalid URL",
			input:    "not a valid url",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "URL with duplicate parameter values sorted",
			input:    "https://example.com?tag=b&tag=a",
			expected: "https://example.com?tag=a&tag=b",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := normalizeURL(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("normalizeURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("normalizeURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHashURL(t *testing.T) {
	tests := []struct {
		name      string
		url1      string
		url2      string
		shouldMatch bool
	}{
		{
			name:      "identical URLs produce same hash",
			url1:      "https://example.com/path",
			url2:      "https://example.com/path",
			shouldMatch: true,
		},
		{
			name:      "URLs with different tracking params produce same hash",
			url1:      "https://example.com/path?utm_source=twitter",
			url2:      "https://example.com/path?utm_source=facebook",
			shouldMatch: true,
		},
		{
			name:      "URLs with different real params produce different hash",
			url1:      "https://example.com/path?page=1",
			url2:      "https://example.com/path?page=2",
			shouldMatch: false,
		},
		{
			name:      "URLs with same params in different order produce same hash",
			url1:      "https://example.com/path?a=1&b=2",
			url2:      "https://example.com/path?b=2&a=1",
			shouldMatch: true,
		},
		{
			name:      "URLs with trailing slash vs without produce same hash",
			url1:      "https://example.com/path/",
			url2:      "https://example.com/path",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash1, err1 := hashURL(tt.url1)
			hash2, err2 := hashURL(tt.url2)

			if err1 != nil {
				t.Fatalf("hashURL(%s) failed: %v", tt.url1, err1)
			}
			if err2 != nil {
				t.Fatalf("hashURL(%s) failed: %v", tt.url2, err2)
			}

			if tt.shouldMatch {
				if hash1 != hash2 {
					t.Errorf("Expected hashes to match:\n  URL1: %s (hash: %s)\n  URL2: %s (hash: %s)",
						tt.url1, hash1, tt.url2, hash2)
				}
			} else {
				if hash1 == hash2 {
					t.Errorf("Expected hashes to differ:\n  URL1: %s (hash: %s)\n  URL2: %s (hash: %s)",
						tt.url1, hash1, tt.url2, hash2)
				}
			}
		})
	}
}

func TestCacheSetAndGet(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	testURL := "https://example.com/test"
	testUUID := "550e8400-e29b-41d4-a716-446655440000"

	// Test Set
	err := cache.Set(ctx, testURL, testUUID)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Test Get - should retrieve the value
	retrievedUUID, err := cache.Get(ctx, testURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrievedUUID != testUUID {
		t.Errorf("Get() = %v, want %v", retrievedUUID, testUUID)
	}
}

func TestCacheGetNonExistent(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	testURL := "https://example.com/nonexistent"

	// Test Get on non-existent key
	retrievedUUID, err := cache.Get(ctx, testURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrievedUUID != "" {
		t.Errorf("Get() = %v, want empty string", retrievedUUID)
	}
}

func TestCacheNormalization(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	testUUID := "550e8400-e29b-41d4-a716-446655440000"

	// Set with tracking parameters
	url1 := "https://example.com/path?utm_source=twitter&page=1"
	err := cache.Set(ctx, url1, testUUID)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get with different tracking parameters but same real parameters
	url2 := "https://example.com/path?utm_campaign=test&page=1&fbclid=xyz"
	retrievedUUID, err := cache.Get(ctx, url2)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrievedUUID != testUUID {
		t.Errorf("Get() = %v, want %v (URLs should normalize to same value)", retrievedUUID, testUUID)
	}
}

func TestCacheDelete(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	testURL := "https://example.com/test"
	testUUID := "550e8400-e29b-41d4-a716-446655440000"

	// Set a value
	err := cache.Set(ctx, testURL, testUUID)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Verify it exists
	retrievedUUID, err := cache.Get(ctx, testURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrievedUUID != testUUID {
		t.Errorf("Get() = %v, want %v", retrievedUUID, testUUID)
	}

	// Delete it
	err = cache.Delete(ctx, testURL)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	retrievedUUID, err = cache.Get(ctx, testURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrievedUUID != "" {
		t.Errorf("Get() after Delete = %v, want empty string", retrievedUUID)
	}
}

func TestCacheTTL(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	testURL := "https://example.com/test"
	testUUID := "550e8400-e29b-41d4-a716-446655440000"

	// Set a value
	err := cache.Set(ctx, testURL, testUUID)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Fast-forward time in miniredis to expire the key
	mr.FastForward(CacheTTL + time.Second)

	// Verify key has expired
	retrievedUUID, err := cache.Get(ctx, testURL)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrievedUUID != "" {
		t.Errorf("Get() after TTL expiration = %v, want empty string", retrievedUUID)
	}
}

func TestCachePing(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()

	// Test Ping
	err := cache.Ping(ctx)
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestMakeKey(t *testing.T) {
	urlHash := "abcdef123456"
	expected := KeyPrefix + urlHash

	result := makeKey(urlHash)
	if result != expected {
		t.Errorf("makeKey() = %v, want %v", result, expected)
	}
}

func TestCacheInvalidURL(t *testing.T) {
	cache, mr := setupTestCache(t)
	defer mr.Close()

	ctx := context.Background()
	invalidURL := "not a valid url"
	testUUID := "550e8400-e29b-41d4-a716-446655440000"

	// Test Set with invalid URL
	err := cache.Set(ctx, invalidURL, testUUID)
	if err == nil {
		t.Error("Set() with invalid URL should return error")
	}

	// Test Get with invalid URL
	_, err = cache.Get(ctx, invalidURL)
	if err == nil {
		t.Error("Get() with invalid URL should return error")
	}

	// Test Delete with invalid URL
	err = cache.Delete(ctx, invalidURL)
	if err == nil {
		t.Error("Delete() with invalid URL should return error")
	}
}
