package queue

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/storage"
)

// URLCache defines the interface for URL caching
type URLCache interface {
	Get(ctx context.Context, url string) (string, error)
	Set(ctx context.Context, url, scraperUUID string) error
	Delete(ctx context.Context, url string) error
}

// Worker wraps the Asynq server for processing tasks
type Worker struct {
	server              *asynq.Server
	mux                 *asynq.ServeMux
	storage             *storage.Storage
	scraperClient       *clients.ScraperClient
	textAnalyzerClient  *clients.TextAnalyzerClient
	linkScoreThreshold  float64
	concurrency         int
	logger              *slog.Logger
	queueClient         *Client
	maxLinkDepth        int
	urlCache            URLCache
}

// WorkerConfig contains configuration for the queue worker
type WorkerConfig struct {
	RedisAddr          string
	Concurrency        int
	LinkScoreThreshold float64
	MaxLinkDepth       int
}

// NewWorker creates a new queue worker
func NewWorker(
	cfg WorkerConfig,
	storage *storage.Storage,
	scraperClient *clients.ScraperClient,
	textAnalyzerClient *clients.TextAnalyzerClient,
	queueClient *Client,
	urlCache URLCache,
) *Worker {
	redisOpt := asynq.RedisClientOpt{
		Addr: cfg.RedisAddr,
	}

	serverCfg := asynq.Config{
		// Concurrency determines how many tasks can be processed simultaneously
		Concurrency: cfg.Concurrency,

		// Queue priority: higher value = higher priority
		// Named queues for clarity: scrape tasks get highest priority, link extraction is lower
		Queues: map[string]int{
			"scrape":         6, // URL scraping tasks (highest priority)
			"link-extraction": 3, // Link extraction and processing (lower priority)
		},

		// StrictPriority: false means queues are processed proportionally
		// true would mean scrape queue must be empty before processing link-extraction
		StrictPriority: false,

		// Retry configuration
		RetryDelayFunc: func(n int, err error, task *asynq.Task) time.Duration {
			// Exponential backoff: 1min, 5min, 15min
			delays := []time.Duration{
				1 * time.Minute,
				5 * time.Minute,
				15 * time.Minute,
			}
			if n < len(delays) {
				return delays[n]
			}
			return delays[len(delays)-1]
		},

		// Graceful shutdown timeout
		ShutdownTimeout: 30 * time.Second,

		// Error handler for logging
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			slog.Error("task processing error",
				"task_type", task.Type(),
				"error", err,
			)
		}),
	}

	server := asynq.NewServer(redisOpt, serverCfg)
	mux := asynq.NewServeMux()

	w := &Worker{
		server:              server,
		mux:                 mux,
		storage:             storage,
		scraperClient:       scraperClient,
		textAnalyzerClient:  textAnalyzerClient,
		linkScoreThreshold:  cfg.LinkScoreThreshold,
		concurrency:         cfg.Concurrency,
		logger:              slog.Default(),
		queueClient:         queueClient,
		maxLinkDepth:        cfg.MaxLinkDepth,
		urlCache:            urlCache,
	}

	// Register task handlers
	w.registerHandlers()

	return w
}

// registerHandlers registers all task handlers with the worker
func (w *Worker) registerHandlers() {
	// Register the scrape URL handler
	w.mux.HandleFunc(TypeScrapeURL, w.handleScrapeTask)
	w.mux.HandleFunc(TypeExtractLinks, w.handleExtractLinksTask)

	// Add more handlers here as needed
	// w.mux.HandleFunc(TypeAnalyzeText, w.handleAnalyzeTask)
}

// Start starts the worker to begin processing tasks
func (w *Worker) Start() error {
	w.logger.Info("starting asynq worker",
		"concurrency", w.concurrency,
		"queues", map[string]int{"scrape": 6, "link-extraction": 3},
	)

	// Run is blocking - starts processing tasks
	if err := w.server.Run(w.mux); err != nil {
		return fmt.Errorf("asynq server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the worker
func (w *Worker) Shutdown() {
	w.logger.Info("shutting down asynq worker")
	w.server.Shutdown()
}

// Server returns the underlying Asynq server (for testing)
func (w *Worker) Server() *asynq.Server {
	return w.server
}
