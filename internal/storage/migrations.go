package storage

import (
	"database/sql"
	"fmt"
	"log"
)

// Migration represents a single database migration
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// migrations contains all database migrations in order
var migrations = []Migration{
	{
		Version: 1,
		Name:    "initial_schema",
		SQL: `
			CREATE TABLE IF NOT EXISTS schema_version (
				version INTEGER PRIMARY KEY,
				applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);

			CREATE TABLE IF NOT EXISTS requests (
				id TEXT PRIMARY KEY,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
				source_type TEXT NOT NULL,
				source_url TEXT,
				scraper_uuid TEXT,
				textanalyzer_uuid TEXT NOT NULL,
				tags_json TEXT,
				metadata_json TEXT
			);

			CREATE INDEX IF NOT EXISTS idx_requests_created_at ON requests(created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_requests_scraper_uuid ON requests(scraper_uuid);
			CREATE INDEX IF NOT EXISTS idx_requests_textanalyzer_uuid ON requests(textanalyzer_uuid);
		`,
	},
	{
		Version: 2,
		Name:    "add_tags_table",
		SQL: `
			CREATE TABLE IF NOT EXISTS tags (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				request_id TEXT NOT NULL,
				tag TEXT NOT NULL,
				FOREIGN KEY (request_id) REFERENCES requests(id) ON DELETE CASCADE
			);

			CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);
			CREATE INDEX IF NOT EXISTS idx_tags_request_id ON tags(request_id);
		`,
	},
	{
		Version: 3,
		Name:    "add_effective_date",
		SQL: `
			-- Add effective_date column to requests table
			ALTER TABLE requests ADD COLUMN effective_date TIMESTAMP;

			-- Create index on effective_date for efficient timeline queries
			CREATE INDEX IF NOT EXISTS idx_requests_effective_date ON requests(effective_date DESC);

			-- Populate effective_date for existing records using the same logic as timeline
			-- This uses the date precedence: publish_date -> published_date -> additional_metadata.date -> created_at
			UPDATE requests
			SET effective_date = COALESCE(
				json_extract(metadata_json, '$.scraper_metadata.publish_date'),
				json_extract(metadata_json, '$.scraper_metadata.published_date'),
				json_extract(metadata_json, '$.additional_metadata.publish_date'),
				json_extract(metadata_json, '$.additional_metadata.published_date'),
				json_extract(metadata_json, '$.additional_metadata.date'),
				created_at
			);
		`,
	},
	{
		Version: 4,
		Name:    "add_slug_for_seo",
		SQL: `
			-- Add slug column to requests table for SEO-friendly URLs
			ALTER TABLE requests ADD COLUMN slug TEXT;

			-- Create unique index on slug for fast lookups
			CREATE UNIQUE INDEX IF NOT EXISTS idx_requests_slug ON requests(slug) WHERE slug IS NOT NULL;
		`,
	},
	{
		Version: 5,
		Name:    "add_seo_enabled",
		SQL: `
			-- Add seo_enabled column to requests table to allow toggling SEO pages per document
			ALTER TABLE requests ADD COLUMN seo_enabled INTEGER NOT NULL DEFAULT 1;

			-- Create index on seo_enabled for filtering
			CREATE INDEX IF NOT EXISTS idx_requests_seo_enabled ON requests(seo_enabled);
		`,
	},
}

// RunMigrations executes all pending migrations
func RunMigrations(db *sql.DB) error {
	log.Println("Creating schema_version table...")
	// Create schema_version table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	log.Println("Checking current schema version...")
	// Get current version
	var currentVersion int
	err = db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}
	log.Printf("Current schema version: %d", currentVersion)

	// Apply pending migrations
	for _, migration := range migrations {
		if migration.Version <= currentVersion {
			log.Printf("Skipping migration %d (already applied)", migration.Version)
			continue
		}

		log.Printf("Applying migration %d: %s", migration.Version, migration.Name)
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.Version, err)
		}

		// Execute migration SQL
		_, err = tx.Exec(migration.SQL)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %d (%s): %w", migration.Version, migration.Name, err)
		}

		// Record migration
		_, err = tx.Exec("INSERT INTO schema_version (version) VALUES (?)", migration.Version)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}

		log.Printf("✓ Applied migration %d: %s", migration.Version, migration.Name)
	}

	log.Println("All migrations complete")
	return nil
}
