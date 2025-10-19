package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/scraper_requests"
	"github.com/zombar/controller/internal/storage"
)

// Handler contains all HTTP handlers
type Handler struct {
	storage            *storage.Storage
	scraper            *clients.ScraperClient
	textAnalyzer       *clients.TextAnalyzerClient
	linkScoreThreshold float64
	scrapeRequests     *scraper_requests.Manager
}

// New creates a new Handler
func New(store *storage.Storage, scraper *clients.ScraperClient, textAnalyzer *clients.TextAnalyzerClient, linkScoreThreshold float64) *Handler {
	return &Handler{
		storage:            store,
		scraper:            scraper,
		textAnalyzer:       textAnalyzer,
		linkScoreThreshold: linkScoreThreshold,
		scrapeRequests:     scraper_requests.NewManager(),
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

	// Score meets or exceeds threshold - proceed with full scraping
	scraperResp, err := h.scraper.Scrape(req.URL)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to scrape URL: %v", err), http.StatusInternalServerError)
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

	// Analyze the content (skip for image URLs)
	var analyzerResp *clients.TextAnalyzerResponse
	if !isImageURL {
		analyzerResp, err = h.textAnalyzer.Analyze(scraperResp.Content)
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

	record := &storage.Request{
		ID:               controllerID,
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &req.URL,
		ScraperUUID:      &scraperResp.ID,
		TextAnalyzerUUID: analyzerUUID,
		Tags:             tags,
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
			"original_text":     req.Text, // Store original submitted text
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
		if err := h.scraper.DeleteScrape(*record.ScraperUUID); err != nil {
			log.Printf("Warning: Failed to delete scrape %s: %v", *record.ScraperUUID, err)
		}
	}

	if record.TextAnalyzerUUID != "" {
		if err := h.textAnalyzer.DeleteAnalysis(record.TextAnalyzerUUID); err != nil {
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
	if err := h.scraper.DeleteImage(imageID); err != nil {
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

	// Add tombstone_datetime to metadata
	if record.Metadata == nil {
		record.Metadata = make(map[string]interface{})
	}
	record.Metadata["tombstone_datetime"] = time.Now().UTC().Format(time.RFC3339)

	// Update the request in storage
	if err := h.storage.UpdateRequestMetadata(id, record.Metadata); err != nil {
		respondError(w, fmt.Sprintf("Failed to update request: %v", err), http.StatusInternalServerError)
		return
	}

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
	if err := h.scraper.TombstoneImage(imageID); err != nil {
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
	if err := h.scraper.UntombstoneImage(imageID); err != nil {
		respondError(w, fmt.Sprintf("Failed to untombstone image: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, map[string]string{"message": "Image tombstone removed successfully"}, http.StatusOK)
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

// GetDocumentImages retrieves images associated with a document's scraper UUID
func (h *Handler) GetDocumentImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract scraper UUID from URL path
	scrapeID := r.URL.Path[len("/api/documents/"):len(r.URL.Path)-len("/images")]
	if scrapeID == "" {
		respondError(w, "Scraper UUID is required", http.StatusBadRequest)
		return
	}

	// Call scraper service to get images by scrape ID
	searchResp, err := h.scraper.GetImagesByScrapeID(scrapeID)
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
	image, err := h.scraper.GetImageByID(imageID)
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

	// Create or get existing scrape request
	scrapeReq, isNew := h.scrapeRequests.Create(req.URL)

	// If new, start background scraping
	if isNew {
		go h.processScrapeRequest(scrapeReq.ID, req.URL)
	}

	respondJSON(w, scrapeReq, http.StatusOK)
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

	requests := h.scrapeRequests.List()

	response := map[string]interface{}{
		"requests": requests,
		"count":    len(requests),
	}

	respondJSON(w, response, http.StatusOK)
}

// GetScrapeRequest returns a specific scrape request by ID
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

	req, ok := h.scrapeRequests.Get(id)
	if !ok {
		respondError(w, "Scrape request not found", http.StatusNotFound)
		return
	}

	respondJSON(w, req, http.StatusOK)
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

	req, ok := h.scrapeRequests.Get(id)
	if !ok {
		respondError(w, "Scrape request not found", http.StatusNotFound)
		return
	}

	// Only allow retrying failed requests
	if req.Status != scraper_requests.StatusFailed {
		respondError(w, "Can only retry failed requests", http.StatusBadRequest)
		return
	}

	// Reset request state
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusPending, 0)
	req.ErrorMessage = ""

	// Start background scraping
	go h.processScrapeRequest(id, req.URL)

	// Get updated request
	updatedReq, _ := h.scrapeRequests.Get(id)
	respondJSON(w, updatedReq, http.StatusOK)
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

	if !h.scrapeRequests.Delete(id) {
		respondError(w, "Scrape request not found", http.StatusNotFound)
		return
	}

	respondJSON(w, map[string]string{"status": "deleted"}, http.StatusOK)
}

// processScrapeRequest processes a scrape request in the background
func (h *Handler) processScrapeRequest(id, url string) {
	// Update status to processing
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 10)

	// Score the URL first
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 30)
	scoreResp, err := h.scraper.ScoreLink(url)
	if err != nil {
		h.scrapeRequests.SetFailed(id, fmt.Sprintf("Failed to score link: %v", err))
		return
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
	if !isImageURL && scoreResp.Score.Score < h.linkScoreThreshold {
		h.scrapeRequests.SetFailed(id, fmt.Sprintf("URL score (%.2f) below threshold (%.2f)", scoreResp.Score.Score, h.linkScoreThreshold))
		return
	}

	// Scrape the URL
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 50)
	scrapeResp, err := h.scraper.Scrape(url)
	if err != nil {
		h.scrapeRequests.SetFailed(id, fmt.Sprintf("Failed to scrape: %v", err))
		return
	}

	// Build scraper metadata from the scraper response
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 70)
	scraperMetadata := make(map[string]interface{})
	scraperMetadata["title"] = scrapeResp.Title
	scraperMetadata["content"] = scrapeResp.Content
	scraperMetadata["url"] = scrapeResp.URL

	// Also include fields from the scraper's Metadata (description, keywords, etc.)
	if scrapeResp.Metadata != nil {
		for k, v := range scrapeResp.Metadata {
			scraperMetadata[k] = v
		}
	}

	// Analyze the content (skip for image URLs)
	var analyzeResp *clients.TextAnalyzerResponse
	if !isImageURL {
		h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 80)
		analyzeResp, err = h.textAnalyzer.Analyze(scrapeResp.Content)
		if err != nil {
			h.scrapeRequests.SetFailed(id, fmt.Sprintf("Failed to analyze: %v", err))
			return
		}
	}

	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 90)

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
		// Fallback to preliminary score if scraper didn't return one
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

	// Get tags and analyzer UUID (handle nil for image URLs)
	var tags []string
	var analyzerUUID string
	if analyzeResp != nil {
		tags = analyzeResp.GetTags()
		analyzerUUID = analyzeResp.ID
	} else {
		// For image URLs, use categories from link score as tags
		if scrapeResp.Score != nil {
			tags = scrapeResp.Score.Categories
		}
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
	}

	if err := h.storage.SaveRequest(req); err != nil {
		h.scrapeRequests.SetFailed(id, fmt.Sprintf("Failed to save: %v", err))
		return
	}

	// Mark as completed
	h.scrapeRequests.SetCompleted(id, requestID)
	log.Printf("Scrape request %s completed successfully, result saved as %s", id, requestID)
}

// processTextAnalysisRequest processes a text analysis request in the background
func (h *Handler) processTextAnalysisRequest(id, text string) {
	// Update status to processing
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 30)

	// Analyze the text
	analyzeResp, err := h.textAnalyzer.Analyze(text)
	if err != nil {
		h.scrapeRequests.SetFailed(id, fmt.Sprintf("Failed to analyze: %v", err))
		return
	}

	// Update progress
	h.scrapeRequests.UpdateStatus(id, scraper_requests.StatusProcessing, 90)

	// Save to database
	requestID := uuid.New().String()
	req := &storage.Request{
		ID:               requestID,
		CreatedAt:        time.Now(),
		SourceType:       "text",
		TextAnalyzerUUID: analyzeResp.ID,
		Tags:             analyzeResp.GetTags(),
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
