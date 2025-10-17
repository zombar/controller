package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TextAnalyzerClient handles communication with the text analyzer service
type TextAnalyzerClient struct {
	baseURL    string
	httpClient *http.Client
}

// TextAnalyzerRequest represents a request to the text analyzer service
type TextAnalyzerRequest struct {
	Text string `json:"text"`
}

// TextAnalyzerResponse represents a response from the text analyzer service
type TextAnalyzerResponse struct {
	UUID     string                 `json:"uuid"`
	Tags     []string               `json:"tags"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewTextAnalyzerClient creates a new text analyzer client
func NewTextAnalyzerClient(baseURL string) *TextAnalyzerClient {
	return &TextAnalyzerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Analyze sends text to the analyzer service and returns the response
func (c *TextAnalyzerClient) Analyze(text string) (*TextAnalyzerResponse, error) {
	reqBody := TextAnalyzerRequest{Text: text}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/analyze", c.baseURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to text analyzer: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("text analyzer service returned status %d: %s", resp.StatusCode, string(body))
	}

	var analyzerResp TextAnalyzerResponse
	if err := json.Unmarshal(body, &analyzerResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &analyzerResp, nil
}
