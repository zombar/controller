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
	os.Setenv("DB_HOST", "test-db-host")
	os.Setenv("DB_PORT", "5433")
	os.Setenv("DB_USER", "testuser")
	os.Setenv("DB_PASSWORD", "testpass")
	os.Setenv("DB_NAME", "testdb")
	defer func() {
		os.Unsetenv("SCRAPER_BASE_URL")
		os.Unsetenv("TEXTANALYZER_BASE_URL")
		os.Unsetenv("CONTROLLER_PORT")
		os.Unsetenv("DB_HOST")
		os.Unsetenv("DB_PORT")
		os.Unsetenv("DB_USER")
		os.Unsetenv("DB_PASSWORD")
		os.Unsetenv("DB_NAME")
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
	if cfg.DBHost != "test-db-host" {
		t.Errorf("Expected DBHost 'test-db-host', got '%s'", cfg.DBHost)
	}
	if cfg.DBPort != 5433 {
		t.Errorf("Expected DBPort 5433, got %d", cfg.DBPort)
	}
	if cfg.DBUser != "testuser" {
		t.Errorf("Expected DBUser 'testuser', got '%s'", cfg.DBUser)
	}
	if cfg.DBPassword != "testpass" {
		t.Errorf("Expected DBPassword 'testpass', got '%s'", cfg.DBPassword)
	}
	if cfg.DBName != "testdb" {
		t.Errorf("Expected DBName 'testdb', got '%s'", cfg.DBName)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Clear environment variables to test defaults
	os.Unsetenv("SCRAPER_BASE_URL")
	os.Unsetenv("TEXTANALYZER_BASE_URL")
	os.Unsetenv("CONTROLLER_PORT")
	os.Unsetenv("DB_HOST")
	os.Unsetenv("DB_PORT")
	os.Unsetenv("DB_USER")
	os.Unsetenv("DB_PASSWORD")
	os.Unsetenv("DB_NAME")

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
	if cfg.DBHost != "localhost" {
		t.Errorf("Expected default DBHost 'localhost', got '%s'", cfg.DBHost)
	}
	if cfg.DBPort != 5432 {
		t.Errorf("Expected default DBPort 5432, got %d", cfg.DBPort)
	}
	if cfg.DBUser != "docutab" {
		t.Errorf("Expected default DBUser 'docutab', got '%s'", cfg.DBUser)
	}
	if cfg.DBPassword != "docutab_dev_pass" {
		t.Errorf("Expected default DBPassword 'docutab_dev_pass', got '%s'", cfg.DBPassword)
	}
	if cfg.DBName != "docutab" {
		t.Errorf("Expected default DBName 'docutab', got '%s'", cfg.DBName)
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
				DBHost:     "localhost",
				DBPort:     5432,
				DBUser:     "postgres",
				DBPassword: "postgres",
				DBName:     "docutab",
				RedisAddr:           "localhost:6379",
				WorkerConcurrency:   10,
				MaxLinkDepth:        1,
				TombstoneTags:       []string{"low-quality", "sparse-content"},
				TombstonePeriodLowScore: 30,
				TombstonePeriodTagBased: 90,
				TombstonePeriodManual:   90,
			},
			expectError: false,
		},
		{
			name: "missing scraper URL",
			config: &Config{
				ScraperBaseURL:      "",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                8080,
				DBHost:     "localhost",
				DBPort:     5432,
				DBUser:     "postgres",
				DBPassword: "postgres",
				DBName:     "docutab",
			},
			expectError: true,
		},
		{
			name: "missing text analyzer URL",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "",
				Port:                8080,
				DBHost:     "localhost",
				DBPort:     5432,
				DBUser:     "postgres",
				DBPassword: "postgres",
				DBName:     "docutab",
			},
			expectError: true,
		},
		{
			name: "invalid port (too low)",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                0,
				DBHost:     "localhost",
				DBPort:     5432,
				DBUser:     "postgres",
				DBPassword: "postgres",
				DBName:     "docutab",
			},
			expectError: true,
		},
		{
			name: "invalid port (too high)",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                70000,
				DBHost:     "localhost",
				DBPort:     5432,
				DBUser:     "postgres",
				DBPassword: "postgres",
				DBName:     "docutab",
			},
			expectError: true,
		},
		{
			name: "missing database path",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                8080,
				DBHost:              "",
			DBPort:              5432,
			DBUser:              "postgres",
			DBPassword:          "postgres",
			DBName:              "docutab",
			},
			expectError: true,
		},
		{
			name: "invalid max link depth (negative)",
			config: &Config{
				ScraperBaseURL:      "http://localhost:8081",
				TextAnalyzerBaseURL: "http://localhost:8082",
				Port:                8080,
				DBHost:     "localhost",
				DBPort:     5432,
				DBUser:     "postgres",
				DBPassword: "postgres",
				DBName:     "docutab",
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
