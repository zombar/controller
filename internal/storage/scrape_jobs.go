package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// ScrapeJob represents an async scrape job tracked in the database
type ScrapeJob struct {
	ID              string     `json:"id"`
	URL             string     `json:"url"`
	ExtractLinks    bool       `json:"extract_links"`
	Status          string     `json:"status"` // queued, processing, completed, failed
	Retries         int        `json:"retries"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	ResultRequestID *string    `json:"result_request_id,omitempty"`
	AsynqTaskID     string     `json:"asynq_task_id,omitempty"`
	ParentJobID     *string    `json:"parent_job_id,omitempty"`
	Depth           int        `json:"depth"`
	ChildJobs       []*ScrapeJob `json:"child_jobs,omitempty"`
}

// SaveScrapeJob inserts a new scrape job into the database
func (s *Storage) SaveScrapeJob(job *ScrapeJob) error {
	query := `
		INSERT INTO scrape_jobs (
			id, url, extract_links, status, retries,
			created_at, updated_at, completed_at,
			error_message, result_request_id, asynq_task_id,
			parent_job_id, depth
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := s.db.Exec(
		query,
		job.ID,
		job.URL,
		boolToInt(job.ExtractLinks),
		job.Status,
		job.Retries,
		job.CreatedAt,
		job.UpdatedAt,
		job.CompletedAt,
		job.ErrorMessage,
		job.ResultRequestID,
		job.AsynqTaskID,
		job.ParentJobID,
		job.Depth,
	)

	if err != nil {
		return fmt.Errorf("failed to save scrape job: %w", err)
	}

	return nil
}

// GetScrapeJob retrieves a scrape job by ID
func (s *Storage) GetScrapeJob(id string) (*ScrapeJob, error) {
	query := `
		SELECT
			id, url, extract_links, status, retries,
			created_at, updated_at, completed_at,
			error_message, result_request_id, asynq_task_id,
			parent_job_id, depth
		FROM scrape_jobs
		WHERE id = $1
	`

	job := &ScrapeJob{}
	var extractLinks int
	var completedAt sql.NullTime
	var errorMessage sql.NullString
	var resultRequestID sql.NullString
	var asynqTaskID sql.NullString
	var parentJobID sql.NullString

	err := s.db.QueryRow(query, id).Scan(
		&job.ID,
		&job.URL,
		&extractLinks,
		&job.Status,
		&job.Retries,
		&job.CreatedAt,
		&job.UpdatedAt,
		&completedAt,
		&errorMessage,
		&resultRequestID,
		&asynqTaskID,
		&parentJobID,
		&job.Depth,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get scrape job: %w", err)
	}

	job.ExtractLinks = extractLinks != 0
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	if errorMessage.Valid {
		job.ErrorMessage = errorMessage.String
	}
	if resultRequestID.Valid {
		job.ResultRequestID = &resultRequestID.String
	}
	if asynqTaskID.Valid {
		job.AsynqTaskID = asynqTaskID.String
	}
	if parentJobID.Valid {
		job.ParentJobID = &parentJobID.String
	}

	return job, nil
}

// ListScrapeJobs retrieves scrape jobs with pagination (only top-level, no parent)
func (s *Storage) ListScrapeJobs(limit, offset int) ([]*ScrapeJob, error) {
	query := `
		SELECT
			id, url, extract_links, status, retries,
			created_at, updated_at, completed_at,
			error_message, result_request_id, asynq_task_id,
			parent_job_id, depth
		FROM scrape_jobs
		WHERE parent_job_id IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := s.db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list scrape jobs: %w", err)
	}
	defer rows.Close()

	// First, load all parent jobs into memory
	var jobs []*ScrapeJob
	for rows.Next() {
		job, err := s.scanScrapeJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scrape jobs: %w", err)
	}

	// Now load child jobs for each parent (after closing the first result set)
	for _, job := range jobs {
		childJobs, err := s.GetChildJobs(job.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get child jobs: %w", err)
		}
		job.ChildJobs = childJobs
	}

	return jobs, nil
}

// GetChildJobs retrieves all child jobs for a parent job
func (s *Storage) GetChildJobs(parentID string) ([]*ScrapeJob, error) {
	query := `
		SELECT
			id, url, extract_links, status, retries,
			created_at, updated_at, completed_at,
			error_message, result_request_id, asynq_task_id,
			parent_job_id, depth
		FROM scrape_jobs
		WHERE parent_job_id = $1
		ORDER BY created_at ASC
	`

	rows, err := s.db.Query(query, parentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get child jobs: %w", err)
	}
	defer rows.Close()

	var jobs []*ScrapeJob
	for rows.Next() {
		job, err := s.scanScrapeJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating child jobs: %w", err)
	}

	return jobs, nil
}

// scanScrapeJob is a helper to scan a scrape job row
func (s *Storage) scanScrapeJob(row interface {
	Scan(dest ...interface{}) error
}) (*ScrapeJob, error) {
	job := &ScrapeJob{}
	var extractLinks int
	var completedAt sql.NullTime
	var errorMessage sql.NullString
	var resultRequestID sql.NullString
	var asynqTaskID sql.NullString
	var parentJobID sql.NullString

	err := row.Scan(
		&job.ID,
		&job.URL,
		&extractLinks,
		&job.Status,
		&job.Retries,
		&job.CreatedAt,
		&job.UpdatedAt,
		&completedAt,
		&errorMessage,
		&resultRequestID,
		&asynqTaskID,
		&parentJobID,
		&job.Depth,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan scrape job: %w", err)
	}

	job.ExtractLinks = extractLinks != 0
	if completedAt.Valid {
		job.CompletedAt = &completedAt.Time
	}
	if errorMessage.Valid {
		job.ErrorMessage = errorMessage.String
	}
	if resultRequestID.Valid {
		job.ResultRequestID = &resultRequestID.String
	}
	if asynqTaskID.Valid {
		job.AsynqTaskID = asynqTaskID.String
	}
	if parentJobID.Valid {
		job.ParentJobID = &parentJobID.String
	}

	return job, nil
}

// UpdateScrapeJobStatus updates the status of a scrape job
func (s *Storage) UpdateScrapeJobStatus(id, status string, errorMessage string) error {
	now := time.Now()
	var completedAt *time.Time

	// Set completed_at if status is completed or failed
	if status == "completed" || status == "failed" {
		completedAt = &now
	}

	query := `
		UPDATE scrape_jobs
		SET status = $1, updated_at = $2, completed_at = $3, error_message = $4
		WHERE id = $5
	`

	result, err := s.db.Exec(query, status, now, completedAt, errorMessage, id)
	if err != nil {
		return fmt.Errorf("failed to update scrape job status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("scrape job not found")
	}

	return nil
}

// UpdateScrapeJobResult updates the result request ID when a job completes
func (s *Storage) UpdateScrapeJobResult(id string, resultRequestID string) error {
	now := time.Now()
	query := `
		UPDATE scrape_jobs
		SET status = $1, result_request_id = $2, updated_at = $3, completed_at = $4
		WHERE id = $5
	`

	result, err := s.db.Exec(query, "completed", resultRequestID, now, now, id)
	if err != nil {
		return fmt.Errorf("failed to update scrape job result: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("scrape job not found")
	}

	return nil
}

// UpdateScrapeJobTaskID updates the Asynq task ID for a job
func (s *Storage) UpdateScrapeJobTaskID(id string, taskID string) error {
	query := `
		UPDATE scrape_jobs
		SET asynq_task_id = $1, updated_at = $2
		WHERE id = $3
	`

	result, err := s.db.Exec(query, taskID, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to update scrape job task ID: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("scrape job not found")
	}

	return nil
}

// IncrementScrapeJobRetries increments the retry count for a job
func (s *Storage) IncrementScrapeJobRetries(id string) error {
	query := `
		UPDATE scrape_jobs
		SET retries = retries + 1, updated_at = $1
		WHERE id = $2
	`

	result, err := s.db.Exec(query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to increment scrape job retries: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("scrape job not found")
	}

	return nil
}

// DeleteScrapeJob deletes a scrape job
func (s *Storage) DeleteScrapeJob(id string) error {
	query := `DELETE FROM scrape_jobs WHERE id = $1`

	result, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("failed to delete scrape job: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("scrape job not found")
	}

	return nil
}

// CountScrapeJobsByStatus counts jobs by status
func (s *Storage) CountScrapeJobsByStatus(status string) (int, error) {
	query := `SELECT COUNT(*) FROM scrape_jobs WHERE status = $1`

	var count int
	err := s.db.QueryRow(query, status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count scrape jobs: %w", err)
	}

	return count, nil
}

// boolToInt converts boolean to integer (for SQLite)
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
