package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/scrapemanager"
	"github.com/zombar/controller/internal/storage"
)

// Handler contains all HTTP handlers
type Handler struct {
	storage            *storage.Storage
	scraper            *clients.ScraperClient
	textAnalyzer       *clients.TextAnalyzerClient
	scrapeManager      *scrapemanager.Manager
	linkScoreThreshold float64
}

// New creates a new Handler
func New(store *storage.Storage, scraper *clients.ScraperClient, textAnalyzer *clients.TextAnalyzerClient, scrapeManager *scrapemanager.Manager, linkScoreThreshold float64) *Handler {
	return &Handler{
		storage:            store,
		scraper:            scraper,
		textAnalyzer:       textAnalyzer,
		scrapeManager:      scrapeManager,
		linkScoreThreshold: linkScoreThreshold,
	}
}

// ScrapeURLRequest represents a request to scrape a URL
type ScrapeURLRequest struct {
	URL string `json:"url"`
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

// ControllerResponse represents the response from the controller
type ControllerResponse struct {
	ID               string                 `json:"id"`
	CreatedAt        time.Time              `json:"created_at"`
	SourceType       string                 `json:"source_type"`
	SourceURL        *string                `json:"source_url,omitempty"`
	ScraperUUID      *string                `json:"scraper_uuid,omitempty"`
	TextAnalyzerUUID string                 `json:"textanalyzer_uuid"`
	Tags             []string               `json:"tags"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
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
	scoreResp, err := h.scraper.ScoreLink(req.URL)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to score URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Create controller request record
	controllerID := uuid.New().String()

	// Check if score meets threshold
	if scoreResp.Score.Score < h.linkScoreThreshold {
		// Score is below threshold - return scoring metadata only
		record := &storage.Request{
			ID:         controllerID,
			CreatedAt:  time.Now().UTC(),
			SourceType: "url",
			SourceURL:  &req.URL,
			Tags:       scoreResp.Score.Categories,
			Metadata: map[string]interface{}{
				"link_score": map[string]interface{}{
					"score":                scoreResp.Score.Score,
					"reason":               scoreResp.Score.Reason,
					"categories":           scoreResp.Score.Categories,
					"is_recommended":       scoreResp.Score.IsRecommended,
					"malicious_indicators": scoreResp.Score.MaliciousIndicators,
				},
				"below_threshold": true,
				"threshold":       h.linkScoreThreshold,
			},
		}

		if err := h.storage.SaveRequest(record); err != nil {
			respondError(w, fmt.Sprintf("Failed to save request: %v", err), http.StatusInternalServerError)
			return
		}

		response := ControllerResponse{
			ID:         record.ID,
			CreatedAt:  record.CreatedAt,
			SourceType: record.SourceType,
			SourceURL:  record.SourceURL,
			Tags:       record.Tags,
			Metadata:   record.Metadata,
		}

		respondJSON(w, response, http.StatusCreated)
		return
	}

	// Score meets or exceeds threshold - proceed with full scraping and analysis
	scraperResp, err := h.scraper.Scrape(req.URL)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to scrape URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Call text analyzer service with the main text
	analyzerResp, err := h.textAnalyzer.Analyze(scraperResp.Content)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to analyze text: %v", err), http.StatusInternalServerError)
		return
	}

	// Build scraper metadata from the scraper response
	scraperMetadata := make(map[string]interface{})
	scraperMetadata["title"] = scraperResp.Title
	scraperMetadata["content"] = scraperResp.Content
	scraperMetadata["url"] = scraperResp.URL

	// Also include fields from the scraper's Metadata (description, keywords, etc.)
	if scraperResp.Metadata != nil {
		for k, v := range scraperResp.Metadata {
			scraperMetadata[k] = v
		}
	}

	// Build combined metadata
	combinedMetadata := map[string]interface{}{
		"scraper_metadata":  scraperMetadata,
		"analyzer_metadata": analyzerResp.Metadata,
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

	record := &storage.Request{
		ID:               controllerID,
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &req.URL,
		ScraperUUID:      &scraperResp.ID,
		TextAnalyzerUUID: analyzerResp.ID,
		Tags:             analyzerResp.GetTags(),
		Metadata:         combinedMetadata,
	}

	if err := h.storage.SaveRequest(record); err != nil {
		respondError(w, fmt.Sprintf("Failed to save request: %v", err), http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := ControllerResponse{
		ID:               record.ID,
		CreatedAt:        record.CreatedAt,
		SourceType:       record.SourceType,
		SourceURL:        record.SourceURL,
		ScraperUUID:      record.ScraperUUID,
		TextAnalyzerUUID: record.TextAnalyzerUUID,
		Tags:             record.Tags,
		Metadata:         record.Metadata,
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
	analyzerResp, err := h.textAnalyzer.Analyze(req.Text)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to analyze text: %v", err), http.StatusInternalServerError)
		return
	}

	// Create controller request record
	controllerID := uuid.New().String()
	record := &storage.Request{
		ID:               controllerID,
		CreatedAt:        time.Now().UTC(),
		SourceType:       "text",
		TextAnalyzerUUID: analyzerResp.ID,
		Tags:             analyzerResp.GetTags(),
		Metadata: map[string]interface{}{
			"analyzer_metadata": analyzerResp.Metadata,
		},
	}

	if err := h.storage.SaveRequest(record); err != nil {
		respondError(w, fmt.Sprintf("Failed to save request: %v", err), http.StatusInternalServerError)
		return
	}

	// Prepare response
	response := ControllerResponse{
		ID:               record.ID,
		CreatedAt:        record.CreatedAt,
		SourceType:       record.SourceType,
		TextAnalyzerUUID: record.TextAnalyzerUUID,
		Tags:             record.Tags,
		Metadata:         record.Metadata,
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
		SourceType:       record.SourceType,
		SourceURL:        record.SourceURL,
		ScraperUUID:      record.ScraperUUID,
		TextAnalyzerUUID: record.TextAnalyzerUUID,
		Tags:             record.Tags,
		Metadata:         record.Metadata,
	}

	respondJSON(w, response, http.StatusOK)
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
			SourceType:       record.SourceType,
			SourceURL:        record.SourceURL,
			ScraperUUID:      record.ScraperUUID,
			TextAnalyzerUUID: record.TextAnalyzerUUID,
			Tags:             record.Tags,
			Metadata:         record.Metadata,
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
	searchResp, err := h.scraper.SearchImagesByTags(req.Tags)
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
	scoreResp, err := h.scraper.ScoreLink(req.URL)
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
	extractResp, err := h.scraper.ExtractLinks(req.URL)
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

// BatchScrapeRequest represents a request to scrape multiple URLs
type BatchScrapeRequest struct {
	URLs  []string `json:"urls"`
	Force bool     `json:"force"`
}

// BatchScrape handles scraping multiple URLs
func (h *Handler) BatchScrape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BatchScrapeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.URLs) == 0 {
		respondError(w, "At least one URL is required", http.StatusBadRequest)
		return
	}

	// Call scraper service to batch scrape
	batchResp, err := h.scraper.BatchScrape(req.URLs, req.Force)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to batch scrape: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"results": batchResp.Results,
		"summary": batchResp.Summary,
	}

	respondJSON(w, response, http.StatusOK)
}

// CreateScrapeRequest handles creating an async scrape request
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

	// Create scrape request (will reuse existing if URL is already being scraped)
	scrapeReq := h.scrapeManager.Create(req.URL)

	// If this is a new request, start processing asynchronously
	if scrapeReq.Status == scrapemanager.StatusPending {
		go h.processScrapeRequest(scrapeReq.ID)
	}

	respondJSON(w, scrapeReq, http.StatusCreated)
}

// GetScrapeRequest retrieves a scrape request by ID
func (h *Handler) GetScrapeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	id := r.URL.Path[len("/api/scrape/request/"):]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	scrapeReq, exists := h.scrapeManager.Get(id)
	if !exists {
		respondError(w, "Scrape request not found", http.StatusNotFound)
		return
	}

	respondJSON(w, scrapeReq, http.StatusOK)
}

// ListScrapeRequests lists all scrape requests
func (h *Handler) ListScrapeRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	requests := h.scrapeManager.List()

	response := map[string]interface{}{
		"requests": requests,
		"count":    len(requests),
	}

	respondJSON(w, response, http.StatusOK)
}

// DeleteScrapeRequest deletes a scrape request
func (h *Handler) DeleteScrapeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path
	id := r.URL.Path[len("/api/scrape/request/"):]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	if !h.scrapeManager.Delete(id) {
		respondError(w, "Scrape request not found", http.StatusNotFound)
		return
	}

	response := map[string]string{
		"message": "Scrape request deleted",
	}
	respondJSON(w, response, http.StatusOK)
}

// RetryScrapeRequest retries a failed scrape request
func (h *Handler) RetryScrapeRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract ID from URL path (format: /api/scrape/request/{id}/retry)
	path := r.URL.Path[len("/api/scrape/request/"):]
	id := path[:len(path)-len("/retry")]
	if id == "" {
		respondError(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	scrapeReq, exists := h.scrapeManager.Get(id)
	if !exists {
		respondError(w, "Scrape request not found", http.StatusNotFound)
		return
	}

	// Only retry failed requests
	if scrapeReq.Status != scrapemanager.StatusFailed {
		respondError(w, "Can only retry failed requests", http.StatusBadRequest)
		return
	}

	// Reset request status
	h.scrapeManager.UpdateStatus(id, scrapemanager.StatusPending)
	h.scrapeManager.UpdateProgress(id, 0)
	h.scrapeManager.SetError(id, "")

	// Start processing asynchronously
	go h.processScrapeRequest(id)

	// Get updated request
	scrapeReq, _ = h.scrapeManager.Get(id)
	respondJSON(w, scrapeReq, http.StatusOK)
}

// processScrapeRequest processes a scrape request asynchronously
func (h *Handler) processScrapeRequest(id string) {
	scrapeReq, exists := h.scrapeManager.Get(id)
	if !exists {
		return
	}

	// Update status to processing
	h.scrapeManager.UpdateStatus(id, scrapemanager.StatusProcessing)
	h.scrapeManager.UpdateProgress(id, 10)

	// Score the link first
	scoreResp, err := h.scraper.ScoreLink(scrapeReq.URL)
	if err != nil {
		h.scrapeManager.SetError(id, fmt.Sprintf("Failed to score URL: %v", err))
		h.scrapeManager.UpdateStatus(id, scrapemanager.StatusFailed)
		return
	}

	h.scrapeManager.UpdateProgress(id, 30)

	// Check if score meets threshold
	if scoreResp.Score.Score < h.linkScoreThreshold {
		// Score is below threshold - save scoring metadata only
		controllerID := uuid.New().String()
		record := &storage.Request{
			ID:         controllerID,
			CreatedAt:  time.Now().UTC(),
			SourceType: "url",
			SourceURL:  &scrapeReq.URL,
			Tags:       scoreResp.Score.Categories,
			Metadata: map[string]interface{}{
				"link_score": map[string]interface{}{
					"score":                scoreResp.Score.Score,
					"reason":               scoreResp.Score.Reason,
					"categories":           scoreResp.Score.Categories,
					"is_recommended":       scoreResp.Score.IsRecommended,
					"malicious_indicators": scoreResp.Score.MaliciousIndicators,
				},
				"below_threshold": true,
				"threshold":       h.linkScoreThreshold,
			},
		}

		if err := h.storage.SaveRequest(record); err != nil {
			h.scrapeManager.SetError(id, fmt.Sprintf("Failed to save request: %v", err))
			h.scrapeManager.UpdateStatus(id, scrapemanager.StatusFailed)
			return
		}

		h.scrapeManager.SetResult(id, record.ID)
		h.scrapeManager.UpdateProgress(id, 100)
		h.scrapeManager.UpdateStatus(id, scrapemanager.StatusCompleted)
		return
	}

	h.scrapeManager.UpdateProgress(id, 50)

	// Proceed with full scraping
	scraperResp, err := h.scraper.Scrape(scrapeReq.URL)
	if err != nil {
		h.scrapeManager.SetError(id, fmt.Sprintf("Failed to scrape URL: %v", err))
		h.scrapeManager.UpdateStatus(id, scrapemanager.StatusFailed)
		return
	}

	h.scrapeManager.UpdateProgress(id, 70)

	// Analyze the text
	analyzerResp, err := h.textAnalyzer.Analyze(scraperResp.Content)
	if err != nil {
		h.scrapeManager.SetError(id, fmt.Sprintf("Failed to analyze text: %v", err))
		h.scrapeManager.UpdateStatus(id, scrapemanager.StatusFailed)
		return
	}

	h.scrapeManager.UpdateProgress(id, 90)

	// Build combined metadata
	scraperMetadata := make(map[string]interface{})
	scraperMetadata["title"] = scraperResp.Title
	scraperMetadata["content"] = scraperResp.Content
	scraperMetadata["url"] = scraperResp.URL

	if scraperResp.Metadata != nil {
		for k, v := range scraperResp.Metadata {
			scraperMetadata[k] = v
		}
	}

	combinedMetadata := map[string]interface{}{
		"scraper_metadata":  scraperMetadata,
		"analyzer_metadata": analyzerResp.Metadata,
	}

	if scraperResp.Score != nil {
		combinedMetadata["link_score"] = map[string]interface{}{
			"score":                scraperResp.Score.Score,
			"reason":               scraperResp.Score.Reason,
			"categories":           scraperResp.Score.Categories,
			"is_recommended":       scraperResp.Score.IsRecommended,
			"malicious_indicators": scraperResp.Score.MaliciousIndicators,
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
	controllerID := uuid.New().String()
	record := &storage.Request{
		ID:               controllerID,
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &scrapeReq.URL,
		ScraperUUID:      &scraperResp.ID,
		TextAnalyzerUUID: analyzerResp.ID,
		Tags:             analyzerResp.GetTags(),
		Metadata:         combinedMetadata,
	}

	if err := h.storage.SaveRequest(record); err != nil {
		h.scrapeManager.SetError(id, fmt.Sprintf("Failed to save request: %v", err))
		h.scrapeManager.UpdateStatus(id, scrapemanager.StatusFailed)
		return
	}

	// Mark as completed
	h.scrapeManager.SetResult(id, record.ID)
	h.scrapeManager.UpdateProgress(id, 100)
	h.scrapeManager.UpdateStatus(id, scrapemanager.StatusCompleted)
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
