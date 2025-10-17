package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Storage handles all database operations
type Storage struct {
	db *sql.DB
}

// Request represents a controller request record
type Request struct {
	ID               string                 `json:"id"`
	CreatedAt        time.Time              `json:"created_at"`
	SourceType       string                 `json:"source_type"` // "url" or "text"
	SourceURL        *string                `json:"source_url,omitempty"`
	ScraperUUID      *string                `json:"scraper_uuid,omitempty"`
	TextAnalyzerUUID string                 `json:"textanalyzer_uuid"`
	Tags             []string               `json:"tags"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// New creates a new Storage instance and runs migrations
func New(databasePath string) (*Storage, error) {
	log.Printf("Opening database at: %s", databasePath)
	db, err := sql.Open("sqlite3", databasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

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

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert request record
	_, err = tx.Exec(`
		INSERT INTO requests (id, created_at, source_type, source_url, scraper_uuid, textanalyzer_uuid, tags_json, metadata_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ID, req.CreatedAt, req.SourceType, req.SourceURL, req.ScraperUUID, req.TextAnalyzerUUID, string(tagsJSON), string(metadataJSON))
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
	var tagsJSON, metadataJSON sql.NullString

	err := s.db.QueryRow(`
		SELECT id, created_at, source_type, source_url, scraper_uuid, textanalyzer_uuid, tags_json, metadata_json
		FROM requests
		WHERE id = ?
	`, id).Scan(&req.ID, &req.CreatedAt, &req.SourceType, &req.SourceURL, &req.ScraperUUID, &req.TextAnalyzerUUID, &tagsJSON, &metadataJSON)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("request not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query request: %w", err)
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

// ListRequests returns all requests ordered by creation time
func (s *Storage) ListRequests(limit, offset int) ([]*Request, error) {
	query := `
		SELECT id, created_at, source_type, source_url, scraper_uuid, textanalyzer_uuid, tags_json, metadata_json
		FROM requests
		ORDER BY created_at DESC
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
		var tagsJSON, metadataJSON sql.NullString

		err := rows.Scan(&req.ID, &req.CreatedAt, &req.SourceType, &req.SourceURL, &req.ScraperUUID, &req.TextAnalyzerUUID, &tagsJSON, &metadataJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to scan request: %w", err)
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
