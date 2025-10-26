package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// ScraperClient handles communication with the scraper service
type ScraperClient struct {
	baseURL    string
	httpClient *http.Client
}

// ScraperRequest represents a request to the scraper service
type ScraperRequest struct {
	URL string `json:"url"`
}

// ScraperResponse represents a response from the scraper service
type ScraperResponse struct {
	ID       string                 `json:"id"`
	URL      string                 `json:"url"`
	Title    string                 `json:"title"`
	Content  string                 `json:"content"`       // AI-cleaned content
	RawText  string                 `json:"raw_text"`      // Original raw text extracted from HTML
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Score    *LinkScore             `json:"score,omitempty"` // Quality score for the URL
	Slug     string                 `json:"slug,omitempty"`  // SEO-friendly URL slug
}

// NewScraperClient creates a new scraper client
func NewScraperClient(baseURL string) *ScraperClient {
	return &ScraperClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // Web scraping can take several minutes
		},
	}
}

// Scrape sends a URL to the scraper service and returns the response
func (c *ScraperClient) Scrape(ctx context.Context, url string) (*ScraperResponse, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.Scrape")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.url", url),
		attribute.String("http.method", "POST"),
		attribute.String("http.url", fmt.Sprintf("%s/api/scrape", c.baseURL)),
	)

	reqBody := ScraperRequest{URL: url}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/scrape", c.baseURL),
		bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var scraperResp ScraperResponse
	if err := json.Unmarshal(body, &scraperResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetStatus(codes.Ok, "success")
	return &scraperResp, nil
}

// ImageInfo represents image data from the scraper service
type ImageInfo struct {
	ID                string     `json:"id,omitempty"`
	URL               string     `json:"url"`
	AltText           string     `json:"alt_text"`
	Summary           string     `json:"summary"`
	Tags              []string   `json:"tags"`
	Base64Data        string     `json:"base64_data,omitempty"`
	ScraperUUID       string     `json:"scraper_uuid,omitempty"`
	TombstoneDatetime *time.Time `json:"tombstone_datetime,omitempty"`
	FilePath          string     `json:"file_path,omitempty"` // Filesystem path for the image
	Slug              string     `json:"slug,omitempty"`      // SEO-friendly URL slug
}

// ImageSearchRequest represents a request to search images by tags
type ImageSearchRequest struct {
	Tags []string `json:"tags"`
}

// ImageSearchResponse represents the response from image search
type ImageSearchResponse struct {
	Images []*ImageInfo `json:"images"`
	Count  int          `json:"count"`
}

// SearchImagesByTags searches for images by tags using the scraper service
func (c *ScraperClient) SearchImagesByTags(ctx context.Context, tags []string) (*ImageSearchResponse, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.SearchImagesByTags")
	defer span.End()

	span.SetAttributes(
		attribute.StringSlice("scraper.tags", tags),
		attribute.String("http.method", "POST"),
	)

	reqBody := ImageSearchRequest{Tags: tags}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/images/search", c.baseURL),
		bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp ImageSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetAttributes(attribute.Int("scraper.image_count", searchResp.Count))
	span.SetStatus(codes.Ok, "success")
	return &searchResp, nil
}

// GetImagesByScrapeID retrieves images associated with a specific scrape ID
func (c *ScraperClient) GetImagesByScrapeID(ctx context.Context, scrapeID string) (*ImageSearchResponse, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.GetImagesByScrapeID")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.scrape_id", scrapeID),
		attribute.String("http.method", "GET"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/scrapes/%s/images", c.baseURL, scrapeID),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp ImageSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetAttributes(attribute.Int("scraper.image_count", searchResp.Count))
	span.SetStatus(codes.Ok, "success")
	return &searchResp, nil
}

// GetImageByID retrieves a single image by ID from the scraper service
func (c *ScraperClient) GetImageByID(ctx context.Context, imageID string) (*ImageInfo, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.GetImageByID")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.image_id", imageID),
		attribute.String("http.method", "GET"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/images/%s", c.baseURL, imageID),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var image ImageInfo
	if err := json.Unmarshal(body, &image); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetStatus(codes.Ok, "success")
	return &image, nil
}

// LinkScore represents a scored link with quality assessment
type LinkScore struct {
	URL                 string   `json:"url"`
	Score               float64  `json:"score"`
	Reason              string   `json:"reason"`
	Categories          []string `json:"categories"`
	IsRecommended       bool     `json:"is_recommended"`
	MaliciousIndicators []string `json:"malicious_indicators,omitempty"`
	AIUsed              bool     `json:"ai_used"`
}

// ScoreRequest represents a request to score a URL
type ScoreRequest struct {
	URL string `json:"url"`
}

// ScoreResponse represents a response containing link score
type ScoreResponse struct {
	URL   string    `json:"url"`
	Score LinkScore `json:"score"`
}

// ScoreLink scores a URL using the scraper service
func (c *ScraperClient) ScoreLink(ctx context.Context, url string) (*ScoreResponse, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.ScoreLink")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.url", url),
		attribute.String("http.method", "POST"),
	)

	reqBody := ScoreRequest{URL: url}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/score", c.baseURL),
		bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "request failed")
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var scoreResp ScoreResponse
	if err := json.Unmarshal(body, &scoreResp); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetAttributes(
		attribute.Float64("scraper.score", scoreResp.Score.Score),
		attribute.Bool("scraper.is_recommended", scoreResp.Score.IsRecommended),
	)
	span.SetStatus(codes.Ok, "success")
	return &scoreResp, nil
}

// ExtractLinksRequest represents a request to extract links from a URL
type ExtractLinksRequest struct {
	URL string `json:"url"`
}

// ExtractLinksResponse represents the response from extracting links
type ExtractLinksResponse struct {
	URL   string   `json:"url"`
	Links []string `json:"links"`
	Count int      `json:"count"`
}

// ExtractLinks extracts links from a URL using the scraper service
func (c *ScraperClient) ExtractLinks(ctx context.Context, url string) (*ExtractLinksResponse, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.ExtractLinks")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.url", url),
		attribute.String("http.method", "POST"),
	)

	reqBody := ExtractLinksRequest{URL: url}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/extract-links", c.baseURL),
		bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var extractResp ExtractLinksResponse
	if err := json.Unmarshal(body, &extractResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetAttributes(attribute.Int("scraper.link_count", extractResp.Count))
	span.SetStatus(codes.Ok, "success")
	return &extractResp, nil
}

// DeleteScrape deletes a scrape by ID
func (c *ScraperClient) DeleteScrape(ctx context.Context, scrapeID string) error {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.DeleteScrape")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.scrape_id", scrapeID),
		attribute.String("http.method", "DELETE"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/scrapes/%s", c.baseURL, scrapeID),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	span.SetStatus(codes.Ok, "success")
	return nil
}

// DeleteImage deletes an image by ID
func (c *ScraperClient) DeleteImage(ctx context.Context, imageID string) error {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.DeleteImage")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.image_id", imageID),
		attribute.String("http.method", "DELETE"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/images/%s", c.baseURL, imageID),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	span.SetStatus(codes.Ok, "success")
	return nil
}

// TombstoneImage tombstones an image by ID
func (c *ScraperClient) TombstoneImage(ctx context.Context, imageID string) error {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.TombstoneImage")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.image_id", imageID),
		attribute.String("http.method", "PUT"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/api/images/%s/tombstone", c.baseURL, imageID),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	span.SetStatus(codes.Ok, "success")
	return nil
}

// UntombstoneImage removes tombstone from an image by ID
func (c *ScraperClient) UntombstoneImage(ctx context.Context, imageID string) error {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.UntombstoneImage")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.image_id", imageID),
		attribute.String("http.method", "DELETE"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/images/%s/tombstone", c.baseURL, imageID),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	span.SetStatus(codes.Ok, "success")
	return nil
}

// PageMetadata represents metadata about a scraped page
type PageMetadata struct {
	Description   string   `json:"description,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	Author        string   `json:"author,omitempty"`
	PublishedDate string   `json:"published_date,omitempty"`
}

// ScrapedData represents the complete scraped data for a URL
type ScrapedData struct {
	ID             string       `json:"id"`
	URL            string       `json:"url"`
	Title          string       `json:"title"`
	Content        string       `json:"content"`
	Images         []ImageInfo  `json:"images"`
	Links          []string     `json:"links"`
	FetchedAt      time.Time    `json:"fetched_at"`
	CreatedAt      time.Time    `json:"created_at"`
	ProcessingTime float64      `json:"processing_time_seconds"`
	Cached         bool         `json:"cached"`
	Metadata       PageMetadata `json:"metadata"`
	Score          *LinkScore   `json:"score,omitempty"`
	Slug           string       `json:"slug,omitempty"` // SEO-friendly URL slug
}

// UpdateImageTags updates the tags for an image by ID
func (c *ScraperClient) UpdateImageTags(ctx context.Context, imageID string, tags []string) error {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scraper.UpdateImageTags")
	defer span.End()

	span.SetAttributes(
		attribute.String("scraper.image_id", imageID),
		attribute.StringSlice("scraper.tags", tags),
		attribute.String("http.method", "PUT"),
	)

	reqBody := struct {
		Tags []string `json:"tags"`
	}{Tags: tags}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal request")
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/api/images/%s/tags", c.baseURL, imageID),
		bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	span.SetStatus(codes.Ok, "success")
	return nil
}
