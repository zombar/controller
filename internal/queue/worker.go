package queue

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/storage"
	"github.com/zombar/purpletab/pkg/metrics"
)

// URLCache defines the interface for URL caching
type URLCache interface {
	Get(ctx context.Context, url string) (string, error)
	Set(ctx context.Context, url, scraperUUID string) error
	Delete(ctx context.Context, url string) error
}

// slogAdapter wraps slog.Logger to implement asynq.Logger interface for structured logging
type slogAdapter struct {
	logger *slog.Logger
}

// Debug implements asynq.Logger
func (l *slogAdapter) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

// Info implements asynq.Logger
func (l *slogAdapter) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

// Warn implements asynq.Logger
func (l *slogAdapter) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

// Error implements asynq.Logger
func (l *slogAdapter) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

// Fatal implements asynq.Logger
func (l *slogAdapter) Fatal(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
	log.Fatal(args...)
}

// Worker wraps the Asynq server for processing tasks
type Worker struct {
	server                  *asynq.Server
	mux                     *asynq.ServeMux
	storage                 *storage.Storage
	scraperClient           *clients.ScraperClient
	textAnalyzerClient      *clients.TextAnalyzerClient
	linkScoreThreshold      float64
	concurrency             int
	logger                  *slog.Logger
	queueClient             *Client
	maxLinkDepth            int
	urlCache                URLCache
	tombstonePeriodLowScore int // Days until deletion for low-score URLs
	maxAnalysisWaitMinutes  int // Maximum minutes to wait for analysis retrieval before giving up
	businessMetrics         *metrics.BusinessMetrics
}

// WorkerConfig contains configuration for the queue worker
type WorkerConfig struct {
	RedisAddr               string
	Concurrency             int
	LinkScoreThreshold      float64
	MaxLinkDepth            int
	TombstonePeriodLowScore int // Days until deletion for low-score URLs
	MaxAnalysisWaitMinutes  int // Maximum minutes to wait for analysis retrieval (0 = unlimited, default 60)
}

// NewWorker creates a new queue worker
func NewWorker(
	cfg WorkerConfig,
	storage *storage.Storage,
	scraperClient *clients.ScraperClient,
	textAnalyzerClient *clients.TextAnalyzerClient,
	queueClient *Client,
	urlCache URLCache,
	businessMetrics *metrics.BusinessMetrics,
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
			"scrape":             6, // URL scraping tasks (highest priority)
			"analysis-retrieval": 4, // Text analysis result retrieval (medium priority)
			"link-extraction":    3, // Link extraction and processing (lower priority)
		},

		// StrictPriority: false means queues are processed proportionally
		// true would mean scrape queue must be empty before processing link-extraction
		StrictPriority: false,

		// Retry configuration
		RetryDelayFunc: func(n int, err error, task *asynq.Task) time.Duration {
			// Exponential backoff up to 24 hours: 1m, 5m, 15m, 30m, 1h, 2h, 4h, 8h
			delays := []time.Duration{
				1 * time.Minute,
				5 * time.Minute,
				15 * time.Minute,
				30 * time.Minute,
				1 * time.Hour,
				2 * time.Hour,
				4 * time.Hour,
				8 * time.Hour,
			}
			if n < len(delays) {
				return delays[n]
			}
			return delays[len(delays)-1] // Cap at 8 hours
		},

		// Graceful shutdown timeout
		ShutdownTimeout: 30 * time.Second,

		// Use structured logging
		Logger: &slogAdapter{
			logger: slog.Default(),
		},

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

	// Set default for max analysis wait time if not specified
	maxAnalysisWait := cfg.MaxAnalysisWaitMinutes
	if maxAnalysisWait == 0 {
		maxAnalysisWait = 60 // Default: 60 minutes for production
	}

	w := &Worker{
		server:                  server,
		mux:                     mux,
		storage:                 storage,
		scraperClient:           scraperClient,
		textAnalyzerClient:      textAnalyzerClient,
		linkScoreThreshold:      cfg.LinkScoreThreshold,
		concurrency:             cfg.Concurrency,
		logger:                  slog.Default(),
		queueClient:             queueClient,
		maxLinkDepth:            cfg.MaxLinkDepth,
		urlCache:                urlCache,
		tombstonePeriodLowScore: cfg.TombstonePeriodLowScore,
		maxAnalysisWaitMinutes:  maxAnalysisWait,
		businessMetrics:         businessMetrics,
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
	w.mux.HandleFunc(TypeRetrieveAnalysis, w.handleRetrieveAnalysis)
}

// Start starts the worker to begin processing tasks
func (w *Worker) Start() error {
	w.logger.Info("starting asynq worker",
		"concurrency", w.concurrency,
		"queues", map[string]int{"scrape": 6, "analysis-retrieval": 4, "link-extraction": 3},
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
