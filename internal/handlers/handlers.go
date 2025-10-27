package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/queue"
	"github.com/zombar/controller/internal/scraper_requests"
	internalslug "github.com/zombar/controller/internal/slug"
	"github.com/zombar/controller/internal/storage"
	"github.com/zombar/purpletab/pkg/metrics"
)

// Handler contains all HTTP handlers
type Handler struct {
	storage                 *storage.Storage
	scraper                 *clients.ScraperClient
	textAnalyzer            *clients.TextAnalyzerClient
	scheduler               *clients.SchedulerClient
	linkScoreThreshold      float64
	scrapeRequests          *scraper_requests.Manager // TODO: Remove after text analysis queue is implemented
	queueClient             *queue.Client
	urlCache                URLCache
	webInterfaceURL         string
	scraperBaseURL          string
	businessMetrics         *metrics.BusinessMetrics
	tombstonePeriodLowScore int // Days until deletion for low-score URLs
	tombstonePeriodManual   int // Days until deletion for manual tombstones
}

// URLCache defines the interface for URL caching
type URLCache interface {
	Get(ctx context.Context, url string) (string, error)
	Set(ctx context.Context, url, scraperUUID string) error
	Delete(ctx context.Context, url string) error
}

// New creates a new Handler (deprecated, use NewWithMetrics instead)
func New(store *storage.Storage, scraper *clients.ScraperClient, textAnalyzer *clients.TextAnalyzerClient, scheduler *clients.SchedulerClient, queueClient *queue.Client, urlCache URLCache, linkScoreThreshold float64, webInterfaceURL string, scraperBaseURL string, tombstonePeriodLowScore, tombstonePeriodManual int) *Handler {
	// Initialize business metrics
	businessMetrics := metrics.NewBusinessMetrics("controller")
	return NewWithMetrics(store, scraper, textAnalyzer, scheduler, queueClient, urlCache, linkScoreThreshold, webInterfaceURL, scraperBaseURL, tombstonePeriodLowScore, tombstonePeriodManual, businessMetrics)
}

// NewWithMetrics creates a new Handler with provided business metrics
func NewWithMetrics(store *storage.Storage, scraper *clients.ScraperClient, textAnalyzer *clients.TextAnalyzerClient, scheduler *clients.SchedulerClient, queueClient *queue.Client, urlCache URLCache, linkScoreThreshold float64, webInterfaceURL string, scraperBaseURL string, tombstonePeriodLowScore, tombstonePeriodManual int, businessMetrics *metrics.BusinessMetrics) *Handler {
	h := &Handler{
		storage:                 store,
		scraper:                 scraper,
		textAnalyzer:            textAnalyzer,
		scheduler:               scheduler,
		linkScoreThreshold:      linkScoreThreshold,
		scrapeRequests:          scraper_requests.NewManager(), // TODO: Remove after text analysis queue is implemented
		queueClient:             queueClient,
		urlCache:                urlCache,
		webInterfaceURL:         webInterfaceURL,
		scraperBaseURL:          scraperBaseURL,
		businessMetrics:         businessMetrics,
		tombstonePeriodLowScore: tombstonePeriodLowScore,
		tombstonePeriodManual:   tombstonePeriodManual,
	}

	// Start periodic metrics updater for gauges
	go h.startMetricsUpdater()

	return h
}

// GetBusinessMetrics returns the business metrics instance
func (h *Handler) GetBusinessMetrics() *metrics.BusinessMetrics {
	return h.businessMetrics
}

// startMetricsUpdater periodically updates gauge metrics
func (h *Handler) startMetricsUpdater() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		h.updateMetrics()
	}
}

// updateMetrics updates gauge metrics for queue and job status
func (h *Handler) updateMetrics() {
	// Update queue length (if queue client is available)
	if h.queueClient != nil {
		// Note: Asynq doesn't provide a simple way to get queue length
		// This would require implementing a custom inspector
		// For now, we'll skip this metric
	}

	// Update job status counts
	statuses := []string{"pending", "processing", "completed", "failed", "queued"}
	for _, status := range statuses {
		count, err := h.storage.CountScrapeJobsByStatus(status)
		if err != nil {
			log.Printf("Failed to count jobs by status %s: %v", status, err)
			continue
		}
		h.businessMetrics.ScrapeJobsByStatus.WithLabelValues(status).Set(float64(count))
	}
}

// ScrapeURLRequest represents a request to scrape a URL
type ScrapeURLRequest struct {
	URL          string `json:"url"`
	ExtractLinks bool   `json:"extract_links,omitempty"`
}

// AnalyzeTextRequest represents a request to analyze text directly
type AnalyzeTextRequest struct {
	Text string `json:"text"`
}

// SearchTagsRequest represents a request to search by tags
type SearchTagsRequest struct {
	Tags  []string `json:"tags"`
	Fuzzy bool     `json:"fuzzy"`
}

// FilterRequestsRequest represents a request to filter requests
type FilterRequestsRequest struct {
	Tags       []string  `json:"tags,omitempty"`
	Fuzzy      bool      `json:"fuzzy"`
	DateStart  *string   `json:"date_start,omitempty"`
	DateEnd    *string   `json:"date_end,omitempty"`
	SourceType *string   `json:"source_type,omitempty"`
	Limit      int       `json:"limit,omitempty"`
	Offset     int       `json:"offset,omitempty"`
}

// ControllerResponse represents the response from the controller
type ControllerResponse struct {
	ID               string                 `json:"id"`
	CreatedAt        time.Time              `json:"created_at"`
	EffectiveDate    time.Time              `json:"effective_date"`
	SourceType       string                 `json:"source_type"`
	SourceURL        *string                `json:"source_url,omitempty"`
	ScraperUUID      *string                `json:"scraper_uuid,omitempty"`
	TextAnalyzerUUID string                 `json:"textanalyzer_uuid"`
	Tags             []string               `json:"tags"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Slug             *string                `json:"slug,omitempty"`
	SEOEnabled       bool                   `json:"seo_enabled"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// ScrapeURL handles URL scraping and text analysis with quality scoring
func (h *Handler) ScrapeURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ScrapeURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		respondError(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Score the link first to determine if it should be fully processed
	scoreResp, err := h.scraper.ScoreLink(r.Context(), req.URL)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to score URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Create controller request record
	controllerID := uuid.New().String()

	// Check if this is an image URL (skip threshold check for images)
	isImageURL := false
	for _, category := range scoreResp.Score.Categories {
		if category == "image" {
			isImageURL = true
			break
		}
	}

	// Check if score meets threshold (skip for image URLs)
	if !isImageURL && scoreResp.Score.Score < h.linkScoreThreshold {
		// Score is below threshold - mark for tombstoning and return scoring metadata only
		tombstoneTime := time.Now().UTC().Add(time.Duration(h.tombstonePeriodLowScore) * 24 * time.Hour)

		// Add domain name to tags
		tags := scoreResp.Score.Categories
		if domain := extractDomainTag(req.URL); domain != "" {
			tags = append(tags, domain)
		}

		// Add 'scrape' tag to all scraped content
		tags = append(tags, "scrape")

		record := &storage.Request{
			ID:         controllerID,
			CreatedAt:  time.Now().UTC(),
			SourceType: "url",
			SourceURL:  &req.URL,
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
				"threshold":          h.linkScoreThreshold,
				"tombstone_datetime": tombstoneTime.Format(time.RFC3339), // Auto-tombstone low quality content
			},
		}

		if err := h.storage.SaveRequest(record); err != nil {
			respondError(w, fmt.Sprintf("Failed to save request: %v", err), http.StatusInternalServerError)
			return
		}

		// Record tombstone metrics
		h.businessMetrics.TombstonesCreatedTotal.WithLabelValues("low-score", "none").Inc()
		h.businessMetrics.TombstoneDaysHistogram.WithLabelValues("low-score").Observe(float64(h.tombstonePeriodLowScore))
		slog.Info("tombstone created",
			"reason", "low-score",
			"url", req.URL,
			"score", scoreResp.Score.Score,
			"threshold", h.linkScoreThreshold,
			"period_days", h.tombstonePeriodLowScore,
		)

		response := ControllerResponse{
			ID:            record.ID,
			CreatedAt:     record.CreatedAt,
			EffectiveDate: record.EffectiveDate,
			SourceType:    record.SourceType,
			SourceURL:     record.SourceURL,
			Tags:          record.Tags,
			Metadata:      record.Metadata,
			Slug:          record.Slug,
			SEOEnabled:    record.SEOEnabled,
		}

		respondJSON(w, response, http.StatusCreated)
		return
	}

	// Score meets or exceeds threshold - proceed with full scraping
	scraperResp, err := h.scraper.Scrape(r.Context(), req.URL)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to scrape URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Build scraper metadata from the scraper response
	scraperMetadata := make(map[string]interface{})
	scraperMetadata["title"] = scraperResp.Title
	scraperMetadata["content"] = scraperResp.Content
	scraperMetadata["raw_text"] = scraperResp.RawText // Include original raw text
	scraperMetadata["url"] = scraperResp.URL

	// Also include fields from the scraper's Metadata (description, keywords, etc.)
	if scraperResp.Metadata != nil {
		for k, v := range scraperResp.Metadata {
			scraperMetadata[k] = v
		}
	}

	// Analyze the content (skip for image URLs)
	var analyzerResp *clients.TextAnalyzerResponse
	if !isImageURL {
		analyzerResp, err = h.textAnalyzer.Analyze(r.Context(), scraperResp.Content)
		if err != nil {
			respondError(w, fmt.Sprintf("Failed to analyze text: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Build combined metadata
	combinedMetadata := make(map[string]interface{})
	combinedMetadata["scraper_metadata"] = scraperMetadata
	if analyzerResp != nil {
		combinedMetadata["analyzer_metadata"] = analyzerResp.Metadata
	}

	// Add link score from scraper response if available, otherwise use preliminary score
	if scraperResp.Score != nil {
		combinedMetadata["link_score"] = map[string]interface{}{
			"score":                scraperResp.Score.Score,
			"reason":               scraperResp.Score.Reason,
			"categories":           scraperResp.Score.Categories,
			"is_recommended":       scraperResp.Score.IsRecommended,
			"malicious_indicators": scraperResp.Score.MaliciousIndicators,
		}
	} else {
		// Fallback to preliminary score if scraper didn't return one
		combinedMetadata["link_score"] = map[string]interface{}{
			"score":                scoreResp.Score.Score,
			"reason":               scoreResp.Score.Reason,
			"categories":           scoreResp.Score.Categories,
			"is_recommended":       scoreResp.Score.IsRecommended,
			"malicious_indicators": scoreResp.Score.MaliciousIndicators,
		}
	}

	// Get tags and analyzer UUID (handle nil for image URLs)
	var tags []string
	var analyzerUUID string
	if analyzerResp != nil {
		tags = analyzerResp.GetTags()
		analyzerUUID = analyzerResp.ID
	} else {
		// For image URLs, use categories from link score as tags
		if scraperResp.Score != nil {
			tags = scraperResp.Score.Categories
		}
	}

	// Add domain name to tags
	if domain := extractDomainTag(req.URL); domain != "" {
		tags = append(tags, domain)
	}

	// Add 'scrape' tag to all scraped content
	tags = append(tags, "scrape")

	// Extract slug from scraper response if available
	var slug *string
	if scraperResp.Slug != "" {
		slug = &scraperResp.Slug
	}

	record := &storage.Request{
		ID:               controllerID,
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &req.URL,
		ScraperUUID:      &scraperResp.ID,
		TextAnalyzerUUID: analyzerUUID,
		Tags:             tags,
		Metadata:         combinedMetadata,
		Slug:             slug,
		SEOEnabled:       true, // Enable SEO by default
	}

	if err := h.storage.SaveRequest(record); err != nil {
		respondError(w, fmt.Sprintf("Failed to save request: %v", err), http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := ControllerResponse{
		ID:               record.ID,
		CreatedAt:        record.CreatedAt,
		EffectiveDate:    record.EffectiveDate,
		SourceType:       record.SourceType,
		SourceURL:        record.SourceURL,
		ScraperUUID:      record.ScraperUUID,
		TextAnalyzerUUID: record.TextAnalyzerUUID,
		Tags:             record.Tags,
		Metadata:         record.Metadata,
		Slug:             record.Slug,
		SEOEnabled:       record.SEOEnabled,
	}

	respondJSON(w, response, http.StatusCreated)
}

// AnalyzeText handles direct text analysis
func (h *Handler) AnalyzeText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AnalyzeTextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		respondError(w, "Text is required", http.StatusBadRequest)
		return
	}

	// Call text analyzer service
	analyzerResp, err := h.textAnalyzer.Analyze(r.Context(), req.Text)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to analyze text: %v", err), http.StatusInternalServerError)
		return
	}

	// Create controller request record
	controllerID := uuid.New().String()

	// Generate slug from cleaned text or first few words
	var slug *string
	textForSlug := ""

	// Try to get cleaned_text from metadata
	if cleanedText, ok := analyzerResp.Metadata["cleaned_text"].(string); ok && cleanedText != "" {
		// Use first 100 chars of cleaned text for slug
		textForSlug = cleanedText
		if len(textForSlug) > 100 {
			textForSlug = textForSlug[:100]
		}
	} else if req.Text != "" {
		// Fallback to first 100 chars of original text
		textForSlug = req.Text
		if len(textForSlug) > 100 {
			textForSlug = textForSlug[:100]
		}
	}

	if textForSlug != "" {
		generatedSlug := internalslug.GenerateWithFallback(textForSlug, controllerID)
		slug = &generatedSlug
	}
	record := &storage.Request{
		ID:               controllerID,
		CreatedAt:        time.Now().UTC(),
		SourceType:       "text",
		TextAnalyzerUUID: analyzerResp.ID,
		Tags:             analyzerResp.GetTags(),
		Metadata: map[string]interface{}{
			"analyzer_metadata": analyzerResp.Metadata,
			"original_text":     req.Text, // Store original submitted text
		},
		Slug:             slug,
		SEOEnabled:       true, // Enable SEO by default
	}

	if err := h.storage.SaveRequest(record); err != nil {
		respondError(w, fmt.Sprintf("Failed to save request: %v", err), http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := ControllerResponse{
		ID:               record.ID,
		CreatedAt:        record.CreatedAt,
		EffectiveDate:    record.EffectiveDate,
		SourceType:       record.SourceType,
		TextAnalyzerUUID: record.TextAnalyzerUUID,
		Tags:             record.Tags,
		Metadata:         record.Metadata,
		Slug:             record.Slug,
		SEOEnabled:       record.SEOEnabled,
	}

	respondJSON(w, response, http.StatusCreated)
}

// SearchTags handles tag searching
func (h *Handler) SearchTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchTagsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Tags) == 0 {
		respondError(w, "At least one tag is required", http.StatusBadRequest)
		return
	}

	requestIDs, err := h.storage.SearchByTags(req.Tags, req.Fuzzy)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to search tags: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"request_ids": requestIDs,
		"count":       len(requestIDs),
	}

	respondJSON(w, response, http.StatusOK)
}

// FilterRequests handles filtering requests with multiple criteria
func (h *Handler) FilterRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req FilterRequestsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Parse date strings to time.Time if provided
	var dateStart, dateEnd *time.Time
	if req.DateStart != nil && *req.DateStart != "" {
		parsedStart, err := time.Parse(time.RFC3339, *req.DateStart)
		if err != nil {
			respondError(w, fmt.Sprintf("Invalid date_start format (use RFC3339): %v", err), http.StatusBadRequest)
			return
		}
		dateStart = &parsedStart
	}
	if req.DateEnd != nil && *req.DateEnd != "" {
		parsedEnd, err := time.Parse(time.RFC3339, *req.DateEnd)
		if err != nil {
			respondError(w, fmt.Sprintf("Invalid date_end format (use RFC3339): %v", err), http.StatusBadRequest)
			return
		}
		dateEnd = &parsedEnd
	}

	// Set default limit if not specified
	limit := req.Limit
	if limit == 0 {
		limit = 100
	}

	// Build filter options
	opts := storage.FilterOptions{
		Tags:       req.Tags,
		Fuzzy:      req.Fuzzy,
		DateStart:  dateStart,
		DateEnd:    dateEnd,
		SourceType: req.SourceType,
		Limit:      limit,
		Offset:     req.Offset,
	}

	// Filter requests
	requests, err := h.storage.FilterRequests(opts)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to filter requests: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to response format
	var responses []ControllerResponse
	for _, record := range requests {
		responses = append(responses, ControllerResponse{
			ID:               record.ID,
			CreatedAt:        record.CreatedAt,
			EffectiveDate:    record.EffectiveDate,
			SourceType:       record.SourceType,
			SourceURL:        record.SourceURL,
			ScraperUUID:      record.ScraperUUID,
			TextAnalyzerUUID: record.TextAnalyzerUUID,
			Tags:             record.Tags,
			Metadata:         record.Metadata,
			Slug:             record.Slug,
		})
	}

	response := map[string]interface{}{
		"requests": responses,
		"count":    len(responses),
		"limit":    limit,
		"offset":   req.Offset,
	}

	respondJSON(w, response, http.StatusOK)
}

// GetTimelineExtents returns the earliest effective date from all documents.
// This endpoint is optimized for timeline visualization and returns only the minimum date.
// The client should compute maxDate as "now".
func (h *Handler) GetTimelineExtents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	earliestDate, err := h.storage.GetTimelineExtents()
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to get timeline extents: %v", err), http.StatusInternalServerError)
		return
	}

	// If no documents exist, return a default (30 days ago)
	if earliestDate == nil {
		defaultDate := time.Now().AddDate(0, 0, -30)
		earliestDate = &defaultDate
	}

	response := map[string]interface{}{
		"earliest_date": earliestDate.Format(time.RFC3339),
	}

	respondJSON(w, response, http.StatusOK)
}

// GetRequest retrieves a request by ID
func (h *Handler) GetRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	id := r.URL.Path[len("/api/requests/"):]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	record, err := h.storage.GetRequest(id)
	if err != nil {
		if err.Error() == "request not found" {
			respondError(w, "Request not found", http.StatusNotFound)
			return
		}
		respondError(w, fmt.Sprintf("Failed to get request: %v", err), http.StatusInternalServerError)
		return
	}

	response := ControllerResponse{
		ID:               record.ID,
		CreatedAt:        record.CreatedAt,
		EffectiveDate:    record.EffectiveDate,
		SourceType:       record.SourceType,
		SourceURL:        record.SourceURL,
		ScraperUUID:      record.ScraperUUID,
		TextAnalyzerUUID: record.TextAnalyzerUUID,
		Tags:             record.Tags,
		Metadata:         record.Metadata,
		Slug:             record.Slug,
		SEOEnabled:       record.SEOEnabled,
	}

	respondJSON(w, response, http.StatusOK)
}

// UpdateSEOEnabled updates the SEO enabled status for a request
func (h *Handler) UpdateSEOEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract request ID from path: /api/requests/{id}/seo-enabled
	path := r.URL.Path
	parts := strings.Split(path, "/")
	if len(parts) < 4 {
		respondError(w, "Invalid request path", http.StatusBadRequest)
		return
	}
	id := parts[3]

	// Parse request body
	var req struct {
		SEOEnabled bool `json:"seo_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update SEO enabled status
	if err := h.storage.UpdateSEOEnabled(id, req.SEOEnabled); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, "Request not found", http.StatusNotFound)
			return
		}
		respondError(w, fmt.Sprintf("Failed to update SEO enabled status: %v", err), http.StatusInternalServerError)
		return
	}

	// Get updated request
	record, err := h.storage.GetRequest(id)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to get updated request: %v", err), http.StatusInternalServerError)
		return
	}

	response := ControllerResponse{
		ID:               record.ID,
		CreatedAt:        record.CreatedAt,
		EffectiveDate:    record.EffectiveDate,
		SourceType:       record.SourceType,
		SourceURL:        record.SourceURL,
		ScraperUUID:      record.ScraperUUID,
		TextAnalyzerUUID: record.TextAnalyzerUUID,
		Tags:             record.Tags,
		Metadata:         record.Metadata,
		Slug:             record.Slug,
		SEOEnabled:       record.SEOEnabled,
	}

	respondJSON(w, response, http.StatusOK)
}

// DeleteRequest deletes a request and all associated data from the controller and upstream services
func (h *Handler) DeleteRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	id := r.URL.Path[len("/api/requests/"):]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	// Get the request to find associated UUIDs before deletion
	record, err := h.storage.GetRequest(id)
	if err != nil {
		if err.Error() == "request not found" {
			respondError(w, "Request not found", http.StatusNotFound)
			return
		}
		respondError(w, fmt.Sprintf("Failed to get request: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete from upstream services first
	if record.ScraperUUID != nil && *record.ScraperUUID != "" {
		if err := h.scraper.DeleteScrape(r.Context(), *record.ScraperUUID); err != nil {
			log.Printf("Warning: Failed to delete scrape %s: %v", *record.ScraperUUID, err)
		}
	}

	if record.TextAnalyzerUUID != "" {
		if err := h.textAnalyzer.DeleteAnalysis(r.Context(), record.TextAnalyzerUUID); err != nil {
			log.Printf("Warning: Failed to delete analysis %s: %v", record.TextAnalyzerUUID, err)
		}
	}

	// Delete from local storage
	if err := h.storage.DeleteRequest(id); err != nil {
		respondError(w, fmt.Sprintf("Failed to delete request: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"message": "Request deleted successfully"}, http.StatusOK)
}

// DeleteImage deletes an image from the scraper service
func (h *Handler) DeleteImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract image ID from URL path
	imageID := r.URL.Path[len("/api/images/"):]
	if imageID == "" {
		respondError(w, "Image ID is required", http.StatusBadRequest)
		return
	}

	// Delete from scraper service
	if err := h.scraper.DeleteImage(r.Context(), imageID); err != nil {
		respondError(w, fmt.Sprintf("Failed to delete image: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"message": "Image deleted successfully"}, http.StatusOK)
}

// TombstoneRequest marks a request as scheduled for deletion by adding tombstone_datetime to metadata
func (h *Handler) TombstoneRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	id := r.URL.Path[len("/api/requests/"):]
	// Remove the "/tombstone" suffix
	id = id[:len(id)-len("/tombstone")]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	// Get the existing request
	record, err := h.storage.GetRequest(id)
	if err != nil {
		if err.Error() == "request not found" {
			respondError(w, "Request not found", http.StatusNotFound)
			return
		}
		respondError(w, fmt.Sprintf("Failed to get request: %v", err), http.StatusInternalServerError)
		return
	}

	// Add tombstone_datetime to metadata (configurable days from now)
	if record.Metadata == nil {
		record.Metadata = make(map[string]interface{})
	}
	tombstoneTime := time.Now().UTC().Add(time.Duration(h.tombstonePeriodManual) * 24 * time.Hour)
	record.Metadata["tombstone_datetime"] = tombstoneTime.Format(time.RFC3339)

	// Update the request in storage
	if err := h.storage.UpdateRequestMetadata(id, record.Metadata); err != nil {
		respondError(w, fmt.Sprintf("Failed to update request: %v", err), http.StatusInternalServerError)
		return
	}

	// Record tombstone metrics
	h.businessMetrics.TombstonesCreatedTotal.WithLabelValues("manual", "none").Inc()
	h.businessMetrics.TombstoneDaysHistogram.WithLabelValues("manual").Observe(float64(h.tombstonePeriodManual))
	slog.Info("tombstone created",
		"reason", "manual",
		"request_id", id,
		"period_days", h.tombstonePeriodManual,
	)

	respondJSON(w, map[string]string{"message": "Request tombstoned successfully"}, http.StatusOK)
}

// UntombstoneRequest removes the tombstone from a request
func (h *Handler) UntombstoneRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	id := r.URL.Path[len("/api/requests/"):]
	// Remove the "/tombstone" suffix
	id = id[:len(id)-len("/tombstone")]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	// Get the existing request
	record, err := h.storage.GetRequest(id)
	if err != nil {
		if err.Error() == "request not found" {
			respondError(w, "Request not found", http.StatusNotFound)
			return
		}
		respondError(w, fmt.Sprintf("Failed to get request: %v", err), http.StatusInternalServerError)
		return
	}

	// Remove tombstone_datetime from metadata
	if record.Metadata != nil {
		delete(record.Metadata, "tombstone_datetime")
	}

	// Update the request in storage
	if err := h.storage.UpdateRequestMetadata(id, record.Metadata); err != nil {
		respondError(w, fmt.Sprintf("Failed to update request: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"message": "Request tombstone removed successfully"}, http.StatusOK)
}

// TombstoneImage marks an image as scheduled for deletion
func (h *Handler) TombstoneImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPatch {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract image ID from URL path
	imageID := r.URL.Path[len("/api/images/"):]
	// Remove the "/tombstone" suffix
	imageID = imageID[:len(imageID)-len("/tombstone")]
	if imageID == "" {
		respondError(w, "Image ID is required", http.StatusBadRequest)
		return
	}

	// Tombstone via scraper service
	if err := h.scraper.TombstoneImage(r.Context(), imageID); err != nil {
		respondError(w, fmt.Sprintf("Failed to tombstone image: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"message": "Image tombstoned successfully"}, http.StatusOK)
}

// UntombstoneImage removes the tombstone from an image
func (h *Handler) UntombstoneImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract image ID from URL path
	imageID := r.URL.Path[len("/api/images/"):]
	// Remove the "/tombstone" suffix
	imageID = imageID[:len(imageID)-len("/tombstone")]
	if imageID == "" {
		respondError(w, "Image ID is required", http.StatusBadRequest)
		return
	}

	// Untombstone via scraper service
	if err := h.scraper.UntombstoneImage(r.Context(), imageID); err != nil {
		respondError(w, fmt.Sprintf("Failed to untombstone image: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"message": "Image tombstone removed successfully"}, http.StatusOK)
}

// UpdateRequestTags updates the tags for a specific request
func (h *Handler) UpdateRequestTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path: /api/requests/{id}/tags
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		respondError(w, "Invalid URL path", http.StatusBadRequest)
		return
	}
	id := parts[len(parts)-2] // ID is second-to-last part

	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update tags in storage
	if err := h.storage.UpdateRequestTags(id, req.Tags); err != nil {
		if err.Error() == "request not found" {
			respondError(w, "Request not found", http.StatusNotFound)
			return
		}
		respondError(w, fmt.Sprintf("Failed to update tags: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"message": "Tags updated successfully"}, http.StatusOK)
}

// UpdateImageTags updates the tags for a specific image
func (h *Handler) UpdateImageTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path: /api/images/{id}/tags
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		respondError(w, "Invalid URL path", http.StatusBadRequest)
		return
	}
	id := parts[len(parts)-2] // ID is second-to-last part

	if id == "" {
		respondError(w, "Image ID is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Update tags via scraper service
	if err := h.scraper.UpdateImageTags(r.Context(), id, req.Tags); err != nil {
		if strings.Contains(err.Error(), "image not found") {
			respondError(w, "Image not found", http.StatusNotFound)
			return
		}
		respondError(w, fmt.Sprintf("Failed to update image tags: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"message": "Image tags updated successfully"}, http.StatusOK)
}

// ListRequests lists all requests with pagination
func (h *Handler) ListRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	limit := 50
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	records, err := h.storage.ListRequests(limit, offset)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to list requests: %v", err), http.StatusInternalServerError)
		return
	}

	var responses []ControllerResponse
	for _, record := range records {
		responses = append(responses, ControllerResponse{
			ID:               record.ID,
			CreatedAt:        record.CreatedAt,
			EffectiveDate:    record.EffectiveDate,
			SourceType:       record.SourceType,
			SourceURL:        record.SourceURL,
			ScraperUUID:      record.ScraperUUID,
			TextAnalyzerUUID: record.TextAnalyzerUUID,
			Tags:             record.Tags,
			Metadata:         record.Metadata,
			Slug:             record.Slug,
		})
	}

	response := map[string]interface{}{
		"requests": responses,
		"count":    len(responses),
		"limit":    limit,
		"offset":   offset,
	}

	respondJSON(w, response, http.StatusOK)
}

// SearchImageTagsRequest represents a request to search images by tags
type SearchImageTagsRequest struct {
	Tags []string `json:"tags"`
}

// SearchImageTags handles fuzzy search for images by tags
func (h *Handler) SearchImageTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SearchImageTagsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Tags) == 0 {
		respondError(w, "At least one tag is required", http.StatusBadRequest)
		return
	}

	// Call scraper service to search images by tags (fuzzy matching)
	searchResp, err := h.scraper.SearchImagesByTags(r.Context(), req.Tags)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to search images: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"images": searchResp.Images,
		"count":  searchResp.Count,
	}

	respondJSON(w, response, http.StatusOK)
}

// GetDocumentImages retrieves images associated with a document's scraper UUID
func (h *Handler) GetDocumentImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract scraper UUID from URL path
	// Path format: /api/documents/{uuid}/images
	path := strings.TrimPrefix(r.URL.Path, "/api/documents/")
	path = strings.TrimSuffix(path, "/images")
	scrapeID := path

	if scrapeID == "" || strings.Contains(scrapeID, "/") {
		respondError(w, "Scraper UUID is required", http.StatusBadRequest)
		return
	}

	// Call scraper service to get images by scrape ID
	searchResp, err := h.scraper.GetImagesByScrapeID(r.Context(), scrapeID)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to retrieve images: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"images": searchResp.Images,
		"count":  searchResp.Count,
	}

	respondJSON(w, response, http.StatusOK)
}

// GetImage retrieves a single image by ID
func (h *Handler) GetImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract image ID from URL path
	imageID := r.URL.Path[len("/api/images/"):]
	if imageID == "" {
		respondError(w, "Image ID is required", http.StatusBadRequest)
		return
	}

	// Call scraper service to get image by ID
	image, err := h.scraper.GetImageByID(r.Context(), imageID)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to retrieve image: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, image, http.StatusOK)
}

// ScoreLinkRequest represents a request to score a link
type ScoreLinkRequest struct {
	URL string `json:"url"`
}

// ScoreLink handles link quality scoring
func (h *Handler) ScoreLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ScoreLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		respondError(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Call scraper service to score the link
	scoreResp, err := h.scraper.ScoreLink(r.Context(), req.URL)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to score link: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"url": scoreResp.URL,
		"score": map[string]interface{}{
			"score":                scoreResp.Score.Score,
			"reason":               scoreResp.Score.Reason,
			"categories":           scoreResp.Score.Categories,
			"is_recommended":       scoreResp.Score.IsRecommended,
			"malicious_indicators": scoreResp.Score.MaliciousIndicators,
		},
		"meets_threshold": scoreResp.Score.Score >= h.linkScoreThreshold,
		"threshold":       h.linkScoreThreshold,
	}

	respondJSON(w, response, http.StatusOK)
}

// ExtractLinksRequest represents a request to extract links from a URL
type ExtractLinksRequest struct {
	URL string `json:"url"`
}

// ExtractLinks handles extracting links from a URL
func (h *Handler) ExtractLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExtractLinksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		respondError(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Call scraper service to extract links
	extractResp, err := h.scraper.ExtractLinks(r.Context(), req.URL)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to extract links: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"url":   extractResp.URL,
		"links": extractResp.Links,
		"count": extractResp.Count,
	}

	respondJSON(w, response, http.StatusOK)
}

// CreateScrapeRequest creates a new async scrape request
func (h *Handler) CreateScrapeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ScrapeURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		respondError(w, "URL is required", http.StatusBadRequest)
		return
	}

	// Record scrape request received
	h.businessMetrics.ScrapeRequestsTotal.WithLabelValues("accepted").Inc()

	// Check cache for recently scraped URL
	if h.urlCache != nil {
		cachedScraperUUID, err := h.urlCache.Get(r.Context(), req.URL)
		if err != nil {
			slog.Warn("failed to check URL cache", "url", req.URL, "error", err)
			// Continue with scraping even if cache check fails
		} else if cachedScraperUUID != "" {
			// Cache hit - URL was scraped recently (within 30 days)
			slog.Info("cache hit for URL", "url", req.URL, "scraper_uuid", cachedScraperUUID)
			h.businessMetrics.ScrapeRequestsTotal.WithLabelValues("cached").Inc()

			// Fetch the existing scraped data
			existingData, err := h.storage.GetRequest(cachedScraperUUID)
			if err != nil {
				slog.Warn("cached scraper UUID not found in storage, proceeding with fresh scrape",
					"url", req.URL,
					"scraper_uuid", cachedScraperUUID,
					"error", err)
				// Cache is stale, invalidate it and proceed with scraping
				if delErr := h.urlCache.Delete(r.Context(), req.URL); delErr != nil {
					slog.Warn("failed to delete stale cache entry", "url", req.URL, "error", delErr)
				}
			} else {
				// Return the cached result
				response := map[string]interface{}{
					"id":         existingData.ID,
					"status":     "completed",
					"cached":     true,
					"created_at": existingData.CreatedAt,
				}
				if existingData.SourceURL != nil {
					response["url"] = *existingData.SourceURL
				}
				if existingData.ScraperUUID != nil {
					response["scraper_uuid"] = *existingData.ScraperUUID
				}
				respondJSON(w, response, http.StatusOK)
				return
			}
		}
	}

	// Create scrape job in database
	jobID := uuid.New().String()
	job := &storage.ScrapeJob{
		ID:           jobID,
		URL:          req.URL,
		ExtractLinks: req.ExtractLinks,
		Status:       "queued",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.storage.SaveScrapeJob(job); err != nil {
		h.businessMetrics.ScrapeRequestsTotal.WithLabelValues("error").Inc()
		respondError(w, fmt.Sprintf("Failed to create scrape job: %v", err), http.StatusInternalServerError)
		return
	}

	// Record scrape job created (parent job)
	h.businessMetrics.ScrapeJobsTotal.WithLabelValues("parent").Inc()

	// Enqueue task to Asynq (skip if queueClient is nil for testing)
	var taskID string
	if h.queueClient != nil {
		var err error
		taskID, err = h.queueClient.EnqueueScrape(r.Context(), jobID, req.URL, req.ExtractLinks)
		if err != nil {
			respondError(w, fmt.Sprintf("Failed to enqueue scrape task: %v", err), http.StatusInternalServerError)
			return
		}

		// Update job with Asynq task ID
		if err := h.storage.UpdateScrapeJobTaskID(jobID, taskID); err != nil {
			log.Printf("Warning: Failed to update task ID for job %s: %v", jobID, err)
		}
	}

	respondJSON(w, job, http.StatusOK)
}

// CreateTextAnalysisRequest creates a new async text analysis request
func (h *Handler) CreateTextAnalysisRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AnalyzeTextRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Text == "" {
		respondError(w, "Text is required", http.StatusBadRequest)
		return
	}

	// Create text analysis request
	analysisReq, _ := h.scrapeRequests.CreateText(req.Text)

	// Start background analysis
	go h.processTextAnalysisRequest(analysisReq.ID, req.Text)

	respondJSON(w, analysisReq, http.StatusOK)
}

// ListScrapeRequests returns all active scrape requests
func (h *Handler) ListScrapeRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse pagination parameters
	limit := 50
	offset := 0

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Query jobs from database
	jobs, err := h.storage.ListScrapeJobs(limit, offset)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to list scrape jobs: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"requests": jobs,
		"count":    len(jobs),
		"limit":    limit,
		"offset":   offset,
	}

	respondJSON(w, response, http.StatusOK)
}

// GetScrapeRequest returns a specific scrape request by ID
// Checks both in-memory text analysis requests and database scrape jobs
func (h *Handler) GetScrapeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/scrape-requests/"):]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	// First check in-memory manager for text analysis requests
	if req, ok := h.scrapeRequests.Get(id); ok {
		respondJSON(w, req, http.StatusOK)
		return
	}

	// If not found in memory, check database for scrape jobs
	job, err := h.storage.GetScrapeJob(id)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to get scrape job: %v", err), http.StatusInternalServerError)
		return
	}

	if job == nil {
		respondError(w, "Scrape request not found", http.StatusNotFound)
		return
	}

	respondJSON(w, job, http.StatusOK)
}

// RetryScrapeRequest retries a failed scrape request
func (h *Handler) RetryScrapeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/scrape-requests/"):len(r.URL.Path)-len("/retry")]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	job, err := h.storage.GetScrapeJob(id)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to get scrape job: %v", err), http.StatusInternalServerError)
		return
	}

	if job == nil {
		respondError(w, "Scrape request not found", http.StatusNotFound)
		return
	}

	// Only allow retrying failed requests
	if job.Status != "failed" {
		respondError(w, "Can only retry failed requests", http.StatusBadRequest)
		return
	}

	// Reset job status
	if err := h.storage.UpdateScrapeJobStatus(id, "queued", ""); err != nil {
		respondError(w, fmt.Sprintf("Failed to update job status: %v", err), http.StatusInternalServerError)
		return
	}

	// Re-enqueue task to Asynq (skip if queueClient is nil for testing)
	if h.queueClient != nil {
		taskID, err := h.queueClient.EnqueueScrape(r.Context(), id, job.URL, job.ExtractLinks)
		if err != nil {
			respondError(w, fmt.Sprintf("Failed to enqueue scrape task: %v", err), http.StatusInternalServerError)
			return
		}

		// Update job with new Asynq task ID
		if err := h.storage.UpdateScrapeJobTaskID(id, taskID); err != nil {
			log.Printf("Warning: Failed to update task ID for job %s: %v", id, err)
		}
	}

	// Get updated job
	updatedJob, _ := h.storage.GetScrapeJob(id)
	respondJSON(w, updatedJob, http.StatusOK)
}

// DeleteScrapeRequest deletes a scrape request
func (h *Handler) DeleteScrapeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Path[len("/api/scrape-requests/"):]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	// Note: This only deletes the job record, not the actual task from Asynq
	// In-flight tasks will continue processing
	if err := h.storage.DeleteScrapeJob(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			respondError(w, "Scrape request not found", http.StatusNotFound)
			return
		}
		respondError(w, fmt.Sprintf("Failed to delete scrape job: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"status": "deleted"}, http.StatusOK)
}

// processTextAnalysisRequest processes a text analysis request in the background
func (h *Handler) processTextAnalysisRequest(id, text string) {
	// Update status to processing
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 30)

	// Analyze the text
	analyzeResp, err := h.textAnalyzer.Analyze(context.Background(), text)
	if err != nil {
		h.scrapeRequests.SetFailed(id, fmt.Sprintf("Failed to analyze: %v", err))
		return
	}

	// Update progress
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 90)

	// Save to database
	requestID := uuid.New().String()

	// Generate slug from cleaned text or original text
	var slug *string
	textForSlug := ""

	// Try to get cleaned_text from metadata
	if cleanedText, ok := analyzeResp.Metadata["cleaned_text"].(string); ok && cleanedText != "" {
		textForSlug = cleanedText
		if len(textForSlug) > 100 {
			textForSlug = textForSlug[:100]
		}
	} else if text != "" {
		textForSlug = text
		if len(textForSlug) > 100 {
			textForSlug = textForSlug[:100]
		}
	}

	if textForSlug != "" {
		generatedSlug := internalslug.GenerateWithFallback(textForSlug, requestID)
		slug = &generatedSlug
	}

	req := &storage.Request{
		ID:               requestID,
		CreatedAt:        time.Now(),
		SourceType:       "text",
		TextAnalyzerUUID: analyzeResp.ID,
		Tags:             analyzeResp.GetTags(),
		Slug:             slug,
		SEOEnabled:       true, // Enable SEO by default
		Metadata: map[string]interface{}{
			"analyzer_metadata": analyzeResp.Metadata,
			"original_text":     text, // Store original submitted text
		},
	}

	if err := h.storage.SaveRequest(req); err != nil {
		h.scrapeRequests.SetFailed(id, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	// Mark as completed
	h.scrapeRequests.SetCompleted(id, requestID)
	log.Printf("Text analysis request %s completed successfully, result saved as %s", id, requestID)
}

// ListSchedulerTasks proxies the scheduler's list tasks endpoint
func (h *Handler) ListSchedulerTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tasks, err := h.scheduler.ListTasks(r.Context())
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to list tasks: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, tasks, http.StatusOK)
}

// GetSchedulerTask proxies the scheduler's get task endpoint
func (h *Handler) GetSchedulerTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract task ID from path
	idStr := r.URL.Path[len("/api/scheduler/tasks/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	task, err := h.scheduler.GetTask(r.Context(), id)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to get task: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, task, http.StatusOK)
}

// CreateSchedulerTask proxies the scheduler's create task endpoint
func (h *Handler) CreateSchedulerTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var task clients.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	createdTask, err := h.scheduler.CreateTask(r.Context(), &task)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to create task: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, createdTask, http.StatusCreated)
}

// UpdateSchedulerTask proxies the scheduler's update task endpoint
func (h *Handler) UpdateSchedulerTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract task ID from path
	idStr := r.URL.Path[len("/api/scheduler/tasks/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	var task clients.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	updatedTask, err := h.scheduler.UpdateTask(r.Context(), id, &task)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to update task: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, updatedTask, http.StatusOK)
}

// DeleteSchedulerTask proxies the scheduler's delete task endpoint
func (h *Handler) DeleteSchedulerTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract task ID from path
	idStr := r.URL.Path[len("/api/scheduler/tasks/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, "Invalid task ID", http.StatusBadRequest)
		return
	}

	if err := h.scheduler.DeleteTask(r.Context(), id); err != nil {
		respondError(w, fmt.Sprintf("Failed to delete task: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Health check endpoint
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]string{
		"status": "healthy",
	}
	respondJSON(w, response, http.StatusOK)
}

func respondJSON(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

// extractDomainTag extracts a clean domain name from a URL to use as a tag
// Returns the domain name without "www." prefix, or empty string if parsing fails
func extractDomainTag(urlStr string) string {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return ""
	}

	domain := parsed.Hostname()
	// Remove "www." prefix if present
	domain = strings.TrimPrefix(domain, "www.")

	return domain
}
