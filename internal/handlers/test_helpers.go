package handlers

import (
	"crypto/md5"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// setupTestDB creates a test PostgreSQL database connection string
// It uses environment variables or defaults to localhost
// Tests will skip if PostgreSQL is not available
func setupTestDB(t *testing.T, testName string) (connStr string, cleanup func()) {
	t.Helper()

	// Get PostgreSQL connection parameters from environment or use defaults
	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5432")
	user := getEnvOrDefault("TEST_DB_USER", "postgres")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "postgres")

	// Create a unique database name for this test
	// PostgreSQL has a 63 character limit for identifiers, so hash long names
	timestamp := time.Now().UnixNano()
	var dbName string
	if len(testName) > 40 {
		// Hash long test names to stay under the 63 character limit
		hash := md5.Sum([]byte(testName))
		dbName = fmt.Sprintf("test_%x_%d", hash[:8], timestamp)
	} else {
		dbName = fmt.Sprintf("test_%s_%d", testName, timestamp)
	}

	// Connect to 'docutab' database to create test database
	adminConnStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=docutab sslmode=disable connect_timeout=5",
		host, port, user, password)

	adminDB, err := sql.Open("postgres", adminConnStr)
	if err != nil {
		t.Skipf("Could not connect to PostgreSQL for testing: %v (set TEST_DB_* env vars if needed)", err)
		return "", func() {}
	}
	defer adminDB.Close()

	// Test connection
	if err := adminDB.Ping(); err != nil {
		t.Skipf("Could not ping PostgreSQL for testing: %v", err)
		return "", func() {}
	}

	// Create test database
	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE \"%s\"", dbName))
	if err != nil {
		t.Fatalf("Could not create test database %s: %v", dbName, err)
		return "", func() {}
	}

	// Return connection string for test database and cleanup function
	testConnStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=5",
		host, port, user, password, dbName)

	cleanup = func() {
		// Reconnect to admin database to drop test database
		adminDB, err := sql.Open("postgres", adminConnStr)
		if err != nil {
			return
		}
		defer adminDB.Close()

		// Force close all connections to test database
		adminDB.Exec(fmt.Sprintf("SELECT pg_terminate_backend(pg_stat_activity.pid) FROM pg_stat_activity WHERE pg_stat_activity.datname = '%s'", dbName))

		// Drop test database
		adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
	}

	return testConnStr, cleanup
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
