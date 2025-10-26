package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	// Set test environment variables
	os.Setenv("SCRAPER_BASE_URL", "http://test-scraper:8081")
	os.Setenv("TEXTANALYZER_BASE_URL", "http://test-analyzer:8082")
	os.Setenv("CONTROLLER_PORT", "9090")
	os.Setenv("DATABASE_PATH", "/tmp/test.db")
	defer func() {
		os.Unsetenv("SCRAPER_BASE_URL")
		os.Unsetenv("TEXTANALYZER_BASE_URL")
		os.Unsetenv("CONTROLLER_PORT")
		os.Unsetenv("DATABASE_PATH")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.ScraperBaseURL != "http://test-scraper:8081" {
		t.Errorf("Expected ScraperBaseURL 'http://test-scraper:8081', got '%s'", cfg.ScraperBaseURL)
	}
	if cfg.TextAnalyzerBaseURL != "http://test-analyzer:8082" {
		t.Errorf("Expected TextAnalyzerBaseURL 'http://test-analyzer:8082', got '%s'", cfg.TextAnalyzerBaseURL)
	}
	if cfg.Port != 9090 {
		t.Errorf("Expected Port 9090, got %d", cfg.Port)
	}
	if cfg.DatabasePath != "/tmp/test.db" {
		t.Errorf("Expected DatabasePath '/tmp/test.db', got '%s'", cfg.DatabasePath)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Clear environment variables to test defaults
	os.Unsetenv("SCRAPER_BASE_URL")
	os.Unsetenv("TEXTANALYZER_BASE_URL")
	os.Unsetenv("CONTROLLER_PORT")
	os.Unsetenv("DATABASE_PATH")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config with defaults: %v", err)
	}

	if cfg.ScraperBaseURL != "http://localhost:8081" {
		t.Errorf("Expected default ScraperBaseURL, got '%s'", cfg.ScraperBaseURL)
	}
	if cfg.TextAnalyzerBaseURL != "http://localhost:8082" {
		t.Errorf("Expected default TextAnalyzerBaseURL, got '%s'", cfg.TextAnalyzerBaseURL)
	}
	if cfg.Port != 8080 {
		t.Errorf("Expected default Port 8080, got %d", cfg.Port)
	}
	if cfg.DatabasePath != "./controller.db" {
		t.Errorf("Expected default DatabasePath, got '%s'", cfg.DatabasePath)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				SchedulerBaseURL:    "http://localhost:8083",
				Port:                8080,
				DatabasePath:        "./test.db",
				RedisAddr:           "localhost:6379",
				WorkerConcurrency:   10,
				MaxLinkDepth:        1,
			},
			expectError: false,
		},
		{
			name: "missing scraper URL",
			config: &Config{
				ScraperBaseURL:      "",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                8080,
				DatabasePath:        "./test.db",
			},
			expectError: true,
		},
		{
			name: "missing text analyzer URL",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "",
				Port:                8080,
				DatabasePath:        "./test.db",
			},
			expectError: true,
		},
		{
			name: "invalid port (too low)",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                0,
				DatabasePath:        "./test.db",
			},
			expectError: true,
		},
		{
			name: "invalid port (too high)",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                70000,
				DatabasePath:        "./test.db",
			},
			expectError: true,
		},
		{
			name: "missing database path",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                8080,
				DatabasePath:        "",
			},
			expectError: true,
		},
		{
			name: "invalid max link depth (negative)",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                8080,
				DatabasePath:        "./test.db",
				RedisAddr:           "localhost:6379",
				WorkerConcurrency:   10,
				MaxLinkDepth:        -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("Expected validation error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}
