package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/storage"
)

// Handler contains all HTTP handlers
type Handler struct {
	storage      *storage.Storage
	scraper      *clients.ScraperClient
	textAnalyzer *clients.TextAnalyzerClient
}

// New creates a new Handler
func New(store *storage.Storage, scraper *clients.ScraperClient, textAnalyzer *clients.TextAnalyzerClient) *Handler {
	return &Handler{
		storage:      store,
		scraper:      scraper,
		textAnalyzer: textAnalyzer,
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

// ScrapeURL handles URL scraping and text analysis
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

	// Call scraper service
	scraperResp, err := h.scraper.Scrape(req.URL)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to scrape URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Call text analyzer service with the main text
	analyzerResp, err := h.textAnalyzer.Analyze(scraperResp.MainText)
	if err != nil {
		respondError(w, fmt.Sprintf("Failed to analyze text: %v", err), http.StatusInternalServerError)
		return
	}

	// Create controller request record
	controllerID := uuid.New().String()
	record := &storage.Request{
		ID:               controllerID,
		CreatedAt:        time.Now().UTC(),
		SourceType:       "url",
		SourceURL:        &req.URL,
		ScraperUUID:      &scraperResp.UUID,
		TextAnalyzerUUID: analyzerResp.UUID,
		Tags:             analyzerResp.Tags,
		Metadata: map[string]interface{}{
			"scraper_metadata":  scraperResp.Metadata,
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
		TextAnalyzerUUID: analyzerResp.UUID,
		Tags:             analyzerResp.Tags,
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
	id := r.URL.Path[len("/requests/"):]
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
