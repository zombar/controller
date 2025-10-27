package urlcache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// CacheTTL is the time-to-live for cached URLs (30 days)
	CacheTTL = 30 * 24 * time.Hour
	// KeyPrefix is the prefix for all cache keys
	KeyPrefix = "urlcache:"
)

// trackingParams are common tracking/analytics parameters that don't affect content
var trackingParams = map[string]bool{
	// UTM parameters (Google Analytics)
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	// Facebook
	"fbclid": true,
	// Google Ads
	"gclid":  true,
	"gclsrc": true,
	// Other tracking
	"ref":        true,
	"source":     true,
	"referrer":   true,
	"campaign":   true,
	"_ga":        true,
	"mc_cid":     true,
	"mc_eid":     true,
	"msclkid":    true,
	"yclid":      true,
	"_openstat":  true,
	"fb_action_ids": true,
	"fb_action_types": true,
	"fb_ref":     true,
	"fb_source":  true,
	"action_object_map": true,
	"action_type_map":   true,
	"action_ref_map":    true,
}

// Cache provides URL caching functionality using Redis
type Cache struct {
	client *redis.Client
}

// New creates a new URL cache instance
func New(redisAddr string) *Cache {
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	return &Cache{
		client: client,
	}
}

// normalizeURL normalizes a URL for caching by:
// 1. Converting scheme and host to lowercase
// 2. Removing tracking parameters
// 3. Sorting remaining query parameters
// 4. Removing trailing slash
// 5. Removing fragment (#)
func normalizeURL(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	// Validate that URL has scheme and host
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return "", fmt.Errorf("invalid URL: missing scheme or host")
	}

	// Normalize scheme and host to lowercase
	parsedURL.Scheme = strings.ToLower(parsedURL.Scheme)
	parsedURL.Host = strings.ToLower(parsedURL.Host)

	// Remove fragment
	parsedURL.Fragment = ""

	// Parse and filter query parameters
	query := parsedURL.Query()
	filteredQuery := url.Values{}

	// Remove tracking parameters and sort the rest
	for key, values := range query {
		if !trackingParams[strings.ToLower(key)] {
			filteredQuery[key] = values
		}
	}

	// Sort query parameters for consistent hashing
	var keys []string
	for key := range filteredQuery {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Rebuild query string with sorted parameters
	sortedQuery := url.Values{}
	for _, key := range keys {
		values := filteredQuery[key]
		sort.Strings(values) // Also sort values for each parameter
		sortedQuery[key] = values
	}
	parsedURL.RawQuery = sortedQuery.Encode()

	// Remove trailing slash from path (unless it's just "/")
	if len(parsedURL.Path) > 1 && strings.HasSuffix(parsedURL.Path, "/") {
		parsedURL.Path = strings.TrimSuffix(parsedURL.Path, "/")
	}

	return parsedURL.String(), nil
}

// hashURL creates a SHA256 hash of the normalized URL for use as a cache key
func hashURL(rawURL string) (string, error) {
	normalized, err := normalizeURL(rawURL)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:]), nil
}

// makeKey creates a Redis key from a URL hash
func makeKey(urlHash string) string {
	return KeyPrefix + urlHash
}

// Get retrieves the scraper UUID for a URL from cache
// Returns the scraper UUID if found, empty string if not found
func (c *Cache) Get(ctx context.Context, url string) (string, error) {
	urlHash, err := hashURL(url)
	if err != nil {
		return "", fmt.Errorf("failed to hash URL: %w", err)
	}

	key := makeKey(urlHash)

	scraperUUID, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		// Key doesn't exist - cache miss
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get cache entry: %w", err)
	}

	return scraperUUID, nil
}

// Set stores a URL -> scraper UUID mapping in cache
func (c *Cache) Set(ctx context.Context, url, scraperUUID string) error {
	urlHash, err := hashURL(url)
	if err != nil {
		return fmt.Errorf("failed to hash URL: %w", err)
	}

	key := makeKey(urlHash)

	err = c.client.Set(ctx, key, scraperUUID, CacheTTL).Err()
	if err != nil {
		return fmt.Errorf("failed to set cache entry: %w", err)
	}

	return nil
}

// Delete removes a URL from the cache
func (c *Cache) Delete(ctx context.Context, url string) error {
	urlHash, err := hashURL(url)
	if err != nil {
		return fmt.Errorf("failed to hash URL: %w", err)
	}

	key := makeKey(urlHash)

	err = c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete cache entry: %w", err)
	}

	return nil
}

// Close closes the Redis connection
func (c *Cache) Close() error {
	return c.client.Close()
}

// Ping checks if the Redis connection is alive
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}
