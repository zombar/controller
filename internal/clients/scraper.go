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
