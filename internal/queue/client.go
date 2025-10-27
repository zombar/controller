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
	TypeScrapeURL    = "scrape:url"
	TypeExtractLinks = "extract:links"
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
			attribute.String("task.id", jobID),
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
		asynq.MaxRetry(3),                     // Max 3 retries
		asynq.Timeout(10 * time.Minute),       // 10 minute timeout per task
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
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Minute),
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
		asynq.MaxRetry(2),                  // Retry up to 2 times
		asynq.Timeout(5 * time.Minute),     // Link extraction should be fast
		asynq.Queue("link-extraction"),     // Link extraction queue (lower priority)
		asynq.ProcessIn(1 * time.Second),   // Small delay to ensure parent task fully completes
	}

	info, err := c.client.Enqueue(task, opts...)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue extract links task: %w", err)
	}

	return info.ID, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.client.Close()
}
