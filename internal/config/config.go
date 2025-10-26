package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all configuration for the controller service
type Config struct {
	ScraperBaseURL      string
	TextAnalyzerBaseURL string
	SchedulerBaseURL    string
	Port                int
	DatabasePath        string
	LinkScoreThreshold  float64 // Minimum score for link recommendation (0.0-1.0)
	GenerateMockData    bool    // Generate 6 months of mock historical data on startup (~600 documents)
	WebInterfaceURL     string  // URL for the web interface (for footer links on static pages)
	RedisAddr           string  // Redis address for queue backend
	WorkerConcurrency   int     // Number of concurrent workers for processing tasks
	MaxLinkDepth        int     // Maximum depth for link extraction (0 = no links, 1 = extract only from root URL)
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	config := &Config{
		ScraperBaseURL:      getEnv("SCRAPER_BASE_URL", "http://localhost:8081"),
		TextAnalyzerBaseURL: getEnv("TEXTANALYZER_BASE_URL", "http://localhost:8082"),
		SchedulerBaseURL:    getEnv("SCHEDULER_BASE_URL", "http://localhost:8083"),
		Port:                getEnvAsInt("CONTROLLER_PORT", 8080),
		DatabasePath:        getEnv("DATABASE_PATH", "./controller.db"),
		LinkScoreThreshold:  getEnvAsFloat("LINK_SCORE_THRESHOLD", 0.5),
		GenerateMockData:    getEnvAsBool("GENERATE_MOCK_DATA", false),
		WebInterfaceURL:     getEnv("WEB_INTERFACE_URL", "http://localhost:5173"),
		RedisAddr:           getEnv("REDIS_ADDR", "localhost:6379"),
		WorkerConcurrency:   getEnvAsInt("WORKER_CONCURRENCY", 10),
		MaxLinkDepth:        getEnvAsInt("MAX_LINK_DEPTH", 1),
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.ScraperBaseURL == "" {
		return fmt.Errorf("SCRAPER_BASE_URL is required")
	}
	if c.TextAnalyzerBaseURL == "" {
		return fmt.Errorf("TEXTANALYZER_BASE_URL is required")
	}
	if c.SchedulerBaseURL == "" {
		return fmt.Errorf("SCHEDULER_BASE_URL is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("CONTROLLER_PORT must be between 1 and 65535")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("DATABASE_PATH is required")
	}
	if c.LinkScoreThreshold < 0.0 || c.LinkScoreThreshold > 1.0 {
		return fmt.Errorf("LINK_SCORE_THRESHOLD must be between 0.0 and 1.0")
	}
	if c.RedisAddr == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}
	if c.WorkerConcurrency <= 0 {
		return fmt.Errorf("WORKER_CONCURRENCY must be greater than 0")
	}
	if c.MaxLinkDepth < 0 {
		return fmt.Errorf("MAX_LINK_DEPTH must be >= 0")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return defaultValue
	}
	return value
}

func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}
	return value
}
