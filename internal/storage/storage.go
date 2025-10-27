package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// Storage handles all database operations
type Storage struct {
	db *sql.DB
}

// Request represents a controller request record
type Request struct {
	ID               string                 `json:"id"`
	CreatedAt        time.Time              `json:"created_at"`
	EffectiveDate    time.Time              `json:"effective_date"` // Normalized date from metadata or created_at
	SourceType       string                 `json:"source_type"`    // "url" or "text"
	SourceURL        *string                `json:"source_url,omitempty"`
	ScraperUUID      *string                `json:"scraper_uuid,omitempty"`
	TextAnalyzerUUID string                 `json:"textanalyzer_uuid"`
	Tags             []string               `json:"tags"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Slug             *string                `json:"slug,omitempty"`     // SEO-friendly URL slug
	SEOEnabled       bool                   `json:"seo_enabled"`        // Whether the SEO page is enabled for this document
}

// extractEffectiveDate extracts the effective date from metadata following a precedence order.
// This is the single source of truth for date extraction logic (DRY principle).
// Precedence: scraper_metadata.publish_date -> scraper_metadata.published_date ->
//            additional_metadata.publish_date -> additional_metadata.published_date ->
//            additional_metadata.date -> fallback (created_at)
func extractEffectiveDate(metadata map[string]interface{}, fallback time.Time) time.Time {
	// Common date formats to try
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	// Helper to try parsing a date string with multiple formats
	tryParseDate := func(dateStr string) (time.Time, bool) {
		for _, format := range formats {
			if t, err := time.Parse(format, dateStr); err == nil {
				return t, true
			}
		}
		return time.Time{}, false
	}

	// Helper to extract string from nested map path
	getNestedString := func(path ...string) (string, bool) {
		current := metadata
		for i, key := range path {
			if i == len(path)-1 {
				// Last key - try to get the string value
				if val, ok := current[key].(string); ok {
					return val, true
				}
				return "", false
			}
			// Intermediate key - navigate deeper
			if next, ok := current[key].(map[string]interface{}); ok {
				current = next
			} else {
				return "", false
			}
		}
		return "", false
	}

	// Try each path in precedence order
	paths := [][]string{
		{"scraper_metadata", "publish_date"},
		{"scraper_metadata", "published_date"},
		{"additional_metadata", "publish_date"},
		{"additional_metadata", "published_date"},
		{"additional_metadata", "date"},
	}

	for _, path := range paths {
		if dateStr, ok := getNestedString(path...); ok && dateStr != "" {
			if t, ok := tryParseDate(dateStr); ok {
				return t
			}
		}
	}

	// No valid date found in metadata, use fallback
	return fallback
}

// New creates a new Storage instance and runs migrations
func New(databasePath string) (*Storage, error) {
	log.Printf("Opening database at: %s", databasePath)
	// Add connection parameters for better concurrency handling
	// WAL mode allows concurrent reads and writes
	// Busy timeout prevents immediate failures on concurrent access
	connStr := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL", databasePath)
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Limit connection pool to 1 to prevent transaction conflicts
	db.SetMaxOpenConns(1)

	log.Println("Testing database connection...")
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Enabling foreign keys...")
	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	log.Println("Running migrations...")
	// Run migrations
	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	log.Println("Database initialization complete")
	return &Storage{db: db}, nil
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// DB returns the underlying database connection for metrics collection
func (s *Storage) DB() *sql.DB {
	return s.db
}

// SaveRequest saves a new request record
func (s *Storage) SaveRequest(req *Request) error {
	tagsJSON, err := json.Marshal(req.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	var metadataJSON []byte
	if req.Metadata != nil {
		metadataJSON, err = json.Marshal(req.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
	}

	// Extract effective date from metadata (DRY: single source of truth)
	// If not already set, extract from metadata with created_at as fallback
	if req.EffectiveDate.IsZero() {
		req.EffectiveDate = extractEffectiveDate(req.Metadata, req.CreatedAt)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert request record with effective_date, slug, and seo_enabled
	// Format effective_date as RFC3339 string for consistent SQLite storage
	_, err = tx.Exec(`
		INSERT INTO requests (id, created_at, effective_date, source_type, source_url, scraper_uuid, textanalyzer_uuid, tags_json, metadata_json, slug, seo_enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ID, req.CreatedAt, req.EffectiveDate.Format(time.RFC3339), req.SourceType, req.SourceURL, req.ScraperUUID, req.TextAnalyzerUUID, string(tagsJSON), string(metadataJSON), req.Slug, req.SEOEnabled)
	if err != nil {
		return fmt.Errorf("failed to insert request: %w", err)
	}

	// Insert individual tags for searching
	if len(req.Tags) > 0 {
		stmt, err := tx.Prepare("INSERT INTO tags (request_id, tag) VALUES (?, ?)")
		if err != nil {
			return fmt.Errorf("failed to prepare tag insert: %w", err)
		}
		defer stmt.Close()

		for _, tag := range req.Tags {
			if _, err := stmt.Exec(req.ID, tag); err != nil {
				return fmt.Errorf("failed to insert tag: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetRequest retrieves a request by ID
func (s *Storage) GetRequest(id string) (*Request, error) {
	var req Request
	var tagsJSON, metadataJSON, effectiveDateStr, slug sql.NullString

	err := s.db.QueryRow(`
		SELECT id, created_at, effective_date, source_type, source_url, scraper_uuid, textanalyzer_uuid, tags_json, metadata_json, slug, seo_enabled
		FROM requests
		WHERE id = ?
	`, id).Scan(&req.ID, &req.CreatedAt, &effectiveDateStr, &req.SourceType, &req.SourceURL, &req.ScraperUUID, &req.TextAnalyzerUUID, &tagsJSON, &metadataJSON, &slug, &req.SEOEnabled)

	// Parse effective_date from string
	if effectiveDateStr.Valid && effectiveDateStr.String != "" {
		if parsedDate, err := time.Parse(time.RFC3339, effectiveDateStr.String); err == nil {
			req.EffectiveDate = parsedDate
		}
	}

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("request not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query request: %w", err)
	}

	// Set slug if present
	if slug.Valid {
		slugStr := slug.String
		req.Slug = &slugStr
	}

	// Unmarshal tags
	if tagsJSON.Valid {
		if err := json.Unmarshal([]byte(tagsJSON.String), &req.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}

	// Unmarshal metadata
	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &req.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &req, nil
}

// DeleteRequest deletes a request and all associated tags
func (s *Storage) DeleteRequest(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete associated tags first (due to foreign key constraint)
	_, err = tx.Exec("DELETE FROM tags WHERE request_id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete tags: %w", err)
	}

	// Delete the request
	result, err := tx.Exec("DELETE FROM requests WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete request: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("request not found")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// UpdateRequestMetadata updates the metadata field of a request
func (s *Storage) UpdateRequestMetadata(id string, metadata map[string]interface{}) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	result, err := s.db.Exec(`
		UPDATE requests
		SET metadata_json = ?
		WHERE id = ?
	`, string(metadataJSON), id)
	if err != nil {
		return fmt.Errorf("failed to update request metadata: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("request not found")
	}

	return nil
}

// SearchByTags searches for requests by tags with fuzzy matching
func (s *Storage) SearchByTags(searchTags []string, fuzzy bool) ([]string, error) {
	if len(searchTags) == 0 {
		return []string{}, nil
	}

	var conditions []string
	var args []interface{}

	for _, tag := range searchTags {
		if fuzzy {
			conditions = append(conditions, "tag LIKE ?")
			args = append(args, "%"+tag+"%")
		} else {
			conditions = append(conditions, "tag = ?")
			args = append(args, tag)
		}
	}

	query := fmt.Sprintf(`
		SELECT DISTINCT request_id
		FROM tags
		WHERE %s
		ORDER BY request_id
	`, strings.Join(conditions, " OR "))

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search tags: %w", err)
	}
	defer rows.Close()

	var requestIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan request ID: %w", err)
		}
		requestIDs = append(requestIDs, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return requestIDs, nil
}

// FilterOptions contains all filter parameters for requests
type FilterOptions struct {
	Tags       []string
	Fuzzy      bool
	DateStart  *time.Time
	DateEnd    *time.Time
	SourceType *string
	Limit      int
	Offset     int
}

// FilterRequests filters requests based on multiple criteria
func (s *Storage) FilterRequests(opts FilterOptions) ([]*Request, error) {
	// Build the WHERE clause dynamically
	var whereClauses []string
	var args []interface{}

	// Date range filter - use effective_date column (normalized at ingestion time)
	// effective_date is stored as RFC3339 string, so we need to format the time.Time values
	if opts.DateStart != nil {
		whereClauses = append(whereClauses, "r.effective_date >= ?")
		args = append(args, opts.DateStart.Format(time.RFC3339))
	}
	if opts.DateEnd != nil {
		whereClauses = append(whereClauses, "r.effective_date <= ?")
		args = append(args, opts.DateEnd.Format(time.RFC3339))
	}

	// Source type filter
	if opts.SourceType != nil {
		whereClauses = append(whereClauses, "r.source_type = ?")
		args = append(args, *opts.SourceType)
	}

	// Build base query
	var query string
	if len(opts.Tags) > 0 {
		// If tags are specified, join with tags table
		var tagConditions []string
		for _, tag := range opts.Tags {
			if opts.Fuzzy {
				tagConditions = append(tagConditions, "t.tag LIKE ?")
				args = append(args, "%"+tag+"%")
			} else {
				tagConditions = append(tagConditions, "t.tag = ?")
				args = append(args, tag)
			}
		}

		// Use INNER JOIN to filter by tags
		query = `
			SELECT DISTINCT r.id, r.created_at, r.effective_date, r.source_type, r.source_url, r.scraper_uuid, r.textanalyzer_uuid, r.tags_json, r.metadata_json
			FROM requests r
			INNER JOIN tags t ON r.id = t.request_id
			WHERE (` + strings.Join(tagConditions, " OR ") + `)`

		// Add other WHERE clauses
		if len(whereClauses) > 0 {
			query += " AND " + strings.Join(whereClauses, " AND ")
		}
	} else {
		// No tags specified, query requests table directly
		query = `
			SELECT id, created_at, effective_date, source_type, source_url, scraper_uuid, textanalyzer_uuid, tags_json, metadata_json
			FROM requests r`

		if len(whereClauses) > 0 {
			query += " WHERE " + strings.Join(whereClauses, " AND ")
		}
	}

	// Add ORDER BY and pagination - order by effective date
	query += " ORDER BY r.effective_date DESC"
	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}
	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to filter requests: %w", err)
	}
	defer rows.Close()

	var requests []*Request
	for rows.Next() {
		var req Request
		var tagsJSON, metadataJSON, effectiveDateStr sql.NullString

		err := rows.Scan(&req.ID, &req.CreatedAt, &effectiveDateStr, &req.SourceType, &req.SourceURL, &req.ScraperUUID, &req.TextAnalyzerUUID, &tagsJSON, &metadataJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request: %w", err)
		}

		// Parse effective_date from string
		if effectiveDateStr.Valid && effectiveDateStr.String != "" {
			if parsedDate, err := time.Parse(time.RFC3339, effectiveDateStr.String); err == nil {
				req.EffectiveDate = parsedDate
			} else {
				// If RFC3339 fails, try other formats
				formats := []string{time.RFC3339Nano, "2006-01-02 15:04:05"}
				for _, format := range formats {
					if parsedDate, err := time.Parse(format, effectiveDateStr.String); err == nil {
						req.EffectiveDate = parsedDate
						break
					}
				}
			}
		}

		if tagsJSON.Valid {
			if err := json.Unmarshal([]byte(tagsJSON.String), &req.Tags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
			}
		}

		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &req.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		requests = append(requests, &req)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return requests, nil
}

// ListRequests returns all requests ordered by creation time
func (s *Storage) ListRequests(limit, offset int) ([]*Request, error) {
	query := `
		SELECT id, created_at, effective_date, source_type, source_url, scraper_uuid, textanalyzer_uuid, tags_json, metadata_json, slug, seo_enabled
		FROM requests
		ORDER BY effective_date DESC
		LIMIT ? OFFSET ?
	`

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list requests: %w", err)
	}
	defer rows.Close()

	var requests []*Request
	for rows.Next() {
		var req Request
		var tagsJSON, metadataJSON, effectiveDateStr sql.NullString

		err := rows.Scan(&req.ID, &req.CreatedAt, &effectiveDateStr, &req.SourceType, &req.SourceURL, &req.ScraperUUID, &req.TextAnalyzerUUID, &tagsJSON, &metadataJSON, &req.Slug, &req.SEOEnabled)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request: %w", err)
		}

		// Parse effective_date from string
		if effectiveDateStr.Valid && effectiveDateStr.String != "" {
			if parsedDate, err := time.Parse(time.RFC3339, effectiveDateStr.String); err == nil {
				req.EffectiveDate = parsedDate
			}
		}

		if tagsJSON.Valid {
			if err := json.Unmarshal([]byte(tagsJSON.String), &req.Tags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
			}
		}

		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &req.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		requests = append(requests, &req)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return requests, nil
}

// GetTimelineExtents returns the earliest effective_date from all documents
// to determine the min date for timeline visualization.
//
// effective_date is normalized at ingestion time using extractEffectiveDate(),
// which follows the precedence: publish_date -> published_date -> additional_metadata.date -> created_at
//
// Returns nil if no requests exist in the database.
func (s *Storage) GetTimelineExtents() (*time.Time, error) {
	// Simple query using the pre-normalized effective_date column
	query := `SELECT MIN(effective_date) FROM requests`

	var earliestDateStr sql.NullString
	err := s.db.QueryRow(query).Scan(&earliestDateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get timeline extents: %w", err)
	}

	// No documents in database
	if !earliestDateStr.Valid || earliestDateStr.String == "" {
		return nil, nil
	}

	// Parse the date string
	parsedDate, err := time.Parse(time.RFC3339, earliestDateStr.String)
	if err != nil {
		return nil, fmt.Errorf("failed to parse earliest date: %w", err)
	}

	return &parsedDate, nil
}

// GenerateMockData generates 6 months of realistic historical data for testing
func (s *Storage) GenerateMockData() error {
	log.Println("Generating mock historical data...")

	// Check if we already have data
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM requests").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count existing requests: %w", err)
	}

	if count > 0 {
		log.Printf("Database already contains %d requests, skipping mock data generation", count)
		return nil
	}

	// Sample data for generating realistic entries
	sampleURLs := []string{
		"https://example.com/article/technology-trends-2024",
		"https://news.example.org/science/quantum-computing-breakthrough",
		"https://blog.example.net/programming/golang-best-practices",
		"https://research.example.edu/papers/artificial-intelligence",
		"https://docs.example.io/guides/docker-deployment",
		"https://medium.example.com/data-science/machine-learning-basics",
		"https://github.example.dev/projects/open-source-tools",
		"https://stackoverflow.example.com/questions/database-optimization",
		"https://arxiv.example.org/papers/distributed-systems",
		"https://dev.example.to/tutorials/kubernetes-intro",
	}

	sampleTags := [][]string{
		{"technology", "trends", "future"},
		{"science", "quantum", "research"},
		{"programming", "golang", "best-practices"},
		{"ai", "machine-learning", "research"},
		{"devops", "docker", "deployment"},
		{"data-science", "ml", "tutorial"},
		{"open-source", "tools", "development"},
		{"database", "optimization", "performance"},
		{"distributed-systems", "architecture", "scalability"},
		{"kubernetes", "containers", "cloud"},
	}

	sampleTitles := []string{
		"Technology Trends to Watch in 2024",
		"Breakthrough in Quantum Computing Research",
		"Go Programming Best Practices",
		"Advances in Artificial Intelligence",
		"Docker Deployment Strategies",
		"Machine Learning Fundamentals",
		"Top Open Source Development Tools",
		"Database Optimization Techniques",
		"Distributed Systems Architecture",
		"Getting Started with Kubernetes",
	}

	sampleAuthors := []string{
		"Dr. Jane Smith",
		"Prof. John Doe",
		"Alice Johnson",
		"Bob Wilson",
		"Carol Martinez",
		"David Chen",
		"Emma Brown",
		"Frank Taylor",
		"Grace Lee",
		"Henry Anderson",
	}

	// Generate 600 mock requests spanning 6 months (180 days)
	// This averages to ~3.3 documents per day
	now := time.Now()
	mockCount := 600
	daysToGenerate := 180.0
	rand.Seed(now.UnixNano())

	for i := 0; i < mockCount; i++ {
		// Random timestamp within the last 6 months (180 days)
		daysAgo := rand.Float64() * daysToGenerate
		hoursAgo := daysAgo * 24
		createdAt := now.Add(-time.Duration(hoursAgo) * time.Hour)

		// Randomly choose between URL scrape (70%) and text ingestion (30%)
		isURL := rand.Float64() < 0.7
		idx := rand.Intn(len(sampleURLs))

		var sourceType string
		var sourceURL *string
		var scraperUUID *string

		if isURL {
			sourceType = "url"
			url := sampleURLs[idx]
			sourceURL = &url
			scraperUUIDStr := uuid.New().String()
			scraperUUID = &scraperUUIDStr
		} else {
			sourceType = "text"
		}

		// Generate metadata with varying quality scores and occasional tombstones
		metadata := make(map[string]interface{})

		// Link score (quality): higher quality more likely
		qualityScore := 0.3 + rand.Float64()*0.7 // Range 0.3-1.0

		metadata["link_score"] = map[string]interface{}{
			"score": qualityScore,
		}

		// Add scraper metadata for URL sources
		if isURL {
			scraperMetadata := map[string]interface{}{
				"title":        sampleTitles[idx],
				"author":       sampleAuthors[rand.Intn(len(sampleAuthors))],
				"publish_date": createdAt.Format(time.RFC3339),
			}

			// 30% chance of having images
			if rand.Float64() < 0.3 {
				scraperMetadata["images"] = []map[string]interface{}{
					{
						"url":      fmt.Sprintf("https://example.com/images/%s.jpg", uuid.New().String()[:8]),
						"alt_text": sampleTitles[idx],
					},
				}
			}

			metadata["scraper_metadata"] = scraperMetadata
		}

		// 15% chance of being tombstoned
		if rand.Float64() < 0.15 {
			tombstoneTime := createdAt.Add(time.Duration(rand.Intn(72)) * time.Hour) // Tombstoned 0-3 days after creation
			metadata["tombstone_datetime"] = tombstoneTime.Format(time.RFC3339)
		}

		// Generate slug for URL-based requests
		var slug *string
		if isURL {
			// Use title as slug base
			slugBase := sampleTitles[idx]
			// Simple slug generation (lowercase, replace spaces with hyphens, remove special chars)
			generatedSlug := strings.ToLower(slugBase)
			generatedSlug = strings.ReplaceAll(generatedSlug, " ", "-")
			// Add random suffix to ensure uniqueness
			generatedSlug = fmt.Sprintf("%s-%d", generatedSlug, rand.Intn(10000))
			slug = &generatedSlug
		}

		// SEO enabled by default (90% of documents)
		seoEnabled := rand.Float64() < 0.9

		// Create request
		req := &Request{
			ID:               uuid.New().String(),
			CreatedAt:        createdAt,
			SourceType:       sourceType,
			SourceURL:        sourceURL,
			ScraperUUID:      scraperUUID,
			TextAnalyzerUUID: uuid.New().String(),
			Tags:             sampleTags[idx],
			Metadata:         metadata,
			Slug:             slug,
			SEOEnabled:       seoEnabled,
		}

		if err := s.SaveRequest(req); err != nil {
			return fmt.Errorf("failed to save mock request: %w", err)
		}
	}

	log.Printf("✓ Generated %d mock requests spanning %.0f days (6 months)", mockCount, daysToGenerate)
	return nil
}

// UpdateSEOEnabled updates the SEO enabled status of a request
func (s *Storage) UpdateSEOEnabled(id string, enabled bool) error {
	result, err := s.db.Exec(`
		UPDATE requests
		SET seo_enabled = ?
		WHERE id = ?
	`, enabled, id)
	if err != nil {
		return fmt.Errorf("failed to update SEO enabled status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("request not found")
	}

	return nil
}

// GetRequestBySlug retrieves a request by its slug
func (s *Storage) GetRequestBySlug(slug string) (*Request, error) {
	query := `
		SELECT id, created_at, effective_date, source_type, source_url, scraper_uuid, textanalyzer_uuid, tags_json, metadata_json, slug, seo_enabled
		FROM requests
		WHERE slug = ?
		LIMIT 1
	`

	var req Request
	var tagsJSON, metadataJSON, effectiveDateStr sql.NullString

	err := s.db.QueryRow(query, slug).Scan(&req.ID, &req.CreatedAt, &effectiveDateStr, &req.SourceType, &req.SourceURL, &req.ScraperUUID, &req.TextAnalyzerUUID, &tagsJSON, &metadataJSON, &req.Slug, &req.SEOEnabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query request by slug: %w", err)
	}

	// Parse effective_date from string
	if effectiveDateStr.Valid && effectiveDateStr.String != "" {
		if parsedDate, err := time.Parse(time.RFC3339, effectiveDateStr.String); err == nil {
			req.EffectiveDate = parsedDate
		}
	}

	if tagsJSON.Valid {
		if err := json.Unmarshal([]byte(tagsJSON.String), &req.Tags); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
		}
	}

	if metadataJSON.Valid && metadataJSON.String != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &req.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return &req, nil
}

// UpdateRequestTags updates the tags for a specific request
func (s *Storage) UpdateRequestTags(id string, tags []string) error {
	// Marshal tags to JSON
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	// Begin transaction to ensure atomicity
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update tags in database
	result, err := tx.Exec("UPDATE requests SET tags_json = ? WHERE id = ?", string(tagsJSON), id)
	if err != nil {
		return fmt.Errorf("failed to update tags: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("request not found")
	}

	// Delete existing tag associations
	if _, err := tx.Exec("DELETE FROM tags WHERE request_id = ?", id); err != nil {
		return fmt.Errorf("failed to delete old tag associations: %w", err)
	}

	// Insert new tag associations
	for _, tag := range tags {
		if _, err := tx.Exec("INSERT INTO tags (request_id, tag) VALUES (?, ?)", id, tag); err != nil {
			return fmt.Errorf("failed to insert tag association: %w", err)
		}
	}

	// Check if tags contain 'low-quality' or 'sparse-content' and apply 90-day tombstone
	hasLowQuality := false
	for _, tag := range tags {
		if tag == "low-quality" || tag == "sparse-content" {
			hasLowQuality = true
			break
		}
	}

	if hasLowQuality {
		// Fetch current metadata
		var metadataJSON string
		err := tx.QueryRow("SELECT metadata_json FROM requests WHERE id = ?", id).Scan(&metadataJSON)
		if err != nil {
			return fmt.Errorf("failed to fetch metadata: %w", err)
		}

		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			return fmt.Errorf("failed to unmarshal metadata: %w", err)
		}

		// Add 90-day tombstone
		tombstoneTime := time.Now().UTC().Add(90 * 24 * time.Hour)
		metadata["tombstone_datetime"] = tombstoneTime.Format(time.RFC3339)
		metadata["tombstone_reason"] = "auto-tombstone: low-quality or sparse-content tag"

		// Marshal updated metadata
		updatedMetadataJSON, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal updated metadata: %w", err)
		}

		// Update metadata in database
		_, err = tx.Exec("UPDATE requests SET metadata_json = ? WHERE id = ?", string(updatedMetadataJSON), id)
		if err != nil {
			return fmt.Errorf("failed to update metadata with tombstone: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
