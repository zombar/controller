package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// SchedulerClient handles communication with the scheduler service
type SchedulerClient struct {
	baseURL    string
	httpClient *http.Client
}

// Task represents a scheduled task
type Task struct {
	ID          int64      `json:"id,omitempty"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Type        string     `json:"type"` // "sql" or "scrape"
	Schedule    string     `json:"schedule"` // Cron expression
	Config      string     `json:"config"` // JSON config
	Enabled     bool       `json:"enabled"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`
}

// NewSchedulerClient creates a new scheduler client
func NewSchedulerClient(baseURL string) *SchedulerClient {
	return &SchedulerClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// ListTasks retrieves all tasks from the scheduler
func (c *SchedulerClient) ListTasks() ([]*Task, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/api/tasks", c.baseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	var tasks []*Task
	if err := json.Unmarshal(body, &tasks); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return tasks, nil
}

// GetTask retrieves a specific task by ID
func (c *SchedulerClient) GetTask(id int64) (*Task, error) {
	resp, err := c.httpClient.Get(fmt.Sprintf("%s/api/tasks/%d", c.baseURL, id))
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	var task Task
	if err := json.Unmarshal(body, &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &task, nil
}

// CreateTask creates a new task in the scheduler
func (c *SchedulerClient) CreateTask(task *Task) (*Task, error) {
	jsonData, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/api/tasks", c.baseURL),
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	var createdTask Task
	if err := json.Unmarshal(body, &createdTask); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &createdTask, nil
}

// UpdateTask updates an existing task
func (c *SchedulerClient) UpdateTask(id int64, task *Task) (*Task, error) {
	jsonData, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/tasks/%d", c.baseURL, id), bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	var updatedTask Task
	if err := json.Unmarshal(body, &updatedTask); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &updatedTask, nil
}

// DeleteTask deletes a task from the scheduler
func (c *SchedulerClient) DeleteTask(id int64) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/tasks/%d", c.baseURL, id), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
