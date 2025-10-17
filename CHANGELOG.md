# Changelog

All notable changes to the Controller service will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-10-17

### Added
- Initial release of Controller service
- URL scraping endpoint with automatic text analysis
- Direct text analysis endpoint
- Tag-based search functionality with fuzzy matching support
- Request metadata retrieval by UUID
- Request listing with pagination
- Health check endpoint
- SQLite database with migration system
- Comprehensive test suite (config, storage, handlers)
- Environment-based configuration management
- HTTP clients for scraper and textanalyzer services
- Graceful shutdown handling
- Complete API documentation with curl examples
- Quick start guide
- Makefile for common tasks

### Database Schema
- `requests` table for storing controller metadata
- `tags` table for efficient tag searching
- `schema_version` table for migration tracking
- Indexes on frequently queried fields

### API Endpoints
- `GET /health` - Health check
- `POST /scrape` - Scrape URL and analyze text
- `POST /analyze` - Analyze text directly
- `POST /search/tags` - Search by tags (exact or fuzzy)
- `GET /requests/{id}` - Get request details
- `GET /requests` - List requests with pagination

### Configuration
- `SCRAPER_BASE_URL` - Scraper service URL
- `TEXTANALYZER_BASE_URL` - Text analyzer service URL
- `CONTROLLER_PORT` - Service port
- `DATABASE_PATH` - SQLite database file path

### Documentation
- README.md - Comprehensive project documentation
- QUICKSTART.md - Getting started guide
- API_EXAMPLES.md - Detailed API usage examples
- CHANGELOG.md - Version history

## [Unreleased]

### Future Enhancements
- PostgreSQL support (migration from SQLite)
- Request filtering by date range
- Tag statistics endpoint
- Bulk operations API
- Authentication and authorization
- Request rate limiting
- Prometheus metrics
- Structured logging with log levels
- Docker and docker-compose setup
- Kubernetes deployment manifests
- GraphQL API option
- WebSocket support for real-time updates
- Export functionality (CSV, JSON)
- Advanced search with multiple filters
- Caching layer for frequent queries

[1.0.0]: https://github.com/zombar/controller/releases/tag/v1.0.0
