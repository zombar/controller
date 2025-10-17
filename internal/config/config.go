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
	Port                int
	DatabasePath        string
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	config := &Config{
		ScraperBaseURL:      getEnv("SCRAPER_BASE_URL", "http://localhost:8081"),
		TextAnalyzerBaseURL: getEnv("TEXTANALYZER_BASE_URL", "http://localhost:8082"),
		Port:                getEnvAsInt("CONTROLLER_PORT", 8080),
		DatabasePath:        getEnv("DATABASE_PATH", "./controller.db"),
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
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("CONTROLLER_PORT must be between 1 and 65535")
	}
	if c.DatabasePath == "" {
		return fmt.Errorf("DATABASE_PATH is required")
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
