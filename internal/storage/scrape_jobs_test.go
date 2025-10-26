package storage

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// setupTestDB creates a temporary test database and returns a cleanup function
func setupTestDB(t *testing.T) (*Storage, func()) {
	t.Helper()
	dbPath := fmt.Sprintf("test_scrape_jobs_%d.db", time.Now().UnixNano())

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(dbPath)
	}

	return store, cleanup
}

func TestScrapeJobCRUD(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a parent job
	parentID := "parent-job-123"
	parentJob := &ScrapeJob{
		ID:           parentID,
		URL:          "https://example.com/parent",
		ExtractLinks: true,
		Status:       "queued",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Depth:        0,
	}

	// Save parent job
	err := store.SaveScrapeJob(parentJob)
	if err != nil {
		t.Fatalf("Failed to save parent job: %v", err)
	}

	// Retrieve parent job
	retrieved, err := store.GetScrapeJob(parentID)
	if err != nil {
		t.Fatalf("Failed to get parent job: %v", err)
	}

	if retrieved == nil {
		t.Fatal("Retrieved job is nil")
	}

	if retrieved.ID != parentID {
		t.Errorf("Expected ID %s, got %s", parentID, retrieved.ID)
	}

	if retrieved.URL != "https://example.com/parent" {
		t.Errorf("Expected URL 'https://example.com/parent', got '%s'", retrieved.URL)
	}

	if retrieved.ExtractLinks != true {
		t.Error("Expected ExtractLinks to be true")
	}

	if retrieved.Depth != 0 {
		t.Errorf("Expected depth 0, got %d", retrieved.Depth)
	}

	if retrieved.ParentJobID != nil {
		t.Errorf("Expected nil parent job ID, got %v", *retrieved.ParentJobID)
	}
}

func TestScrapeJobParentChild(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create parent job
	parentID := "parent-job-456"
	parentJob := &ScrapeJob{
		ID:           parentID,
		URL:          "https://example.com/parent",
		ExtractLinks: true,
		Status:       "completed",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Depth:        0,
	}

	err := store.SaveScrapeJob(parentJob)
	if err != nil {
		t.Fatalf("Failed to save parent job: %v", err)
	}

	// Create child jobs
	childID1 := "child-job-001"
	childJob1 := &ScrapeJob{
		ID:           childID1,
		URL:          "https://example.com/child1",
		ExtractLinks: false,
		Status:       "queued",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ParentJobID:  &parentID,
		Depth:        1,
	}

	childID2 := "child-job-002"
	childJob2 := &ScrapeJob{
		ID:           childID2,
		URL:          "https://example.com/child2",
		ExtractLinks: false,
		Status:       "processing",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ParentJobID:  &parentID,
		Depth:        1,
	}

	err = store.SaveScrapeJob(childJob1)
	if err != nil {
		t.Fatalf("Failed to save child job 1: %v", err)
	}

	err = store.SaveScrapeJob(childJob2)
	if err != nil {
		t.Fatalf("Failed to save child job 2: %v", err)
	}

	// Retrieve child jobs
	children, err := store.GetChildJobs(parentID)
	if err != nil {
		t.Fatalf("Failed to get child jobs: %v", err)
	}

	if len(children) != 2 {
		t.Fatalf("Expected 2 child jobs, got %d", len(children))
	}

	// Verify child job properties
	for _, child := range children {
		if child.ParentJobID == nil {
			t.Error("Child job has nil parent ID")
			continue
		}

		if *child.ParentJobID != parentID {
			t.Errorf("Expected parent ID %s, got %s", parentID, *child.ParentJobID)
		}

		if child.Depth != 1 {
			t.Errorf("Expected depth 1, got %d", child.Depth)
		}
	}
}

func TestListScrapeJobsOnlyParents(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create parent jobs
	parent1ID := "parent-1"
	parent1 := &ScrapeJob{
		ID:           parent1ID,
		URL:          "https://example.com/parent1",
		ExtractLinks: true,
		Status:       "completed",
		Retries:      0,
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now(),
		Depth:        0,
	}

	parent2ID := "parent-2"
	parent2 := &ScrapeJob{
		ID:           parent2ID,
		URL:          "https://example.com/parent2",
		ExtractLinks: false,
		Status:       "queued",
		Retries:      0,
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now(),
		Depth:        0,
	}

	// Create child jobs
	child1ID := "child-1"
	child1 := &ScrapeJob{
		ID:           child1ID,
		URL:          "https://example.com/child1",
		ExtractLinks: false,
		Status:       "queued",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ParentJobID:  &parent1ID,
		Depth:        1,
	}

	child2ID := "child-2"
	child2 := &ScrapeJob{
		ID:           child2ID,
		URL:          "https://example.com/child2",
		ExtractLinks: false,
		Status:       "processing",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ParentJobID:  &parent1ID,
		Depth:        1,
	}

	// Save all jobs
	for _, job := range []*ScrapeJob{parent1, parent2, child1, child2} {
		if err := store.SaveScrapeJob(job); err != nil {
			t.Fatalf("Failed to save job %s: %v", job.ID, err)
		}
	}

	// List jobs (should only return parents with their children)
	jobs, err := store.ListScrapeJobs(10, 0)
	if err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}

	// Should return 2 parent jobs (not child jobs)
	if len(jobs) != 2 {
		t.Fatalf("Expected 2 parent jobs, got %d", len(jobs))
	}

	// Find parent1 in results
	var parent1Result *ScrapeJob
	for _, job := range jobs {
		if job.ID == parent1ID {
			parent1Result = job
			break
		}
	}

	if parent1Result == nil {
		t.Fatal("Parent1 not found in results")
	}

	// Verify parent1 has child jobs loaded
	if len(parent1Result.ChildJobs) != 2 {
		t.Errorf("Expected parent1 to have 2 children, got %d", len(parent1Result.ChildJobs))
	}

	// Find parent2 in results
	var parent2Result *ScrapeJob
	for _, job := range jobs {
		if job.ID == parent2ID {
			parent2Result = job
			break
		}
	}

	if parent2Result == nil {
		t.Fatal("Parent2 not found in results")
	}

	// Verify parent2 has no children
	if len(parent2Result.ChildJobs) != 0 {
		t.Errorf("Expected parent2 to have 0 children, got %d", len(parent2Result.ChildJobs))
	}

	// Verify jobs are sorted by created_at descending
	if jobs[0].CreatedAt.Before(jobs[1].CreatedAt) {
		t.Error("Jobs are not sorted by created_at descending")
	}
}

func TestUpdateScrapeJobStatus(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	jobID := "test-job-789"
	job := &ScrapeJob{
		ID:        jobID,
		URL:       "https://example.com/test",
		Status:    "queued",
		Retries:   0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Depth:     0,
	}

	err := store.SaveScrapeJob(job)
	if err != nil {
		t.Fatalf("Failed to save job: %v", err)
	}

	// Update to processing
	err = store.UpdateScrapeJobStatus(jobID, "processing", "")
	if err != nil {
		t.Fatalf("Failed to update status: %v", err)
	}

	retrieved, err := store.GetScrapeJob(jobID)
	if err != nil {
		t.Fatalf("Failed to retrieve job: %v", err)
	}

	if retrieved.Status != "processing" {
		t.Errorf("Expected status 'processing', got '%s'", retrieved.Status)
	}

	// Update to failed with error message
	errorMsg := "Connection timeout"
	err = store.UpdateScrapeJobStatus(jobID, "failed", errorMsg)
	if err != nil {
		t.Fatalf("Failed to update status to failed: %v", err)
	}

	retrieved, err = store.GetScrapeJob(jobID)
	if err != nil {
		t.Fatalf("Failed to retrieve job after failure: %v", err)
	}

	if retrieved.Status != "failed" {
		t.Errorf("Expected status 'failed', got '%s'", retrieved.Status)
	}

	if retrieved.ErrorMessage != errorMsg {
		t.Errorf("Expected error message '%s', got '%s'", errorMsg, retrieved.ErrorMessage)
	}

	if retrieved.CompletedAt == nil {
		t.Error("Expected completed_at to be set for failed job")
	}
}

func TestIncrementScrapeJobRetries(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	jobID := "retry-job-001"
	job := &ScrapeJob{
		ID:        jobID,
		URL:       "https://example.com/retry",
		Status:    "queued",
		Retries:   0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Depth:     0,
	}

	err := store.SaveScrapeJob(job)
	if err != nil {
		t.Fatalf("Failed to save job: %v", err)
	}

	// Increment retries 3 times
	for i := 1; i <= 3; i++ {
		err = store.IncrementScrapeJobRetries(jobID)
		if err != nil {
			t.Fatalf("Failed to increment retries (attempt %d): %v", i, err)
		}

		retrieved, err := store.GetScrapeJob(jobID)
		if err != nil {
			t.Fatalf("Failed to retrieve job: %v", err)
		}

		if retrieved.Retries != i {
			t.Errorf("Expected retries to be %d, got %d", i, retrieved.Retries)
		}
	}
}

func TestDeleteScrapeJob(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	jobID := "delete-job-001"
	job := &ScrapeJob{
		ID:        jobID,
		URL:       "https://example.com/delete",
		Status:    "completed",
		Retries:   0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Depth:     0,
	}

	err := store.SaveScrapeJob(job)
	if err != nil {
		t.Fatalf("Failed to save job: %v", err)
	}

	// Verify job exists
	retrieved, err := store.GetScrapeJob(jobID)
	if err != nil {
		t.Fatalf("Failed to retrieve job: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Job should exist before deletion")
	}

	// Delete job
	err = store.DeleteScrapeJob(jobID)
	if err != nil {
		t.Fatalf("Failed to delete job: %v", err)
	}

	// Verify job is deleted
	retrieved, err = store.GetScrapeJob(jobID)
	if err != nil {
		t.Fatalf("Error retrieving deleted job: %v", err)
	}
	if retrieved != nil {
		t.Error("Job should be nil after deletion")
	}
}

func TestMultiLevelHierarchy(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a 3-level hierarchy
	// Root -> Child1 -> GrandChild1
	//      -> Child2

	rootID := "root-job"
	rootJob := &ScrapeJob{
		ID:           rootID,
		URL:          "https://example.com/root",
		ExtractLinks: true,
		Status:       "completed",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Depth:        0,
	}

	child1ID := "child1-job"
	child1Job := &ScrapeJob{
		ID:           child1ID,
		URL:          "https://example.com/child1",
		ExtractLinks: true,
		Status:       "completed",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ParentJobID:  &rootID,
		Depth:        1,
	}

	child2ID := "child2-job"
	child2Job := &ScrapeJob{
		ID:           child2ID,
		URL:          "https://example.com/child2",
		ExtractLinks: false,
		Status:       "queued",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ParentJobID:  &rootID,
		Depth:        1,
	}

	grandChild1ID := "grandchild1-job"
	grandChild1Job := &ScrapeJob{
		ID:           grandChild1ID,
		URL:          "https://example.com/grandchild1",
		ExtractLinks: false,
		Status:       "processing",
		Retries:      0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ParentJobID:  &child1ID,
		Depth:        2,
	}

	// Save all jobs
	for _, job := range []*ScrapeJob{rootJob, child1Job, child2Job, grandChild1Job} {
		if err := store.SaveScrapeJob(job); err != nil {
			t.Fatalf("Failed to save job %s: %v", job.ID, err)
		}
	}

	// Get root's children
	rootChildren, err := store.GetChildJobs(rootID)
	if err != nil {
		t.Fatalf("Failed to get root children: %v", err)
	}

	if len(rootChildren) != 2 {
		t.Errorf("Expected 2 children for root, got %d", len(rootChildren))
	}

	// Get child1's children
	child1Children, err := store.GetChildJobs(child1ID)
	if err != nil {
		t.Fatalf("Failed to get child1 children: %v", err)
	}

	if len(child1Children) != 1 {
		t.Errorf("Expected 1 child for child1, got %d", len(child1Children))
	}

	if child1Children[0].Depth != 2 {
		t.Errorf("Expected grandchild depth 2, got %d", child1Children[0].Depth)
	}

	// Get child2's children (should be empty)
	child2Children, err := store.GetChildJobs(child2ID)
	if err != nil {
		t.Fatalf("Failed to get child2 children: %v", err)
	}

	if len(child2Children) != 0 {
		t.Errorf("Expected 0 children for child2, got %d", len(child2Children))
	}
}
