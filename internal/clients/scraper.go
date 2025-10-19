package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
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
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	Score    *LinkScore             `json:"score,omitempty"` // Quality score for the URL
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
func (c *ScraperClient) Scrape(url string) (*ScraperResponse, error) {
	reqBody := ScraperRequest{URL: url}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/scrape", c.baseURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var scraperResp ScraperResponse
	if err := json.Unmarshal(body, &scraperResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &scraperResp, nil
}

// ImageInfo represents image data from the scraper service
type ImageInfo struct {
	ID         string   `json:"id,omitempty"`
	URL        string   `json:"url"`
	AltText    string   `json:"alt_text"`
	Summary    string   `json:"summary"`
	Tags       []string `json:"tags"`
	Base64Data string   `json:"base64_data,omitempty"`
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
func (c *ScraperClient) SearchImagesByTags(tags []string) (*ImageSearchResponse, error) {
	reqBody := ImageSearchRequest{Tags: tags}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/images/search", c.baseURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp ImageSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &searchResp, nil
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
func (c *ScraperClient) ScoreLink(url string) (*ScoreResponse, error) {
	reqBody := ScoreRequest{URL: url}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/score", c.baseURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var scoreResp ScoreResponse
	if err := json.Unmarshal(body, &scoreResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

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
func (c *ScraperClient) ExtractLinks(url string) (*ExtractLinksResponse, error) {
	reqBody := ExtractLinksRequest{URL: url}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/extract-links", c.baseURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var extractResp ExtractLinksResponse
	if err := json.Unmarshal(body, &extractResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &extractResp, nil
}

// BatchScrapeRequest represents a request to scrape multiple URLs
type BatchScrapeRequest struct {
	URLs  []string `json:"urls"`
	Force bool     `json:"force"`
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
}

// BatchResult represents the result of scraping a single URL in a batch
type BatchResult struct {
	URL     string       `json:"url"`
	Success bool         `json:"success"`
	Data    *ScrapedData `json:"data,omitempty"`
	Error   string       `json:"error,omitempty"`
	Cached  bool         `json:"cached"`
}

// BatchSummary provides summary statistics for a batch scrape operation
type BatchSummary struct {
	Total   int `json:"total"`
	Success int `json:"success"`
	Failed  int `json:"failed"`
	Cached  int `json:"cached"`
	Scraped int `json:"scraped"`
}

// BatchScrapeResponse represents the response from a batch scrape operation
type BatchScrapeResponse struct {
	Results []BatchResult `json:"results"`
	Summary BatchSummary  `json:"summary"`
}

// BatchScrape scrapes multiple URLs using the scraper service
func (c *ScraperClient) BatchScrape(urls []string, force bool) (*BatchScrapeResponse, error) {
	reqBody := BatchScrapeRequest{URLs: urls, Force: force}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/scrape/batch", c.baseURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scraper: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scraper service returned status %d: %s", resp.StatusCode, string(body))
	}

	var batchResp BatchScrapeResponse
	if err := json.Unmarshal(body, &batchResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &batchResp, nil
}
