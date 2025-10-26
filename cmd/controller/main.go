package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/config"
	"github.com/zombar/controller/internal/handlers"
	"github.com/zombar/controller/internal/queue"
	"github.com/zombar/controller/internal/storage"
	"github.com/zombar/controller/pkg/logging"
	"github.com/zombar/purpletab/pkg/metrics"
	"github.com/zombar/purpletab/pkg/tracing"
)

// corsMiddleware adds CORS headers to allow web UI access
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "3600")

		// Handle preflight OPTIONS request
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

func main() {
	// Setup structured logging with JSON output
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("controller service initializing", "version", "1.0.0")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize tracing
	tp, err := tracing.InitTracer("docutab-controller")
	if err != nil {
		logger.Warn("failed to initialize tracer, continuing without tracing", "error", err)
	} else {
		defer func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				logger.Error("error shutting down tracer", "error", err)
			}
		}()
		logger.Info("tracing initialized successfully")
	}

	// Initialize storage
	store, err := storage.New(cfg.DatabasePath)
	if err != nil {
		logger.Error("failed to initialize storage", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	// Initialize database metrics
	dbMetrics := metrics.NewDatabaseMetrics("controller")
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			dbMetrics.UpdateDBStats(store.DB())
		}
	}()
	logger.Info("database metrics initialized")

	// Generate mock data if enabled
	if cfg.GenerateMockData {
		logger.Info("mock data generation enabled")
		if err := store.GenerateMockData(); err != nil {
			logger.Warn("failed to generate mock data", "error", err)
		}
	}

	// Initialize clients
	scraperClient := clients.NewScraperClient(cfg.ScraperBaseURL)
	textAnalyzerClient := clients.NewTextAnalyzerClient(cfg.TextAnalyzerBaseURL)
	schedulerClient := clients.NewSchedulerClient(cfg.SchedulerBaseURL)

	// Initialize queue client
	queueClient := queue.NewClient(queue.ClientConfig{
		RedisAddr: cfg.RedisAddr,
	})
	defer queueClient.Close()
	logger.Info("queue client initialized", "redis_addr", cfg.RedisAddr)

	// Initialize queue worker
	worker := queue.NewWorker(
		queue.WorkerConfig{
			RedisAddr:          cfg.RedisAddr,
			Concurrency:        cfg.WorkerConcurrency,
			LinkScoreThreshold: cfg.LinkScoreThreshold,
			MaxLinkDepth:       cfg.MaxLinkDepth,
		},
		store,
		scraperClient,
		textAnalyzerClient,
		queueClient,
	)
	logger.Info("queue worker initialized", "concurrency", cfg.WorkerConcurrency, "max_link_depth", cfg.MaxLinkDepth)

	// Start worker in background
	go func() {
		logger.Info("starting queue worker")
		if err := worker.Start(); err != nil {
			logger.Error("queue worker failed", "error", err)
			os.Exit(1)
		}
	}()

	// Initialize handlers
	handler := handlers.New(store, scraperClient, textAnalyzerClient, schedulerClient, queueClient, cfg.LinkScoreThreshold, cfg.WebInterfaceURL, cfg.ScraperBaseURL)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.Health)
	mux.Handle("/metrics", promhttp.Handler()) // Prometheus metrics endpoint
	mux.HandleFunc("/api/scrape", handler.ScrapeURL)
	mux.HandleFunc("/api/analyze", handler.AnalyzeText)
	mux.HandleFunc("/api/score", handler.ScoreLink)
	mux.HandleFunc("/api/search", handler.SearchTags)
	mux.HandleFunc("/api/images/search", handler.SearchImageTags)
	mux.HandleFunc("/api/requests/filter", handler.FilterRequests)
	mux.HandleFunc("/api/extract-links", handler.ExtractLinks)
	mux.HandleFunc("/api/requests/", func(w http.ResponseWriter, r *http.Request) {
		// Redirect /api/requests/filter to dedicated handler
		if r.URL.Path == "/api/requests/filter" {
			handler.FilterRequests(w, r)
			return
		}

		// Handle /api/requests/timeline-extents
		if r.URL.Path == "/api/requests/timeline-extents" {
			handler.GetTimelineExtents(w, r)
			return
		}

		// Handle /api/requests/{id}/seo-enabled
		if len(r.URL.Path) > len("/api/requests/") && r.URL.Path[len(r.URL.Path)-12:] == "/seo-enabled" {
			if r.Method == http.MethodPut {
				handler.UpdateSEOEnabled(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		// Handle /api/requests/{id}/tombstone
		if len(r.URL.Path) > len("/api/requests/") && r.URL.Path[len(r.URL.Path)-10:] == "/tombstone" {
			if r.Method == http.MethodPut {
				handler.TombstoneRequest(w, r)
			} else if r.Method == http.MethodDelete {
				handler.UntombstoneRequest(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		// Handle /api/requests/{id}/tags
		if len(r.URL.Path) > len("/api/requests/") && r.URL.Path[len(r.URL.Path)-5:] == "/tags" {
			if r.Method == http.MethodPut {
				handler.UpdateRequestTags(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		// Handle /api/requests/{id}
		if r.Method == http.MethodGet {
			handler.GetRequest(w, r)
		} else if r.Method == http.MethodDelete {
			handler.DeleteRequest(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/requests", handler.ListRequests)
	mux.HandleFunc("/api/documents/", handler.GetDocumentImages) // Handles /api/documents/{scraper_uuid}/images
	mux.HandleFunc("/api/images/", func(w http.ResponseWriter, r *http.Request) {
		// Handle /api/images/{id}/tags
		if len(r.URL.Path) > len("/api/images/") && r.URL.Path[len(r.URL.Path)-5:] == "/tags" {
			if r.Method == http.MethodPut {
				handler.UpdateImageTags(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		// Handle /api/images/{id}/tombstone
		if len(r.URL.Path) > len("/api/images/") && r.URL.Path[len(r.URL.Path)-10:] == "/tombstone" {
			if r.Method == http.MethodPut {
				handler.TombstoneImage(w, r)
			} else if r.Method == http.MethodDelete {
				handler.UntombstoneImage(w, r)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		// Handle GET /api/images/{id}
		if r.Method == http.MethodGet {
			handler.GetImage(w, r)
		} else if r.Method == http.MethodDelete {
			handler.DeleteImage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Async scrape request routes
	mux.HandleFunc("/api/scrape-requests", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			handler.CreateScrapeRequest(w, r)
		} else if r.Method == http.MethodGet {
			handler.ListScrapeRequests(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Async text analysis request route
	mux.HandleFunc("/api/analyze-requests", handler.CreateTextAnalysisRequest)
	mux.HandleFunc("/api/scrape-requests/", func(w http.ResponseWriter, r *http.Request) {
		// Handle /api/scrape-requests/{id}/retry
		if len(r.URL.Path) > len("/api/scrape-requests/") && r.URL.Path[len(r.URL.Path)-6:] == "/retry" {
			handler.RetryScrapeRequest(w, r)
			return
		}

		// Handle /api/scrape-requests/{id}
		if r.Method == http.MethodGet {
			handler.GetScrapeRequest(w, r)
		} else if r.Method == http.MethodDelete {
			handler.DeleteScrapeRequest(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Scheduler routes
	mux.HandleFunc("/api/scheduler/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.ListSchedulerTasks(w, r)
		} else if r.Method == http.MethodPost {
			handler.CreateSchedulerTask(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/scheduler/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.GetSchedulerTask(w, r)
		} else if r.Method == http.MethodPut {
			handler.UpdateSchedulerTask(w, r)
		} else if r.Method == http.MethodDelete {
			handler.DeleteSchedulerTask(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// SEO routes (public-facing)
	mux.HandleFunc("/content/", handler.ServeContent)            // Serve SEO-optimized content pages
	mux.HandleFunc("/sitemap.xml", handler.ServeSitemap)         // XML sitemap for search engines
	mux.HandleFunc("/images-sitemap.xml", handler.ServeImageSitemap) // XML image sitemap
	mux.HandleFunc("/robots.txt", handler.ServeRobotsTxt)        // Robots.txt for crawlers

	// Setup server with middleware chain: CORS -> HTTP logging -> metrics -> tracing -> handlers
	addr := fmt.Sprintf(":%d", cfg.Port)
	var httpHandler http.Handler = mux

	// Wrap with tracing middleware if initialized
	if tp != nil {
		httpHandler = tracing.HTTPMiddleware("docutab-controller")(httpHandler)
	}

	// Add HTTP metrics middleware
	httpHandler = metrics.HTTPMiddleware("controller")(httpHandler)

	// Add HTTP request logging
	httpHandler = logging.HTTPLoggingMiddleware(logger)(httpHandler)

	// Apply CORS middleware
	httpHandler = corsMiddleware(httpHandler)

	server := &http.Server{
		Addr:    addr,
		Handler: httpHandler,
	}

	// Setup graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		logger.Info("controller service starting",
			"port", cfg.Port,
			"scraper_url", cfg.ScraperBaseURL,
			"textanalyzer_url", cfg.TextAnalyzerBaseURL,
			"scheduler_url", cfg.SchedulerBaseURL,
			"database", cfg.DatabasePath,
			"link_score_threshold", cfg.LinkScoreThreshold,
		)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	logger.Info("shutting down controller service")

	// Shutdown worker
	worker.Shutdown()
	logger.Info("queue worker stopped")

	// Close storage
	if err := store.Close(); err != nil {
		logger.Error("error closing storage", "error", err)
	}

	logger.Info("controller service stopped")
}
