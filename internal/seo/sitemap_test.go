package seo

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateSitemap(t *testing.T) {
	baseURL := "https://example.com"
	entries := []SitemapEntry{
		{
			Slug:       "article-one",
			UpdatedAt:  time.Date(2025, 10, 22, 10, 0, 0, 0, time.UTC),
			ChangeFreq: "monthly",
			Priority:   0.8,
		},
		{
			Slug:       "article-two",
			UpdatedAt:  time.Date(2025, 10, 21, 14, 30, 0, 0, time.UTC),
			ChangeFreq: "weekly",
			Priority:   0.9,
		},
	}

	xmlData, err := GenerateSitemap(baseURL, entries)
	if err != nil {
		t.Fatalf("Failed to generate sitemap: %v", err)
	}

	xmlString := string(xmlData)

	// Verify XML declaration
	if !strings.Contains(xmlString, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Error("Sitemap missing XML declaration")
	}

	// Verify urlset tag
	if !strings.Contains(xmlString, "<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">") {
		t.Error("Sitemap missing urlset tag")
	}

	// Verify first entry
	if !strings.Contains(xmlString, "<loc>https://example.com/content/article-one</loc>") {
		t.Error("Sitemap missing first article URL")
	}
	if !strings.Contains(xmlString, "<changefreq>monthly</changefreq>") {
		t.Error("Sitemap missing changefreq")
	}
	if !strings.Contains(xmlString, "<priority>0.8</priority>") {
		t.Error("Sitemap missing priority")
	}

	// Verify second entry
	if !strings.Contains(xmlString, "<loc>https://example.com/content/article-two</loc>") {
		t.Error("Sitemap missing second article URL")
	}
}

func TestGenerateSitemapEmpty(t *testing.T) {
	baseURL := "https://example.com"
	entries := []SitemapEntry{}

	xmlData, err := GenerateSitemap(baseURL, entries)
	if err != nil {
		t.Fatalf("Failed to generate empty sitemap: %v", err)
	}

	xmlString := string(xmlData)

	// Should still have valid XML structure
	if !strings.Contains(xmlString, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Error("Empty sitemap missing XML declaration")
	}
	if !strings.Contains(xmlString, "<urlset") {
		t.Error("Empty sitemap missing urlset tag")
	}
}

func TestGenerateImageSitemap(t *testing.T) {
	baseURL := "https://example.com"
	entries := []ImageSitemapEntry{
		{
			Slug:    "image-one",
			Caption: "Test Image 1",
			Title:   "Image Title 1",
		},
		{
			Slug:    "image-two",
			Caption: "Test Image 2",
			Title:   "Image Title 2",
		},
	}

	xmlData, err := GenerateImageSitemap(baseURL, entries)
	if err != nil {
		t.Fatalf("Failed to generate image sitemap: %v", err)
	}

	xmlString := string(xmlData)

	// Verify XML declaration
	if !strings.Contains(xmlString, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Error("Image sitemap missing XML declaration")
	}

	// Verify image namespace
	if !strings.Contains(xmlString, "xmlns:image=\"http://www.google.com/schemas/sitemap-image/1.1\"") {
		t.Error("Image sitemap missing image namespace")
	}

	// Verify page URLs (each image gets its own content page)
	if !strings.Contains(xmlString, "<loc>https://example.com/content/image-one</loc>") {
		t.Error("Image sitemap missing page URL for image-one")
	}
	if !strings.Contains(xmlString, "<loc>https://example.com/content/image-two</loc>") {
		t.Error("Image sitemap missing page URL for image-two")
	}

	// Verify image elements
	if !strings.Contains(xmlString, "<image:loc>https://example.com/images/image-one</image:loc>") {
		t.Error("Image sitemap missing first image URL")
	}
	if !strings.Contains(xmlString, "<image:caption>Test Image 1</image:caption>") {
		t.Error("Image sitemap missing image caption")
	}
	if !strings.Contains(xmlString, "<image:title>Image Title 1</image:title>") {
		t.Error("Image sitemap missing image title")
	}
}

func TestDefaultChangeFreq(t *testing.T) {
	changeFreq := DefaultChangeFreq()
	if changeFreq != "weekly" {
		t.Errorf("Expected default changefreq 'weekly', got '%s'", changeFreq)
	}
}

func TestDefaultPriority(t *testing.T) {
	priority := DefaultPriority()
	if priority != 0.5 {
		t.Errorf("Expected default priority 0.5, got %f", priority)
	}
}

func TestSitemapXMLEncoding(t *testing.T) {
	baseURL := "https://example.com"
	entries := []SitemapEntry{
		{
			Slug:       "article-with-special-&-chars",
			UpdatedAt:  time.Now(),
			ChangeFreq: "daily",
			Priority:   0.5,
		},
	}

	xmlData, err := GenerateSitemap(baseURL, entries)
	if err != nil {
		t.Fatalf("Failed to generate sitemap with special chars: %v", err)
	}

	xmlString := string(xmlData)

	// XML should encode special characters
	if !strings.Contains(xmlString, "&amp;") || strings.Contains(xmlString, "special-&-chars") {
		t.Error("Sitemap did not properly encode special characters")
	}
}
