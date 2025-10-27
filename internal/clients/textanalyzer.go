package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// TextAnalyzerClient handles communication with the text analyzer service
type TextAnalyzerClient struct {
	baseURL    string
	httpClient *http.Client
}

// TextAnalyzerRequest represents a request to the text analyzer service
type TextAnalyzerRequest struct {
	Text         string   `json:"text"`
	OriginalHTML string   `json:"original_html,omitempty"` // Compressed + base64 encoded original HTML/raw text
	Images       []string `json:"images,omitempty"`
}

// TextAnalyzerResponse represents a response from the text analyzer service
type TextAnalyzerResponse struct {
	ID       string                 `json:"id"`
	Metadata map[string]interface{} `json:"metadata"`
}

// TextAnalyzerQueueResponse represents the initial queue response
type TextAnalyzerQueueResponse struct {
	JobID   string `json:"job_id"`
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// NormalizeTag ensures a tag is at most double-barrelled (max one hyphen)
// Examples: "machine-learning" stays as is, "machine-learning-model" becomes "machine-learning"
func NormalizeTag(tag string) string {
	parts := strings.Split(tag, "-")
	if len(parts) <= 2 {
		return tag
	}
	return strings.Join(parts[:2], "-")
}

// GetTags extracts tags from the metadata and normalizes them to be at most double-barrelled
func (r *TextAnalyzerResponse) GetTags() []string {
	if r.Metadata == nil {
		return []string{}
	}
	if tags, ok := r.Metadata["tags"].([]interface{}); ok {
		result := make([]string, 0, len(tags))
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok {
				normalized := NormalizeTag(tagStr)
				result = append(result, normalized)
			}
		}
		return result
	}
	return []string{}
}

// NewTextAnalyzerClient creates a new text analyzer client
func NewTextAnalyzerClient(baseURL string) *TextAnalyzerClient {
	return &TextAnalyzerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Minute, // AI analysis can take several minutes
			Transport: otelhttp.NewTransport(http.DefaultTransport), // Inject trace context headers
		},
	}
}

// EnqueueAnalysis enqueues text, original HTML, and images for analysis and returns the job ID
func (c *TextAnalyzerClient) EnqueueAnalysis(ctx context.Context, text, originalHTML string, images []string) (string, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "textanalyzer.EnqueueAnalysis")
	defer span.End()

	span.SetAttributes(
		attribute.Int("textanalyzer.text_length", len(text)),
		attribute.Bool("textanalyzer.has_original_html", originalHTML != ""),
		attribute.Int("textanalyzer.image_count", len(images)),
		attribute.String("http.method", "POST"),
	)

	reqBody := TextAnalyzerRequest{
		Text:         text,
		OriginalHTML: originalHTML,
		Images:       images,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal request")
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/analyze", c.baseURL),
		bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return "", fmt.Errorf("failed to send request to text analyzer: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return "", fmt.Errorf("text analyzer service returned status %d: %s", resp.StatusCode, string(body))
	}

	var queueResp TextAnalyzerQueueResponse
	if err := json.Unmarshal(body, &queueResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetAttributes(
		attribute.String("textanalyzer.job_id", queueResp.JobID),
		attribute.String("textanalyzer.status", queueResp.Status),
	)
	span.SetStatus(codes.Ok, "success")
	return queueResp.JobID, nil
}

// Analyze sends text to the analyzer service and returns the response (DEPRECATED - use EnqueueAnalysis)
// This is kept for backwards compatibility but will be removed in future versions
func (c *TextAnalyzerClient) Analyze(ctx context.Context, text string) (*TextAnalyzerResponse, error) {
	// For backwards compatibility, enqueue and immediately return empty response
	// The actual analysis will happen asynchronously via the queue
	jobID, err := c.EnqueueAnalysis(ctx, text, "", nil)
	if err != nil {
		return nil, err
	}

	// Return minimal response with job ID
	return &TextAnalyzerResponse{
		ID: jobID,
		Metadata: map[string]interface{}{
			"status":  "queued",
			"message": "Analysis queued for processing",
		},
	}, nil
}

// DeleteAnalysis deletes an analysis by ID
func (c *TextAnalyzerClient) DeleteAnalysis(ctx context.Context, analysisID string) error {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "textanalyzer.DeleteAnalysis")
	defer span.End()

	span.SetAttributes(
		attribute.String("textanalyzer.analysis_id", analysisID),
		attribute.String("http.method", "DELETE"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/analyses/%s", c.baseURL, analysisID),
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
		return fmt.Errorf("failed to send request to text analyzer: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return fmt.Errorf("text analyzer service returned status %d: %s", resp.StatusCode, string(body))
	}

	span.SetStatus(codes.Ok, "success")
	return nil
}
