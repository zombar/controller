package seo

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestGenerateArticleSchema(t *testing.T) {
	data := ArticleData{
		Title:         "Test Article",
		Description:   "This is a test article description",
		Author:        "John Doe",
		PublishedDate: time.Date(2025, 10, 22, 10, 0, 0, 0, time.UTC),
		ModifiedDate:  time.Date(2025, 10, 22, 11, 0, 0, 0, time.UTC),
		Images:        []string{"https://example.com/image1.jpg", "https://example.com/image2.jpg"},
		Keywords:      []string{"technology", "programming", "web"},
		Content:       "Full article content here...",
		URL:           "https://example.com/content/test-article",
	}

	jsonLD, err := GenerateArticleSchema(data)
	if err != nil {
		t.Fatalf("Failed to generate article schema: %v", err)
	}

	// Verify it's valid JSON
	var schema ArticleSchema
	if err := json.Unmarshal([]byte(jsonLD), &schema); err != nil {
		t.Fatalf("Generated JSON-LD is not valid JSON: %v", err)
	}

	// Verify schema fields
	if schema.Context != "https://schema.org" {
		t.Errorf("Expected context 'https://schema.org', got '%s'", schema.Context)
	}

	if schema.Type != "Article" {
		t.Errorf("Expected type 'Article', got '%s'", schema.Type)
	}

	if schema.Headline != data.Title {
		t.Errorf("Expected headline '%s', got '%s'", data.Title, schema.Headline)
	}

	if schema.Description != data.Description {
		t.Errorf("Expected description '%s', got '%s'", data.Description, schema.Description)
	}

	if schema.Author == nil || schema.Author.Name != data.Author {
		t.Error("Author not properly set in schema")
	}

	if schema.Author.Type != "Person" {
		t.Errorf("Expected author type 'Person', got '%s'", schema.Author.Type)
	}

	if schema.DatePublished == "" {
		t.Error("DatePublished should not be empty")
	}

	if schema.DateModified == "" {
		t.Error("DateModified should not be empty")
	}

	if len(schema.Image) != 2 {
		t.Errorf("Expected 2 images, got %d", len(schema.Image))
	}

	if len(schema.Keywords) != 3 {
		t.Errorf("Expected 3 keywords, got %d", len(schema.Keywords))
	}

	if schema.ArticleBody != data.Content {
		t.Error("ArticleBody does not match content")
	}

	if schema.URL != data.URL {
		t.Errorf("Expected URL '%s', got '%s'", data.URL, schema.URL)
	}
}

func TestGenerateArticleSchemaWithoutAuthor(t *testing.T) {
	data := ArticleData{
		Title:         "Test Article",
		Description:   "Description",
		PublishedDate: time.Now(),
		ModifiedDate:  time.Now(),
	}

	jsonLD, err := GenerateArticleSchema(data)
	if err != nil {
		t.Fatalf("Failed to generate article schema without author: %v", err)
	}

	var schema ArticleSchema
	if err := json.Unmarshal([]byte(jsonLD), &schema); err != nil {
		t.Fatalf("Generated JSON-LD is not valid JSON: %v", err)
	}

	// Author should be nil when not provided
	if schema.Author != nil {
		t.Error("Expected nil author when not provided")
	}
}

func TestGenerateArticleSchemaWithoutDates(t *testing.T) {
	data := ArticleData{
		Title:       "Test Article",
		Description: "Description",
	}

	jsonLD, err := GenerateArticleSchema(data)
	if err != nil {
		t.Fatalf("Failed to generate article schema without dates: %v", err)
	}

	var schema ArticleSchema
	if err := json.Unmarshal([]byte(jsonLD), &schema); err != nil {
		t.Fatalf("Generated JSON-LD is not valid JSON: %v", err)
	}

	// Dates should be empty when not provided
	if schema.DatePublished != "" {
		t.Error("Expected empty DatePublished when not provided")
	}
	if schema.DateModified != "" {
		t.Error("Expected empty DateModified when not provided")
	}
}

func TestGenerateImageObjectSchema(t *testing.T) {
	data := ImageData{
		URL:         "https://example.com/image.jpg",
		Description: "Test image description",
		Title:       "Test Image",
		Width:       1200,
		Height:      800,
	}

	jsonLD, err := GenerateImageObjectSchema(data)
	if err != nil {
		t.Fatalf("Failed to generate image object schema: %v", err)
	}

	// Verify it's valid JSON
	var schema ImageObjectSchema
	if err := json.Unmarshal([]byte(jsonLD), &schema); err != nil {
		t.Fatalf("Generated JSON-LD is not valid JSON: %v", err)
	}

	// Verify schema fields
	if schema.Context != "https://schema.org" {
		t.Errorf("Expected context 'https://schema.org', got '%s'", schema.Context)
	}

	if schema.Type != "ImageObject" {
		t.Errorf("Expected type 'ImageObject', got '%s'", schema.Type)
	}

	if schema.ContentURL != data.URL {
		t.Errorf("Expected contentUrl '%s', got '%s'", data.URL, schema.ContentURL)
	}

	if schema.Description != data.Description {
		t.Error("Description does not match")
	}

	if schema.Name != data.Title {
		t.Error("Name does not match title")
	}

	if schema.Width != data.Width {
		t.Errorf("Expected width %d, got %d", data.Width, schema.Width)
	}

	if schema.Height != data.Height {
		t.Errorf("Expected height %d, got %d", data.Height, schema.Height)
	}
}

func TestGenerateWebPageSchema(t *testing.T) {
	title := "Test Page"
	description := "Test page description"
	url := "https://example.com/test"
	published := time.Date(2025, 10, 22, 10, 0, 0, 0, time.UTC)
	modified := time.Date(2025, 10, 22, 11, 0, 0, 0, time.UTC)

	jsonLD, err := GenerateWebPageSchema(title, description, url, published, modified)
	if err != nil {
		t.Fatalf("Failed to generate web page schema: %v", err)
	}

	// Verify it's valid JSON
	var schema WebPageSchema
	if err := json.Unmarshal([]byte(jsonLD), &schema); err != nil {
		t.Fatalf("Generated JSON-LD is not valid JSON: %v", err)
	}

	// Verify schema fields
	if schema.Type != "WebPage" {
		t.Errorf("Expected type 'WebPage', got '%s'", schema.Type)
	}

	if schema.Name != title {
		t.Error("Name does not match title")
	}

	if schema.Description != description {
		t.Error("Description does not match")
	}

	if schema.URL != url {
		t.Error("URL does not match")
	}

	if schema.DatePublished == "" {
		t.Error("DatePublished should not be empty")
	}

	if schema.DateModified == "" {
		t.Error("DateModified should not be empty")
	}
}

func TestGenerateBreadcrumbSchema(t *testing.T) {
	items := []BreadcrumbItem{
		{
			Position: 1,
			Name:     "Home",
			Item:     "https://example.com",
		},
		{
			Position: 2,
			Name:     "Articles",
			Item:     "https://example.com/articles",
		},
		{
			Position: 3,
			Name:     "Test Article",
			Item:     "https://example.com/articles/test",
		},
	}

	jsonLD, err := GenerateBreadcrumbSchema(items)
	if err != nil {
		t.Fatalf("Failed to generate breadcrumb schema: %v", err)
	}

	// Verify it's valid JSON
	var schema BreadcrumbListSchema
	if err := json.Unmarshal([]byte(jsonLD), &schema); err != nil {
		t.Fatalf("Generated JSON-LD is not valid JSON: %v", err)
	}

	// Verify schema fields
	if schema.Type != "BreadcrumbList" {
		t.Errorf("Expected type 'BreadcrumbList', got '%s'", schema.Type)
	}

	if len(schema.ItemListElement) != 3 {
		t.Errorf("Expected 3 breadcrumb items, got %d", len(schema.ItemListElement))
	}

	// Verify all items have correct type
	for i, item := range schema.ItemListElement {
		if item.Type != "ListItem" {
			t.Errorf("Item %d: expected type 'ListItem', got '%s'", i, item.Type)
		}
		if item.Position != i+1 {
			t.Errorf("Item %d: expected position %d, got %d", i, i+1, item.Position)
		}
	}
}

func TestSchemaJSONFormatting(t *testing.T) {
	data := ArticleData{
		Title:       "Test",
		Description: "Description",
	}

	jsonLD, err := GenerateArticleSchema(data)
	if err != nil {
		t.Fatalf("Failed to generate schema: %v", err)
	}

	// Should be indented (pretty-printed)
	if !strings.Contains(jsonLD, "\n") {
		t.Error("Schema should be indented/pretty-printed")
	}

	// Should contain proper spacing
	if !strings.Contains(jsonLD, "  ") {
		t.Error("Schema should contain indentation spaces")
	}
}

func TestSchemaRFC3339DateFormat(t *testing.T) {
	data := ArticleData{
		Title:         "Test",
		PublishedDate: time.Date(2025, 10, 22, 10, 30, 45, 0, time.UTC),
	}

	jsonLD, err := GenerateArticleSchema(data)
	if err != nil {
		t.Fatalf("Failed to generate schema: %v", err)
	}

	// Should contain RFC3339 formatted date
	if !strings.Contains(jsonLD, "2025-10-22T10:30:45Z") {
		t.Error("Date should be formatted in RFC3339")
	}
}
