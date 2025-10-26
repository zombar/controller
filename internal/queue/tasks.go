package queue

import (
	"context"
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

	w.logger.Info("processing scrape task",
		"job_id", jobID,
		"url", url,
		"extract_links", extractLinks,
	)

	// Add tracing if available
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		span.SetAttributes(
			attribute.String("job.id", jobID),
			attribute.String("job.url", url),
			attribute.Bool("job.extract_links", extractLinks),
		)
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
		tombstoneTime := time.Now().UTC().Add(72 * time.Hour) // Tombstone in 3 days
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

	// Analyze the content (skip for image URLs)
	var analyzeResp *clients.TextAnalyzerResponse
	if !isImageURL {
		analyzeResp, err = w.textAnalyzerClient.Analyze(ctx, scrapeResp.Content)
		if err != nil {
			return fmt.Errorf("failed to analyze: %w", err)
		}
	}

	// Combine metadata
	combinedMetadata := make(map[string]interface{})
	combinedMetadata["scraper_metadata"] = scraperMetadata
	if analyzeResp != nil {
		combinedMetadata["analyzer_metadata"] = analyzeResp.Metadata
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

	// Get tags and analyzer UUID
	var tags []string
	var analyzerUUID string
	if analyzeResp != nil {
		tags = analyzeResp.GetTags()
		analyzerUUID = analyzeResp.ID
	} else {
		// For image URLs, use categories from link score as tags (normalized)
		if scrapeResp.Score != nil {
			tags = make([]string, 0, len(scrapeResp.Score.Categories))
			for _, cat := range scrapeResp.Score.Categories {
				tags = append(tags, clients.NormalizeTag(cat))
			}
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
		TextAnalyzerUUID: analyzerUUID,
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

	// Limit to top 10 by length
	links := scrapableLinks
	if len(links) > 10 {
		// Sort by length descending and take top 10
		sortedLinks := make([]string, len(links))
		copy(sortedLinks, links)
		for i := 0; i < len(sortedLinks); i++ {
			for j := i + 1; j < len(sortedLinks); j++ {
				if len(sortedLinks[j]) > len(sortedLinks[i]) {
					sortedLinks[i], sortedLinks[j] = sortedLinks[j], sortedLinks[i]
				}
			}
		}
		links = sortedLinks[:10]
	}

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

	w.logger.Info("processing extract links task",
		"parent_job_id", payload.ParentJobID,
		"source_url", payload.SourceURL,
		"parent_depth", payload.ParentDepth,
	)

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
