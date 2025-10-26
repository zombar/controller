package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
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
			Transport: otelhttp.NewTransport(http.DefaultTransport), // Inject trace context headers
		},
	}
}

// ListTasks retrieves all tasks from the scheduler
func (c *SchedulerClient) ListTasks(ctx context.Context) ([]*Task, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scheduler.ListTasks")
	defer span.End()

	span.SetAttributes(attribute.String("http.method", "GET"))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/tasks", c.baseURL),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	var tasks []*Task
	if err := json.Unmarshal(body, &tasks); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetAttributes(attribute.Int("scheduler.task_count", len(tasks)))
	span.SetStatus(codes.Ok, "success")
	return tasks, nil
}

// GetTask retrieves a specific task by ID
func (c *SchedulerClient) GetTask(ctx context.Context, id int64) (*Task, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scheduler.GetTask")
	defer span.End()

	span.SetAttributes(
		attribute.Int64("scheduler.task_id", id),
		attribute.String("http.method", "GET"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/api/tasks/%d", c.baseURL, id),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	var task Task
	if err := json.Unmarshal(body, &task); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetStatus(codes.Ok, "success")
	return &task, nil
}

// CreateTask creates a new task in the scheduler
func (c *SchedulerClient) CreateTask(ctx context.Context, task *Task) (*Task, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scheduler.CreateTask")
	defer span.End()

	span.SetAttributes(
		attribute.String("scheduler.task_name", task.Name),
		attribute.String("scheduler.task_type", task.Type),
		attribute.String("http.method", "POST"),
	)

	jsonData, err := json.Marshal(task)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/api/tasks", c.baseURL),
		bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	var createdTask Task
	if err := json.Unmarshal(body, &createdTask); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetStatus(codes.Ok, "success")
	return &createdTask, nil
}

// UpdateTask updates an existing task
func (c *SchedulerClient) UpdateTask(ctx context.Context, id int64, task *Task) (*Task, error) {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scheduler.UpdateTask")
	defer span.End()

	span.SetAttributes(
		attribute.Int64("scheduler.task_id", id),
		attribute.String("scheduler.task_name", task.Name),
		attribute.String("scheduler.task_type", task.Type),
		attribute.String("http.method", "PUT"),
	)

	jsonData, err := json.Marshal(task)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to marshal request")
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/api/tasks/%d", c.baseURL, id),
		bytes.NewBuffer(jsonData))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return nil, fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to read response")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return nil, fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	var updatedTask Task
	if err := json.Unmarshal(body, &updatedTask); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to unmarshal response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	span.SetStatus(codes.Ok, "success")
	return &updatedTask, nil
}

// DeleteTask deletes a task from the scheduler
func (c *SchedulerClient) DeleteTask(ctx context.Context, id int64) error {
	tracer := otel.Tracer("controller")
	ctx, span := tracer.Start(ctx, "scheduler.DeleteTask")
	defer span.End()

	span.SetAttributes(
		attribute.Int64("scheduler.task_id", id),
		attribute.String("http.method", "DELETE"),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/api/tasks/%d", c.baseURL, id),
		nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create request")
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to send request")
		return fmt.Errorf("failed to send request to scheduler: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		span.SetStatus(codes.Error, fmt.Sprintf("status %d", resp.StatusCode))
		return fmt.Errorf("scheduler service returned status %d: %s", resp.StatusCode, string(body))
	}

	span.SetStatus(codes.Ok, "success")
	return nil
}
