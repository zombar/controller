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
}

// ExtractLinksTaskPayload represents the payload for a link extraction task
type ExtractLinksTaskPayload struct {
	ParentJobID string `json:"parent_job_id"`
	SourceURL   string `json:"source_url"`
	ParentDepth int    `json:"parent_depth"`
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
	// Create task payload
	payload := ScrapeTaskPayload{
		JobID:        jobID,
		URL:          url,
		ExtractLinks: extractLinks,
		ParentJobID:  parentJobID,
		Depth:        depth,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task payload: %w", err)
	}

	// Create Asynq task
	task := asynq.NewTask(TypeScrapeURL, payloadBytes)

	// Task options
	opts := []asynq.Option{
		asynq.MaxRetry(3),                     // Max 3 retries
		asynq.Timeout(10 * time.Minute),       // 10 minute timeout per task
		asynq.Queue("default"),                // Use default queue
		asynq.Retention(7 * 24 * time.Hour),   // Keep completed tasks for 7 days
	}

	// Add tracing context if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		spanCtx := span.SpanContext()
		opts = append(opts, asynq.TaskID(jobID)) // Use job ID as task ID for correlation

		// Record enqueue event
		span.AddEvent("task_enqueued", trace.WithAttributes(
			attribute.String("task.type", TypeScrapeURL),
			attribute.String("task.id", jobID),
			attribute.String("url", url),
			attribute.Bool("extract_links", extractLinks),
		))

		// Store trace context in task (for propagation to worker)
		traceID := spanCtx.TraceID().String()
		spanID := spanCtx.SpanID().String()
		opts = append(opts, asynq.Retention(7*24*time.Hour))

		// Add trace metadata to task
		task = asynq.NewTask(
			TypeScrapeURL,
			payloadBytes,
			asynq.TaskID(jobID),
			asynq.Unique(time.Minute), // Prevent duplicate tasks within 1 minute
		)

		_ = traceID // TODO: Propagate trace context through task metadata
		_ = spanID
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
		asynq.Queue("default"),
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
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal task payload: %w", err)
	}

	task := asynq.NewTask(TypeExtractLinks, payloadBytes)

	opts := []asynq.Option{
		asynq.MaxRetry(2),                  // Retry up to 2 times
		asynq.Timeout(5 * time.Minute),     // Link extraction should be fast
		asynq.Queue("default"),
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
