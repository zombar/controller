package storage

import (
	"database/sql"
	"fmt"
	"log"
)

// PostgreSQL-specific migrations
// These migrations are designed for PostgreSQL and use PostgreSQL-specific features like SERIAL, JSONB, etc.

var postgresMigrations = []Migration{
	{
		Version: 1,
		Name:    "initial_schema",
		SQL: `
			CREATE TABLE IF NOT EXISTS schema_version (
				version INTEGER PRIMARY KEY,
				applied_at TIMESTAMPTZ DEFAULT NOW()
			);

			CREATE TABLE IF NOT EXISTS requests (
				id TEXT PRIMARY KEY,
				created_at TIMESTAMPTZ DEFAULT NOW(),
				source_type TEXT NOT NULL,
				source_url TEXT,
				scraper_uuid TEXT,
				textanalyzer_uuid TEXT NOT NULL,
				tags_json TEXT,
				metadata_json JSONB
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
				id SERIAL PRIMARY KEY,
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
			ALTER TABLE requests ADD COLUMN IF NOT EXISTS effective_date TIMESTAMPTZ;

			-- Create index on effective_date for efficient timeline queries
			CREATE INDEX IF NOT EXISTS idx_requests_effective_date ON requests(effective_date DESC);

			-- Populate effective_date for existing records using PostgreSQL JSONB operators
			-- This uses the date precedence: publish_date -> published_date -> additional_metadata.date -> created_at
			UPDATE requests
			SET effective_date = COALESCE(
				(metadata_json->'scraper_metadata'->>'publish_date')::TIMESTAMPTZ,
				(metadata_json->'scraper_metadata'->>'published_date')::TIMESTAMPTZ,
				(metadata_json->'additional_metadata'->>'publish_date')::TIMESTAMPTZ,
				(metadata_json->'additional_metadata'->>'published_date')::TIMESTAMPTZ,
				(metadata_json->'additional_metadata'->>'date')::TIMESTAMPTZ,
				created_at
			)
			WHERE effective_date IS NULL;
		`,
	},
	{
		Version: 4,
		Name:    "add_slug_for_seo",
		SQL: `
			-- Add slug column to requests table for SEO-friendly URLs
			ALTER TABLE requests ADD COLUMN IF NOT EXISTS slug TEXT;

			-- Create unique partial index on slug for fast lookups (only non-NULL slugs)
			CREATE UNIQUE INDEX IF NOT EXISTS idx_requests_slug ON requests(slug) WHERE slug IS NOT NULL;
		`,
	},
	{
		Version: 5,
		Name:    "add_seo_enabled",
		SQL: `
			-- Add seo_enabled column to requests table to allow toggling SEO pages per document
			ALTER TABLE requests ADD COLUMN IF NOT EXISTS seo_enabled BOOLEAN NOT NULL DEFAULT true;

			-- Create index on seo_enabled for filtering
			CREATE INDEX IF NOT EXISTS idx_requests_seo_enabled ON requests(seo_enabled);
		`,
	},
	{
		Version: 6,
		Name:    "add_scrape_jobs_table",
		SQL: `
			-- Create table for tracking async scrape jobs (replacing in-memory manager)
			CREATE TABLE IF NOT EXISTS scrape_jobs (
				id TEXT PRIMARY KEY,
				url TEXT NOT NULL,
				extract_links BOOLEAN NOT NULL DEFAULT false,
				status TEXT NOT NULL CHECK(status IN ('queued', 'processing', 'completed', 'failed')),
				retries INTEGER NOT NULL DEFAULT 0,
				created_at TIMESTAMPTZ NOT NULL,
				updated_at TIMESTAMPTZ NOT NULL,
				completed_at TIMESTAMPTZ,
				error_message TEXT,
				result_request_id TEXT,
				asynq_task_id TEXT,
				FOREIGN KEY(result_request_id) REFERENCES requests(id) ON DELETE SET NULL
			);

			-- Indexes for efficient querying
			CREATE INDEX IF NOT EXISTS idx_scrape_jobs_status ON scrape_jobs(status);
			CREATE INDEX IF NOT EXISTS idx_scrape_jobs_created_at ON scrape_jobs(created_at DESC);
			CREATE INDEX IF NOT EXISTS idx_scrape_jobs_url ON scrape_jobs(url);
			CREATE INDEX IF NOT EXISTS idx_scrape_jobs_asynq_task_id ON scrape_jobs(asynq_task_id);
		`,
	},
	{
		Version: 7,
		Name:    "add_parent_job_and_depth",
		SQL: `
			-- Add parent_job_id and depth for hierarchical scrape jobs
			ALTER TABLE scrape_jobs ADD COLUMN IF NOT EXISTS parent_job_id TEXT;
			ALTER TABLE scrape_jobs ADD COLUMN IF NOT EXISTS depth INTEGER NOT NULL DEFAULT 0;

			-- Create index for parent lookup
			CREATE INDEX IF NOT EXISTS idx_scrape_jobs_parent_job_id ON scrape_jobs(parent_job_id);

			-- Add foreign key constraint (separate statement in PostgreSQL)
			DO $$
			BEGIN
				IF NOT EXISTS (
					SELECT 1 FROM pg_constraint WHERE conname = 'fk_scrape_jobs_parent'
				) THEN
					ALTER TABLE scrape_jobs ADD CONSTRAINT fk_scrape_jobs_parent
						FOREIGN KEY (parent_job_id) REFERENCES scrape_jobs(id) ON DELETE CASCADE;
				END IF;
			END $$;
		`,
	},
}

// RunPostgresMigrations executes all pending PostgreSQL migrations
func RunPostgresMigrations(db *sql.DB) error {
	log.Println("Creating schema_version table...")
	// Create schema_version table if it doesn't exist
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
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
	for _, migration := range postgresMigrations {
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

		// Record migration (use PostgreSQL $1 placeholder instead of ?)
		_, err = tx.Exec("INSERT INTO schema_version (version) VALUES ($1)", migration.Version)
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
