package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/config"
	"github.com/zombar/controller/internal/handlers"
	"github.com/zombar/controller/internal/scrapemanager"
	"github.com/zombar/controller/internal/storage"
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
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize storage
	store, err := storage.New(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}
	defer store.Close()

	// Initialize clients
	scraperClient := clients.NewScraperClient(cfg.ScraperBaseURL)
	textAnalyzerClient := clients.NewTextAnalyzerClient(cfg.TextAnalyzerBaseURL)

	// Initialize scrape manager with 15-minute TTL
	scrapeManager := scrapemanager.New(15 * time.Minute)

	// Start cleanup loop (runs every 1 minute)
	cleanupStop := scrapeManager.StartCleanupLoop(1 * time.Minute)
	defer close(cleanupStop)

	// Initialize handlers
	handler := handlers.New(store, scraperClient, textAnalyzerClient, scrapeManager, cfg.LinkScoreThreshold)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/api/scrape", handler.ScrapeURL)
	mux.HandleFunc("/api/analyze", handler.AnalyzeText)
	mux.HandleFunc("/api/score", handler.ScoreLink)
	mux.HandleFunc("/api/search", handler.SearchTags)
	mux.HandleFunc("/api/images/search", handler.SearchImageTags)
	mux.HandleFunc("/api/extract-links", handler.ExtractLinks)
	mux.HandleFunc("/api/scrape/batch", handler.BatchScrape)
	mux.HandleFunc("/api/requests/", handler.GetRequest)
	mux.HandleFunc("/api/requests", handler.ListRequests)

	// Scrape request management routes
	scrapeRequestHandler := func(w http.ResponseWriter, r *http.Request) {
		// Route to appropriate handler based on path
		path := r.URL.Path
		if path == "/api/scrape/request/" || path == "/api/scrape/request" {
			if r.Method == http.MethodPost {
				handler.CreateScrapeRequest(w, r)
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusMethodNotAllowed)
				w.Write([]byte(`{"error":"Method not allowed"}`))
			}
		} else if r.Method == http.MethodGet {
			handler.GetScrapeRequest(w, r)
		} else if r.Method == http.MethodDelete {
			handler.DeleteScrapeRequest(w, r)
		} else if len(path) > len("/api/scrape/request/") && path[len(path)-6:] == "/retry" {
			handler.RetryScrapeRequest(w, r)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Not found"}`))
		}
	}
	mux.HandleFunc("/api/scrape/request/", scrapeRequestHandler)
	mux.HandleFunc("/api/scrape/request", scrapeRequestHandler)
	mux.HandleFunc("/api/scrape/requests", handler.ListScrapeRequests)

	// Setup server with CORS middleware
	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: corsMiddleware(mux),
	}

	// Setup graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		log.Printf("Controller service starting on port %d", cfg.Port)
		log.Printf("Scraper URL: %s", cfg.ScraperBaseURL)
		log.Printf("TextAnalyzer URL: %s", cfg.TextAnalyzerBaseURL)
		log.Printf("Database: %s", cfg.DatabasePath)
		log.Printf("Link Score Threshold: %.2f", cfg.LinkScoreThreshold)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	log.Println("Shutting down controller service...")

	// Close storage
	if err := store.Close(); err != nil {
		log.Printf("Error closing storage: %v", err)
	}

	log.Println("Controller service stopped")
}
