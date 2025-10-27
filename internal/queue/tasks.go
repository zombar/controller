package queue

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zombar/controller/internal/clients"
	internalslug "github.com/zombar/controller/internal/slug"
	"github.com/zombar/controller/internal/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// handleScrapeTask processes a scrape URL task
func (w *Worker) handleScrapeTask(ctx context.Context, t *asynq.Task) error {
	// Parse payload
	var payload ScrapeTaskPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		w.logger.Error("failed to unmarshal task payload", "error", err)
		return fmt.Errorf("invalid task payload: %w", err)
	}

	jobID := payload.JobID
	url := payload.URL
	extractLinks := payload.ExtractLinks

	// Calculate queue wait time
	var queueWaitTime time.Duration
	if payload.EnqueuedAt > 0 {
		enqueuedTime := time.Unix(0, payload.EnqueuedAt)
		queueWaitTime = time.Since(enqueuedTime)
	}

	w.logger.Info("processing scrape task",
		"job_id", jobID,
		"url", url,
		"extract_links", extractLinks,
		"queue_wait_seconds", queueWaitTime.Seconds(),
	)

	// Recreate trace context from payload if available
	var span trace.Span
	if payload.TraceID != "" && payload.SpanID != "" {
		// Parse trace ID and span ID from hex strings
		traceID, err := trace.TraceIDFromHex(payload.TraceID)
		if err == nil {
			spanID, err := trace.SpanIDFromHex(payload.SpanID)
			if err == nil {
				// Create span context from stored IDs
				remoteSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    traceID,
					SpanID:     spanID,
					TraceFlags: trace.FlagsSampled,
					Remote:     true,
				})

				// Create new context with the remote span context
				ctx = trace.ContextWithRemoteSpanContext(ctx, remoteSpanCtx)

				// Start a new span linked to the enqueue span
				ctx, span = otel.Tracer("controller").Start(ctx, "asynq.task.process",
					trace.WithSpanKind(trace.SpanKindConsumer),
					trace.WithAttributes(
						attribute.String("task.type", TypeScrapeURL),
						attribute.String("task.id", jobID),
						attribute.String("job.id", jobID),
						attribute.String("job.url", url),
						attribute.Bool("job.extract_links", extractLinks),
						attribute.Float64("queue.wait_time_seconds", queueWaitTime.Seconds()),
						attribute.Int64("enqueued_at", payload.EnqueuedAt),
					),
				)
				defer span.End()

				// Record queue wait time event
				span.AddEvent("task_processing_started", trace.WithAttributes(
					attribute.Float64("wait_time_seconds", queueWaitTime.Seconds()),
				))
			}
		}
	} else {
		// No trace context in payload, check current context
		if existingSpan := trace.SpanFromContext(ctx); existingSpan.SpanContext().IsValid() {
			existingSpan.SetAttributes(
				attribute.String("job.id", jobID),
				attribute.String("job.url", url),
				attribute.Bool("job.extract_links", extractLinks),
				attribute.Float64("queue.wait_time_seconds", queueWaitTime.Seconds()),
			)
		}
	}

	// Update job status to processing
	if err := w.storage.UpdateScrapeJobStatus(jobID, "processing", ""); err != nil {
		w.logger.Error("failed to update job status", "job_id", jobID, "error", err)
		// Continue processing even if status update fails
	}

	// Execute the scrape workflow
	err := w.processScrape(ctx, jobID, url, extractLinks)
	if err != nil {
		// Update job status to failed
		errMsg := err.Error()
		if updateErr := w.storage.UpdateScrapeJobStatus(jobID, "failed", errMsg); updateErr != nil {
			w.logger.Error("failed to update job status to failed", "job_id", jobID, "error", updateErr)
		}

		// Increment retry count
		if retryErr := w.storage.IncrementScrapeJobRetries(jobID); retryErr != nil {
			w.logger.Error("failed to increment retries", "job_id", jobID, "error", retryErr)
		}

		w.logger.Error("scrape task failed", "job_id", jobID, "error", err)
		return err // Asynq will retry
	}

	w.logger.Info("scrape task completed", "job_id", jobID)
	return nil
}

// processScrape contains the main scraping logic
func (w *Worker) processScrape(ctx context.Context, jobID, url string, extractLinks bool) error {
	// Score the URL first
	scoreResp, err := w.scraperClient.ScoreLink(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to score link: %w", err)
	}

	// Check if this is an image URL (skip threshold check for images)
	isImageURL := false
	for _, category := range scoreResp.Score.Categories {
		if category == "image" {
			isImageURL = true
			break
		}
	}

	// Check score threshold (skip for image URLs)
	if !isImageURL && scoreResp.Score.Score < w.linkScoreThreshold {
		// Save a tombstoned record for low-quality content
		tombstoneTime := time.Now().UTC().Add(time.Duration(w.tombstonePeriodLowScore) * 24 * time.Hour)
		requestID := uuid.New().String()

		// Add domain name to tags, normalizing categories
		tags := make([]string, 0, len(scoreResp.Score.Categories))
		for _, cat := range scoreResp.Score.Categories {
			tags = append(tags, clients.NormalizeTag(cat))
		}
		if domain := extractDomainTag(url); domain != "" {
			tags = append(tags, domain)
		}

		// Add 'scrape' tag to all scraped content
		tags = append(tags, "scrape")

		record := &storage.Request{
			ID:         requestID,
			CreatedAt:  time.Now().UTC(),
			SourceType: "url",
			SourceURL:  &url,
			Tags:       tags,
			SEOEnabled: false, // Disable SEO for below-threshold content
			Metadata: map[string]interface{}{
				"link_score": map[string]interface{}{
					"score":                scoreResp.Score.Score,
					"reason":               scoreResp.Score.Reason,
					"categories":           scoreResp.Score.Categories,
					"is_recommended":       scoreResp.Score.IsRecommended,
					"malicious_indicators": scoreResp.Score.MaliciousIndicators,
				},
				"below_threshold":    true,
				"threshold":          w.linkScoreThreshold,
				"tombstone_datetime": tombstoneTime.Format(time.RFC3339),
			},
		}

		if err := w.storage.SaveRequest(record); err != nil {
			return fmt.Errorf("failed to save low-quality record: %w", err)
		}

		// Update job with result
		if err := w.storage.UpdateScrapeJobResult(jobID, requestID); err != nil {
			return fmt.Errorf("failed to update job result: %w", err)
		}

		// Record tombstone metrics
		if w.businessMetrics != nil {
			w.businessMetrics.TombstonesCreatedTotal.WithLabelValues("low-score", "none").Inc()
			w.businessMetrics.TombstoneDaysHistogram.WithLabelValues("low-score").Observe(float64(w.tombstonePeriodLowScore))
		}

		log.Printf("Low-quality URL marked for tombstoning: %s (score: %.2f, threshold: %.2f)",
			url, scoreResp.Score.Score, w.linkScoreThreshold)
		return nil
	}

	// Scrape the URL
	scrapeResp, err := w.scraperClient.Scrape(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to scrape: %w", err)
	}

	// Build scraper metadata
	scraperMetadata := make(map[string]interface{})
	scraperMetadata["title"] = scrapeResp.Title
	scraperMetadata["content"] = scrapeResp.Content
	scraperMetadata["raw_text"] = scrapeResp.RawText
	scraperMetadata["url"] = scrapeResp.URL

	// Include fields from the scraper's Metadata
	if scrapeResp.Metadata != nil {
		for k, v := range scrapeResp.Metadata {
			scraperMetadata[k] = v
		}
	}

	// Extract image URLs from scraper response for textanalyzer
	images := make([]string, 0, len(scrapeResp.Images))
	for _, img := range scrapeResp.Images {
		images = append(images, img.URL)
	}

	// Enqueue text analysis (skip for image URLs)
	var textAnalyzerJobID string
	if !isImageURL {
		// Compress the raw text for storage and AI enrichment
		compressedRawText, err := compressHTML(scrapeResp.RawText)
		if err != nil {
			log.Printf("Warning: failed to compress raw text for %s: %v", url, err)
			compressedRawText = "" // Continue without compressed HTML
		}

		jobID, err := w.textAnalyzerClient.EnqueueAnalysis(ctx, scrapeResp.Content, compressedRawText, images)
		if err != nil {
			// Log error but don't fail the scrape - analysis can be retried later
			log.Printf("Warning: failed to enqueue text analysis for %s: %v", url, err)
		} else {
			textAnalyzerJobID = jobID
			log.Printf("Enqueued text analysis job %s for %s (images: %d, compressed HTML: %v)",
				jobID, url, len(images), compressedRawText != "")
		}
	}

	// Combine metadata
	combinedMetadata := make(map[string]interface{})
	combinedMetadata["scraper_metadata"] = scraperMetadata
	if textAnalyzerJobID != "" {
		combinedMetadata["textanalyzer_job_id"] = textAnalyzerJobID
		combinedMetadata["textanalyzer_status"] = "queued"
	}

	// Add link score
	if scrapeResp.Score != nil {
		combinedMetadata["link_score"] = map[string]interface{}{
			"score":                scrapeResp.Score.Score,
			"reason":               scrapeResp.Score.Reason,
			"categories":           scrapeResp.Score.Categories,
			"is_recommended":       scrapeResp.Score.IsRecommended,
			"malicious_indicators": scrapeResp.Score.MaliciousIndicators,
		}
	} else {
		combinedMetadata["link_score"] = map[string]interface{}{
			"score":                scoreResp.Score.Score,
			"reason":               scoreResp.Score.Reason,
			"categories":           scoreResp.Score.Categories,
			"is_recommended":       scoreResp.Score.IsRecommended,
			"malicious_indicators": scoreResp.Score.MaliciousIndicators,
		}
	}

	// Save to database
	requestID := uuid.New().String()

	// Get initial tags from link score categories (normalized)
	// Analyzer tags will be added later when textanalyzer completes
	var tags []string
	if scrapeResp.Score != nil {
		tags = make([]string, 0, len(scrapeResp.Score.Categories))
		for _, cat := range scrapeResp.Score.Categories {
			tags = append(tags, clients.NormalizeTag(cat))
		}
	} else if scoreResp != nil {
		tags = make([]string, 0, len(scoreResp.Score.Categories))
		for _, cat := range scoreResp.Score.Categories {
			tags = append(tags, clients.NormalizeTag(cat))
		}
	}

	// Add domain name to tags
	if domain := extractDomainTag(url); domain != "" {
		tags = append(tags, domain)
	}

	// Add 'scrape' tag to all scraped content
	tags = append(tags, "scrape")

	// Extract slug from scraper response if available
	var slug *string
	if scrapeResp.Slug != "" {
		slug = &scrapeResp.Slug
	} else {
		// Generate slug from title or URL
		slugSource := scrapeResp.Title
		if slugSource == "" {
			slugSource = url
		}
		generatedSlug := internalslug.GenerateWithFallback(slugSource, requestID)
		slug = &generatedSlug
	}

	req := &storage.Request{
		ID:               requestID,
		CreatedAt:        time.Now(),
		SourceType:       "url",
		SourceURL:        &url,
		ScraperUUID:      &scrapeResp.ID,
		TextAnalyzerUUID: textAnalyzerJobID, // Store the job ID for async tracking
		Tags:             tags,
		Metadata:         combinedMetadata,
		Slug:             slug,
		SEOEnabled:       true, // Enable SEO by default
	}

	if err := w.storage.SaveRequest(req); err != nil {
		return fmt.Errorf("failed to save request: %w", err)
	}

	// Update job with result
	if err := w.storage.UpdateScrapeJobResult(jobID, requestID); err != nil {
		return fmt.Errorf("failed to update job result: %w", err)
	}

	log.Printf("Scrape job %s completed successfully, result saved as %s", jobID, requestID)

	// Populate URL cache with scraper UUID for 30-day caching
	if w.urlCache != nil && scrapeResp.ID != "" {
		if err := w.urlCache.Set(ctx, url, scrapeResp.ID); err != nil {
			// Log error but don't fail the task
			w.logger.Warn("failed to populate URL cache", "url", url, "scraper_uuid", scrapeResp.ID, "error", err)
		} else {
			w.logger.Info("URL cached for 30 days", "url", url, "scraper_uuid", scrapeResp.ID)
		}
	}

	// Extract links if requested (skip for image URLs)
	if extractLinks && !isImageURL {
		// Get current job to check depth
		job, err := w.storage.GetScrapeJob(jobID)
		if err != nil {
			log.Printf("Failed to get job for link extraction: %v", err)
		} else if job != nil && job.Depth < w.maxLinkDepth {
			log.Printf("Queueing link extraction task for %s (depth: %d/%d)", url, job.Depth, w.maxLinkDepth)
			// Enqueue link extraction as a separate task to avoid context cancellation issues
			if w.queueClient != nil {
				_, err := w.queueClient.EnqueueExtractLinks(context.Background(), jobID, url, job.Depth)
				if err != nil {
					log.Printf("Failed to enqueue extract links task for %s: %v", url, err)
				}
			}
		} else if job != nil {
			log.Printf("Skipping link extraction from %s: max depth %d reached", url, w.maxLinkDepth)
		}
	}

	return nil
}

// isImageURL checks if a URL points to an image file
func isImageURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Get the path without query parameters
	path := strings.ToLower(parsedURL.Path)

	// Common image extensions
	imageExtensions := []string{
		".jpg", ".jpeg", ".png", ".gif", ".webp",
		".svg", ".bmp", ".ico", ".tiff", ".tif",
	}

	for _, ext := range imageExtensions {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}

	return false
}

// shouldSkipURL checks if a URL should be skipped for scraping
// Returns true if the URL is not scrapeable (non-HTTP/HTTPS, mailto, tel, etc.)
func shouldSkipURL(rawURL string) bool {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return true // Skip invalid URLs
	}

	// Only allow http and https schemes
	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return true
	}

	// Skip image URLs
	if isImageURL(rawURL) {
		return true
	}

	return false
}

// extractAndQueueLinks extracts links and queues them for scraping
func (w *Worker) extractAndQueueLinks(ctx context.Context, parentJobID, sourceURL string, parentDepth int) {
	extractResp, err := w.scraperClient.ExtractLinks(ctx, sourceURL)
	if err != nil {
		log.Printf("Failed to extract links from %s: %v", sourceURL, err)
		return
	}

	// Filter out URLs that should not be scraped (images, mailto, tel, etc.)
	var scrapableLinks []string
	for _, link := range extractResp.Links {
		if !shouldSkipURL(link) {
			scrapableLinks = append(scrapableLinks, link)
		}
	}

	skippedCount := len(extractResp.Links) - len(scrapableLinks)
	if skippedCount > 0 {
		log.Printf("Filtered out %d non-scrapable URLs from %s", skippedCount, sourceURL)
	}

	// Process all extracted links (no limit)
	links := scrapableLinks

	log.Printf("Queueing %d extracted links for scraping (child depth: %d)", len(links), parentDepth+1)

	childDepth := parentDepth + 1
	shouldExtractLinks := childDepth < w.maxLinkDepth

	for i, link := range links {
		jobID := uuid.New().String()
		job := &storage.ScrapeJob{
			ID:           jobID,
			URL:          link,
			ExtractLinks: shouldExtractLinks,
			Status:       "queued",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			ParentJobID:  &parentJobID,
			Depth:        childDepth,
		}

		if err := w.storage.SaveScrapeJob(job); err != nil {
			log.Printf("Failed to save scrape job for %s: %v", link, err)
			continue
		}

		// Enqueue to Asynq with delay to spread load
		if w.queueClient != nil {
			taskID, err := w.queueClient.EnqueueScrapeWithParent(ctx, jobID, link, shouldExtractLinks, &parentJobID, childDepth)
			if err != nil {
				log.Printf("Failed to enqueue task for %s: %v", link, err)
				continue
			}

			// Update job with task ID
			if err := w.storage.UpdateScrapeJobTaskID(jobID, taskID); err != nil {
				log.Printf("Warning: Failed to update task ID for job %s: %v", jobID, err)
			}

			log.Printf("[%d/%d] Queued child job %s for link: %s (extract_links=%v)", i+1, len(links), jobID, link, shouldExtractLinks)
		}
	}
}

// handleExtractLinksTask processes a link extraction task
func (w *Worker) handleExtractLinksTask(ctx context.Context, t *asynq.Task) error {
	// Parse payload
	var payload ExtractLinksTaskPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		w.logger.Error("failed to unmarshal extract links task payload", "error", err)
		return fmt.Errorf("invalid task payload: %w", err)
	}

	// Calculate queue wait time
	var queueWaitTime time.Duration
	if payload.EnqueuedAt > 0 {
		enqueuedTime := time.Unix(0, payload.EnqueuedAt)
		queueWaitTime = time.Since(enqueuedTime)
	}

	w.logger.Info("processing extract links task",
		"parent_job_id", payload.ParentJobID,
		"source_url", payload.SourceURL,
		"parent_depth", payload.ParentDepth,
		"queue_wait_seconds", queueWaitTime.Seconds(),
	)

	// Recreate trace context from payload if available
	var span trace.Span
	if payload.TraceID != "" && payload.SpanID != "" {
		// Parse trace ID and span ID from hex strings
		traceID, err := trace.TraceIDFromHex(payload.TraceID)
		if err == nil {
			spanID, err := trace.SpanIDFromHex(payload.SpanID)
			if err == nil {
				// Create span context from stored IDs
				remoteSpanCtx := trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    traceID,
					SpanID:     spanID,
					TraceFlags: trace.FlagsSampled,
					Remote:     true,
				})

				// Create new context with the remote span context
				ctx = trace.ContextWithRemoteSpanContext(ctx, remoteSpanCtx)

				// Start a new span linked to the enqueue span
				ctx, span = otel.Tracer("controller").Start(ctx, "asynq.task.process",
					trace.WithSpanKind(trace.SpanKindConsumer),
					trace.WithAttributes(
						attribute.String("task.type", TypeExtractLinks),
						attribute.String("parent_job_id", payload.ParentJobID),
						attribute.String("source_url", payload.SourceURL),
						attribute.Int("parent_depth", payload.ParentDepth),
						attribute.Float64("queue.wait_time_seconds", queueWaitTime.Seconds()),
						attribute.Int64("enqueued_at", payload.EnqueuedAt),
					),
				)
				defer span.End()

				// Record queue wait time event
				span.AddEvent("task_processing_started", trace.WithAttributes(
					attribute.Float64("wait_time_seconds", queueWaitTime.Seconds()),
				))
			}
		}
	} else {
		// No trace context in payload, check current context
		if existingSpan := trace.SpanFromContext(ctx); existingSpan.SpanContext().IsValid() {
			existingSpan.SetAttributes(
				attribute.String("parent_job_id", payload.ParentJobID),
				attribute.String("source_url", payload.SourceURL),
				attribute.Int("parent_depth", payload.ParentDepth),
				attribute.Float64("queue.wait_time_seconds", queueWaitTime.Seconds()),
			)
		}
	}

	// Extract and queue links - this runs in its own task with its own context
	w.extractAndQueueLinks(ctx, payload.ParentJobID, payload.SourceURL, payload.ParentDepth)

	return nil
}

// extractDomainTag extracts a domain tag from a URL
func extractDomainTag(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	domain := parsedURL.Hostname()
	// Remove www. prefix if present
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}

// compressHTML compresses and base64 encodes HTML text
func compressHTML(html string) (string, error) {
	if html == "" {
		return "", nil
	}

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	if _, err := gzWriter.Write([]byte(html)); err != nil {
		return "", fmt.Errorf("failed to write to gzip: %w", err)
	}

	if err := gzWriter.Close(); err != nil {
		return "", fmt.Errorf("failed to close gzip writer: %w", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
