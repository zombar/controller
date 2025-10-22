package seo

import (
	"encoding/xml"
	"fmt"
	"time"
)

// URLSet represents the root element of a sitemap
type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	XMLNS   string   `xml:"xmlns,attr"`
	URLs    []URL    `xml:"url"`
}

// URL represents a single URL entry in the sitemap
type URL struct {
	Loc        string  `xml:"loc"`
	LastMod    string  `xml:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

// ImageURLSet represents the root element of an image sitemap
type ImageURLSet struct {
	XMLName    xml.Name   `xml:"urlset"`
	XMLNS      string     `xml:"xmlns,attr"`
	XMLNSImage string     `xml:"xmlns:image,attr"`
	URLs       []ImageURL `xml:"url"`
}

// ImageURL represents a URL with image entries
type ImageURL struct {
	Loc    string  `xml:"loc"`
	Images []Image `xml:"image:image"`
}

// Image represents an image entry in the sitemap
type Image struct {
	Loc     string `xml:"image:loc"`
	Caption string `xml:"image:caption,omitempty"`
	Title   string `xml:"image:title,omitempty"`
}

// SitemapEntry represents a single content entry for sitemap generation
type SitemapEntry struct {
	Slug       string
	UpdatedAt  time.Time
	ChangeFreq string
	Priority   float64
}

// ImageSitemapEntry represents a single image entry for sitemap generation
type ImageSitemapEntry struct {
	Slug    string
	Caption string
	Title   string
}

// GenerateSitemap creates an XML sitemap from content entries
func GenerateSitemap(baseURL string, entries []SitemapEntry) ([]byte, error) {
	urlset := URLSet{
		XMLNS: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  make([]URL, 0, len(entries)),
	}

	for _, entry := range entries {
		url := URL{
			Loc:        fmt.Sprintf("%s/content/%s", baseURL, entry.Slug),
			LastMod:    entry.UpdatedAt.Format("2006-01-02"),
			ChangeFreq: entry.ChangeFreq,
			Priority:   entry.Priority,
		}
		urlset.URLs = append(urlset.URLs, url)
	}

	output, err := xml.MarshalIndent(urlset, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal sitemap: %w", err)
	}

	// Add XML declaration
	xmlDeclaration := []byte(xml.Header)
	return append(xmlDeclaration, output...), nil
}

// GenerateImageSitemap creates an XML image sitemap from image entries
func GenerateImageSitemap(baseURL string, entries []ImageSitemapEntry) ([]byte, error) {
	urlset := ImageURLSet{
		XMLNS:      "http://www.sitemaps.org/schemas/sitemap/0.9",
		XMLNSImage: "http://www.google.com/schemas/sitemap-image/1.1",
		URLs:       make([]ImageURL, 0),
	}

	// Group images by their parent content slug
	imagesBySlug := make(map[string][]Image)
	for _, entry := range entries {
		img := Image{
			Loc:     fmt.Sprintf("%s/images/%s", baseURL, entry.Slug),
			Caption: entry.Caption,
			Title:   entry.Title,
		}

		// For now, group all images under a common page
		// In a full implementation, you'd track which images belong to which content
		slug := entry.Slug
		imagesBySlug[slug] = append(imagesBySlug[slug], img)
	}

	// Create URL entries with images
	for slug, images := range imagesBySlug {
		imageURL := ImageURL{
			Loc:    fmt.Sprintf("%s/content/%s", baseURL, slug),
			Images: images,
		}
		urlset.URLs = append(urlset.URLs, imageURL)
	}

	output, err := xml.MarshalIndent(urlset, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal image sitemap: %w", err)
	}

	// Add XML declaration
	xmlDeclaration := []byte(xml.Header)
	return append(xmlDeclaration, output...), nil
}

// DefaultChangeFreq returns the default change frequency for content
func DefaultChangeFreq() string {
	return "weekly"
}

// DefaultPriority returns the default priority for content
func DefaultPriority() float64 {
	return 0.5
}
