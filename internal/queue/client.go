package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Task type constants
const (
	TypeScrapeURL        = "scrape:url"
	TypeExtractLinks     = "extract:links"
	TypeRetrieveAnalysis = "retrieve:analysis"
)

// ScrapeTaskPayload represents the payload for a scrape task
type ScrapeTaskPayload struct {
	JobID        string  `json:"job_id"`
	URL          string  `json:"url"`
	ExtractLinks bool    `json:"extract_links"`
	ParentJobID  *string `json:"parent_job_id,omitempty"`
	Depth        int     `json:"depth"`
	// Tracing and timing fields
	TraceID    string `json:"trace_id,omitempty"`
	SpanID     string `json:"span_id,omitempty"`
	EnqueuedAt int64  `json:"enqueued_at"` // Unix timestamp in nanoseconds
}

// ExtractLinksTaskPayload represents the payload for a link extraction task
type ExtractLinksTaskPayload struct {
	ParentJobID string `json:"parent_job_id"`
	SourceURL   string `json:"source_url"`
	ParentDepth int    `json:"parent_depth"`
	// Tracing and timing fields
	TraceID    string `json:"trace_id,omitempty"`
	SpanID     string `json:"span_id,omitempty"`
	EnqueuedAt int64  `json:"enqueued_at"` // Unix timestamp in nanoseconds
}

// RetrieveAnalysisTaskPayload represents the payload for retrieving text analysis results
type RetrieveAnalysisTaskPayload struct {
	RequestID     string `json:"request_id"`      // The request ID to update
	AnalysisJobID string `json:"analysis_job_id"` // The TextAnalyzer job ID to poll
	AttemptCount  int    `json:"attempt_count"`   // Current retry attempt (for logging)
	// Tracing and timing fields
	TraceID    string `json:"trace_id,omitempty"`
	SpanID     string `json:"span_id,omitempty"`
	EnqueuedAt int64  `json:"enqueued_at"` // Unix timestamp in nanoseconds
}

// Client wraps the Asynq client for enqueueing tasks
type Client struct {
	client *asynq.Client
	tracer trace.Tracer
}

// ClientConfig contains configuration for the queue client
type ClientConfig struct {
	RedisAddr string
}

// NewClient creates a new queue client
func NewClient(cfg ClientConfig) *Client {
	redisOpt := asynq.RedisClientOpt{
		Addr: cfg.RedisAddr,
	}

	client := asynq.NewClient(redisOpt)

	return &Client{
		client: client,
	}
}

// EnqueueScrape enqueues a scrape job to the queue
func (c *Client) EnqueueScrape(ctx context.Context, jobID, url string, extractLinks bool) (string, error) {
	return c.EnqueueScrapeWithParent(ctx, jobID, url, extractLinks, nil, 0)
}

// EnqueueScrapeWithParent enqueues a scrape job with parent and depth tracking
func (c *Client) EnqueueScrapeWithParent(ctx context.Context, jobID, url string, extractLinks bool, parentJobID *string, depth int) (string, error) {
	// Create task payload with trace context
	payload := ScrapeTaskPayload{
		JobID:        jobID,
		URL:          url,
		ExtractLinks: extractLinks,
		ParentJobID:  parentJobID,
		Depth:        depth,
		EnqueuedAt:   time.Now().UnixNano(), // Record enqueue time for queue wait metrics
	}

	// Add tracing context if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		payload.TraceID = spanCtx.TraceID().String()
		payload.SpanID = spanCtx.SpanID().String()

		// Record enqueue event
		span.AddEvent("task_enqueued", trace.WithAttributes(
			attribute.String("task.type", TypeScrapeURL),
			attribute.String("scrape_request_id", jobID),
			attribute.String("url", url),
			attribute.Bool("extract_links", extractLinks),
			attribute.Int64("enqueued_at", payload.EnqueuedAt),
		))
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task payload: %w", err)
	}

	// Create Asynq task
	task := asynq.NewTask(TypeScrapeURL, payloadBytes)

	// Task options
	opts := []asynq.Option{
		asynq.TaskID(jobID),                   // Use job ID as task ID for correlation
		asynq.MaxRetry(12),                    // Max 12 retries over 24 hours
		asynq.Timeout(3 * time.Hour),          // 3 hour timeout per task (handles service overload scenarios)
		asynq.Queue("scrape"),                 // Scrape queue (high priority)
		asynq.Retention(7 * 24 * time.Hour),   // Keep completed tasks for 7 days
		asynq.Unique(time.Minute),             // Prevent duplicate tasks within 1 minute
	}

	// Enqueue the task
	info, err := c.client.Enqueue(task, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	return info.ID, nil
}

// EnqueueScrapeWithDelay enqueues a scrape job with a delay
func (c *Client) EnqueueScrapeWithDelay(ctx context.Context, jobID, url string, extractLinks bool, delay time.Duration) (string, error) {
	payload := ScrapeTaskPayload{
		JobID:        jobID,
		URL:          url,
		ExtractLinks: extractLinks,
		EnqueuedAt:   time.Now().UnixNano(),
	}

	// Add tracing context if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		payload.TraceID = spanCtx.TraceID().String()
		payload.SpanID = spanCtx.SpanID().String()
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(TypeScrapeURL, payloadBytes, asynq.TaskID(jobID))

	opts := []asynq.Option{
		asynq.ProcessIn(delay),              // Delay execution
		asynq.MaxRetry(12),                  // Max 12 retries over 24 hours
		asynq.Timeout(3 * time.Hour),        // 3 hour timeout per task
		asynq.Queue("scrape"),               // Scrape queue (high priority)
	}

	info, err := c.client.Enqueue(task, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue delayed task: %w", err)
	}

	return info.ID, nil
}

// EnqueueExtractLinks enqueues a link extraction task
func (c *Client) EnqueueExtractLinks(ctx context.Context, parentJobID, sourceURL string, parentDepth int) (string, error) {
	payload := ExtractLinksTaskPayload{
		ParentJobID: parentJobID,
		SourceURL:   sourceURL,
		ParentDepth: parentDepth,
		EnqueuedAt:  time.Now().UnixNano(),
	}

	// Add tracing context if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		payload.TraceID = spanCtx.TraceID().String()
		payload.SpanID = spanCtx.SpanID().String()

		// Record enqueue event
		span.AddEvent("link_extraction_enqueued", trace.WithAttributes(
			attribute.String("task.type", TypeExtractLinks),
			attribute.String("parent_job_id", parentJobID),
			attribute.String("source_url", sourceURL),
		))
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(TypeExtractLinks, payloadBytes)

	opts := []asynq.Option{
		asynq.MaxRetry(12),                 // Max 12 retries over 24 hours
		asynq.Timeout(1 * time.Hour),       // 1 hour timeout for link extraction
		asynq.Queue("link-extraction"),     // Link extraction queue (lower priority)
		asynq.ProcessIn(1 * time.Second),   // Small delay to ensure parent task fully completes
	}

	info, err := c.client.Enqueue(task, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue extract links task: %w", err)
	}

	return info.ID, nil
}

// EnqueueRetrieveAnalysis enqueues a task to retrieve text analysis results from TextAnalyzer
// First attempt is delayed by 30 seconds, subsequent retries use exponential backoff up to 24 hours
func (c *Client) EnqueueRetrieveAnalysis(ctx context.Context, requestID, analysisJobID string, attemptCount int) (string, error) {
	payload := RetrieveAnalysisTaskPayload{
		RequestID:     requestID,
		AnalysisJobID: analysisJobID,
		AttemptCount:  attemptCount,
		EnqueuedAt:    time.Now().UnixNano(),
	}

	// Add tracing context if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		payload.TraceID = spanCtx.TraceID().String()
		payload.SpanID = spanCtx.SpanID().String()

		// Record enqueue event
		span.AddEvent("analysis_retrieval_enqueued", trace.WithAttributes(
			attribute.String("task.type", TypeRetrieveAnalysis),
			attribute.String("request_id", requestID),
			attribute.String("analysis_job_id", analysisJobID),
			attribute.Int("attempt_count", attemptCount),
		))
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(TypeRetrieveAnalysis, payloadBytes)

	// Calculate delay based on attempt count
	// Delays: 30s, 2m, 5m, 10m, 20m, 40m, 1h, 2h, 4h, 8h (total: ~15 attempts over 24 hours)
	var delay time.Duration
	if attemptCount == 0 {
		delay = 30 * time.Second
	} else {
		// Exponential backoff: 2m, 5m, 10m, 20m, 40m, 1h, 2h, 4h, 8h
		delays := []time.Duration{
			2 * time.Minute,
			5 * time.Minute,
			10 * time.Minute,
			20 * time.Minute,
			40 * time.Minute,
			1 * time.Hour,
			2 * time.Hour,
			4 * time.Hour,
			8 * time.Hour,
		}
		if attemptCount-1 < len(delays) {
			delay = delays[attemptCount-1]
		} else {
			delay = 8 * time.Hour // Cap at 8 hours
		}
	}

	opts := []asynq.Option{
		asynq.ProcessIn(delay),              // Delay for exponential backoff
		asynq.MaxRetry(12),                  // Max 12 retries over 24 hours
		asynq.Timeout(3 * time.Hour),        // 3 hour timeout - includes waiting for AI processing (Ollama)
		asynq.Queue("analysis-retrieval"),   // Analysis retrieval queue (medium priority)
		asynq.Retention(7 * 24 * time.Hour), // Keep completed tasks for 7 days
	}

	info, err := c.client.Enqueue(task, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue retrieve analysis task: %w", err)
	}

	return info.ID, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.client.Close()
}
