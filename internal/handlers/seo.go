package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/zombar/controller/internal/seo"
	"github.com/zombar/controller/internal/templates"
)

// ServeContent serves SEO-optimized HTML content page
func (h *Handler) ServeContent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract slug from path: /content/{slug}
	slug := strings.TrimPrefix(r.URL.Path, "/content/")
	if slug == "" || slug == r.URL.Path {
		http.Error(w, "Slug is required", http.StatusBadRequest)
		return
	}

	// Get request by slug
	request, err := h.storage.GetRequestBySlug(slug)
	if err != nil {
		log.Printf("Error getting request by slug %s: %v", slug, err)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if request == nil {
		http.Error(w, "Content not found", http.StatusNotFound)
		return
	}

	// Check if SEO is enabled for this document
	if !request.SEOEnabled {
		log.Printf("SEO disabled for request %s (slug: %s)", request.ID, slug)
		http.Error(w, "SEO page not available for this content", http.StatusNotFound)
		return
	}

	// Extract metadata
	scraperMeta, _ := request.Metadata["scraper_metadata"].(map[string]interface{})
	textMeta, _ := request.Metadata["text_analysis"].(map[string]interface{})

	// Get title, description, content from metadata
	title := getString(scraperMeta, "title", "Untitled")
	description := getString(scraperMeta, "description", "")
	rawContent := getString(textMeta, "content", getString(scraperMeta, "content", ""))
	content := formatContentHTML(rawContent)

	// Get author and validate it's not a URL
	author := getString(scraperMeta, "author", "")
	if isURL(author) {
		log.Printf("Author field contains URL, clearing it: %s", author)
		author = ""
	}

	// Get base URL from config or request (needed early for image insertion)
	baseURL := getBaseURL(r)

	// Get keywords from tags
	keywords := request.Tags
	canonicalURL := fmt.Sprintf("%s/content/%s", baseURL, slug)

	// Select best thumbnail based on relevance score
	var ogImage string
	var bestImageSlug string
	log.Printf("DEBUG: Processing images for slug %s, scraperBaseURL=%s", slug, h.scraperBaseURL)
	if images, ok := scraperMeta["images"].([]interface{}); ok && len(images) > 0 {
		log.Printf("DEBUG: Found %d images in metadata", len(images))
		// Find image with highest relevance score
		var bestScore float64 = -1
		for _, imgInterface := range images {
			if img, ok := imgInterface.(map[string]interface{}); ok {
				imgSlug, hasSlug := img["slug"].(string)
				if !hasSlug || imgSlug == "" {
					continue
				}

				// Get relevance score (default to 0.5 if not present)
				relevanceScore := 0.5
				if score, ok := img["relevance_score"].(float64); ok {
					relevanceScore = score
				}

				if relevanceScore > bestScore {
					bestScore = relevanceScore
					bestImageSlug = imgSlug
				}
			}
		}

		// Use best scored image as OG image (served by scraper service)
		if bestImageSlug != "" {
			ogImage = fmt.Sprintf("%s/images/%s", h.scraperBaseURL, bestImageSlug)
			log.Printf("Selected thumbnail %s with relevance score %.2f, URL: %s", bestImageSlug, bestScore, ogImage)

			// Insert image midway through content (use scraper service URL)
			log.Printf("DEBUG: Inserting image into content with baseURL=%s, slug=%s", h.scraperBaseURL, bestImageSlug)
			content = insertImageInContent(content, h.scraperBaseURL, bestImageSlug)
			log.Printf("DEBUG: Content length after image insertion: %d", len(content))
		} else {
			log.Printf("DEBUG: No bestImageSlug found")
		}
	} else {
		log.Printf("DEBUG: No images found in scraper metadata")
	}

	// Generate JSON-LD schema
	schemaData := seo.ArticleData{
		Title:         title,
		Description:   description,
		Author:        author,
		PublishedDate: request.CreatedAt,
		ModifiedDate:  request.CreatedAt,
		Keywords:      keywords,
		Content:       content,
		URL:           canonicalURL,
	}

	if ogImage != "" {
		schemaData.Images = []string{ogImage}
	}

	jsonLD, err := seo.GenerateArticleSchema(schemaData)
	if err != nil {
		log.Printf("Error generating schema: %v", err)
		jsonLD = ""
	}

	// Prepare source URL (dereference pointer or use empty string)
	sourceURL := ""
	if request.SourceURL != nil {
		sourceURL = *request.SourceURL
	}

	// Render HTML template
	pageData := templates.ContentPageData{
		Title:           title,
		Description:     description,
		Content:         content,
		Author:          author,
		Keywords:        keywords,
		PublishedDate:   request.CreatedAt.Format("2006-01-02"),
		CanonicalURL:    canonicalURL,
		OGImage:         ogImage,
		JSONLDSchema:    jsonLD,
		BaseURL:         baseURL,
		WebInterfaceURL: h.webInterfaceURL,
		BestImageSlug:   bestImageSlug,   // Best scored image for mid-article insertion
		RequestID:       request.ID,      // For linking to admin interface
		ScraperBaseURL:  h.scraperBaseURL, // For image serving
		SourceURL:       sourceURL,       // Original source URL
	}

	html, err := templates.RenderContentPage(pageData)
	if err != nil {
		log.Printf("Error rendering template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(html))
}

// ServeSitemap generates and serves the XML sitemap
func (h *Handler) ServeSitemap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all requests with slugs
	requests, err := h.storage.ListRequests(1000, 0) // Get up to 1000 entries
	if err != nil {
		log.Printf("Error listing requests for sitemap: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Build sitemap entries
	entries := make([]seo.SitemapEntry, 0)
	for _, req := range requests {
		// Skip requests without slugs or with SEO disabled
		if req.Slug == nil || *req.Slug == "" || !req.SEOEnabled {
			continue
		}

		entry := seo.SitemapEntry{
			Slug:       *req.Slug,
			UpdatedAt:  req.CreatedAt,
			ChangeFreq: seo.DefaultChangeFreq(),
			Priority:   seo.DefaultPriority(),
		}
		entries = append(entries, entry)
	}

	// Generate sitemap XML
	baseURL := getBaseURL(r)
	xmlData, err := seo.GenerateSitemap(baseURL, entries)
	if err != nil {
		log.Printf("Error generating sitemap: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	w.WriteHeader(http.StatusOK)
	w.Write(xmlData)
}

// ServeImageSitemap generates and serves the XML image sitemap
func (h *Handler) ServeImageSitemap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Note: Images are stored in the Scraper service, not in the Controller database.
	// For now, we generate an empty sitemap. In the future, this could query the Scraper
	// service to get all images and include them in the sitemap.

	// Build empty image sitemap entries for now
	entries := make([]seo.ImageSitemapEntry, 0)

	// TODO: Query Scraper service for images with slugs
	// This would require an HTTP call to the Scraper service's image listing endpoint

	// Generate image sitemap XML
	baseURL := getBaseURL(r)
	xmlData, err := seo.GenerateImageSitemap(baseURL, entries)
	if err != nil {
		log.Printf("Error generating image sitemap: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")

	w.WriteHeader(http.StatusOK)
	w.Write(xmlData)
}

// ServeRobotsTxt serves the robots.txt file
func (h *Handler) ServeRobotsTxt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	baseURL := getBaseURL(r)
	robotsTxt := fmt.Sprintf(`User-agent: *
Allow: /

Sitemap: %s/sitemap.xml
Sitemap: %s/images-sitemap.xml
`, baseURL, baseURL)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(robotsTxt))
}

// ServeImage serves an image by slug from the scraper service
func (h *Handler) ServeImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract slug from path: /images/{slug}
	slug := strings.TrimPrefix(r.URL.Path, "/images/")
	if slug == "" || slug == r.URL.Path {
		http.Error(w, "Slug is required", http.StatusBadRequest)
		return
	}

	// Search for image by slug in scraper service
	// We need to get the image ID first, then serve the file
	// For now, we'll construct the URL to proxy to the scraper service
	// The scraper service will need to support image serving by slug

	// Proxy the request to the scraper service
	// For now, return a not implemented error - this needs to be wired up properly
	http.Error(w, "Image serving by slug not yet implemented - use scraper service directly", http.StatusNotImplemented)

	// TODO: Implement proper image lookup by slug and proxying from scraper service
	// This requires either:
	// 1. Adding a /images/{slug} endpoint to the scraper service
	// 2. Or looking up the image ID by slug and proxying to /api/images/{id}/file
}

// Helper functions

func getString(m map[string]interface{}, key, defaultValue string) string {
	if m == nil {
		return defaultValue
	}
	if val, ok := m[key].(string); ok {
		return val
	}
	return defaultValue
}

// isURL checks if a string appears to be a URL
func isURL(s string) bool {
	s = strings.TrimSpace(s)
	// Check for common URL patterns
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "www.") ||
		strings.Contains(s, "://")
}

func getBaseURL(r *http.Request) string {
	// Try to get from X-Forwarded-Proto and Host headers
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = fwdHost
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func formatContentHTML(content string) string {
	if content == "" {
		return ""
	}

	// Split by double newlines to get paragraphs
	paragraphs := strings.Split(content, "\n\n")

	var formatted strings.Builder
	for _, para := range paragraphs {
		// Trim whitespace
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// Replace single newlines within paragraphs with <br>
		para = strings.ReplaceAll(para, "\n", "<br>")

		// Wrap in paragraph tags
		formatted.WriteString("<p>")
		formatted.WriteString(para)
		formatted.WriteString("</p>\n")
	}

	return formatted.String()
}

// insertImageInContent inserts an image midway through the HTML content
func insertImageInContent(content, baseURL, imageSlug string) string {
	if content == "" || imageSlug == "" {
		return content
	}

	// Split content into paragraphs (assuming it's already formatted as HTML)
	paragraphs := strings.Split(content, "</p>")

	// Filter out empty paragraphs
	nonEmptyParagraphs := 0
	for _, p := range paragraphs {
		if strings.TrimSpace(p) != "" {
			nonEmptyParagraphs++
		}
	}

	// Need at least 3 paragraphs (since split creates extra empty entry)
	if nonEmptyParagraphs < 3 {
		return content // Not enough content to split
	}

	// Find midpoint
	midpoint := len(paragraphs) / 2

	// Create image HTML with pixel-perfect scaling and responsive design
	imageHTML := fmt.Sprintf(`
<figure class="article-image" style="margin: 2rem 0; text-align: center;">
	<img src="%s/images/%s"
	     alt="Article illustration"
	     style="max-width: 100%%; height: auto; display: block; margin: 0 auto; image-rendering: -webkit-optimize-contrast; image-rendering: crisp-edges;"
	     loading="lazy">
</figure>`, baseURL, imageSlug)

	// Insert image at midpoint
	var result strings.Builder
	for i, para := range paragraphs {
		if para != "" {
			result.WriteString(para)
			result.WriteString("</p>")
		}

		// Insert image after midpoint paragraph
		if i == midpoint {
			result.WriteString(imageHTML)
		}
	}

	return result.String()
}
