package seo

import (
	"encoding/json"
	"fmt"
	"time"
)

// ArticleSchema represents a JSON-LD Article schema
type ArticleSchema struct {
	Context         string   `json:"@context"`
	Type            string   `json:"@type"`
	Headline        string   `json:"headline"`
	Description     string   `json:"description,omitempty"`
	Author          *Author  `json:"author,omitempty"`
	DatePublished   string   `json:"datePublished,omitempty"`
	DateModified    string   `json:"dateModified,omitempty"`
	Image           []string `json:"image,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
	ArticleBody     string   `json:"articleBody,omitempty"`
	URL             string   `json:"url,omitempty"`
}

// Author represents an author in JSON-LD
type Author struct {
	Type string `json:"@type"`
	Name string `json:"name"`
}

// ImageObjectSchema represents a JSON-LD ImageObject schema
type ImageObjectSchema struct {
	Context     string `json:"@context"`
	Type        string `json:"@type"`
	ContentURL  string `json:"contentUrl"`
	Description string `json:"description,omitempty"`
	Name        string `json:"name,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// WebPageSchema represents a JSON-LD WebPage schema
type WebPageSchema struct {
	Context         string `json:"@context"`
	Type            string `json:"@type"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	URL             string `json:"url,omitempty"`
	DatePublished   string `json:"datePublished,omitempty"`
	DateModified    string `json:"dateModified,omitempty"`
}

// BreadcrumbListSchema represents a JSON-LD BreadcrumbList schema
type BreadcrumbListSchema struct {
	Context         string            `json:"@context"`
	Type            string            `json:"@type"`
	ItemListElement []BreadcrumbItem  `json:"itemListElement"`
}

// BreadcrumbItem represents a single breadcrumb item
type BreadcrumbItem struct {
	Type     string `json:"@type"`
	Position int    `json:"position"`
	Name     string `json:"name"`
	Item     string `json:"item,omitempty"`
}

// ArticleData contains the data needed to generate an Article schema
type ArticleData struct {
	Title         string
	Description   string
	Author        string
	PublishedDate time.Time
	ModifiedDate  time.Time
	Images        []string
	Keywords      []string
	Content       string
	URL           string
}

// ImageData contains the data needed to generate an ImageObject schema
type ImageData struct {
	URL         string
	Description string
	Title       string
	Width       int
	Height      int
}

// GenerateArticleSchema creates a JSON-LD Article schema
func GenerateArticleSchema(data ArticleData) (string, error) {
	schema := ArticleSchema{
		Context:       "https://schema.org",
		Type:          "Article",
		Headline:      data.Title,
		Description:   data.Description,
		Image:         data.Images,
		Keywords:      data.Keywords,
		ArticleBody:   data.Content,
		URL:           data.URL,
	}

	if data.Author != "" {
		schema.Author = &Author{
			Type: "Person",
			Name: data.Author,
		}
	}

	if !data.PublishedDate.IsZero() {
		schema.DatePublished = data.PublishedDate.Format(time.RFC3339)
	}

	if !data.ModifiedDate.IsZero() {
		schema.DateModified = data.ModifiedDate.Format(time.RFC3339)
	}

	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal Article schema: %w", err)
	}

	return string(jsonBytes), nil
}

// GenerateImageObjectSchema creates a JSON-LD ImageObject schema
func GenerateImageObjectSchema(data ImageData) (string, error) {
	schema := ImageObjectSchema{
		Context:     "https://schema.org",
		Type:        "ImageObject",
		ContentURL:  data.URL,
		Description: data.Description,
		Name:        data.Title,
		Width:       data.Width,
		Height:      data.Height,
	}

	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal ImageObject schema: %w", err)
	}

	return string(jsonBytes), nil
}

// GenerateWebPageSchema creates a JSON-LD WebPage schema
func GenerateWebPageSchema(title, description, url string, published, modified time.Time) (string, error) {
	schema := WebPageSchema{
		Context:     "https://schema.org",
		Type:        "WebPage",
		Name:        title,
		Description: description,
		URL:         url,
	}

	if !published.IsZero() {
		schema.DatePublished = published.Format(time.RFC3339)
	}

	if !modified.IsZero() {
		schema.DateModified = modified.Format(time.RFC3339)
	}

	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal WebPage schema: %w", err)
	}

	return string(jsonBytes), nil
}

// GenerateBreadcrumbSchema creates a JSON-LD BreadcrumbList schema
func GenerateBreadcrumbSchema(items []BreadcrumbItem) (string, error) {
	schema := BreadcrumbListSchema{
		Context:         "https://schema.org",
		Type:            "BreadcrumbList",
		ItemListElement: items,
	}

	// Ensure all items have the correct type
	for i := range schema.ItemListElement {
		schema.ItemListElement[i].Type = "ListItem"
	}

	jsonBytes, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal BreadcrumbList schema: %w", err)
	}

	return string(jsonBytes), nil
}
