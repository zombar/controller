package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/zombar/controller/internal/clients"
	"github.com/zombar/controller/internal/config"
	"github.com/zombar/controller/internal/handlers"
	"github.com/zombar/controller/internal/storage"
)

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

	// Initialize handlers
	handler := handlers.New(store, scraperClient, textAnalyzerClient)

	// Setup routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handler.Health)
	mux.HandleFunc("/api/scrape", handler.ScrapeURL)
	mux.HandleFunc("/api/analyze", handler.AnalyzeText)
	mux.HandleFunc("/api/search", handler.SearchTags)
	mux.HandleFunc("/api/images/search", handler.SearchImageTags)
	mux.HandleFunc("/api/requests/", handler.GetRequest)
	mux.HandleFunc("/api/requests", handler.ListRequests)

	// Setup server
	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
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
