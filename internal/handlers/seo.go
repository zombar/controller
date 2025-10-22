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

	// Extract metadata
	scraperMeta, _ := request.Metadata["scraper_metadata"].(map[string]interface{})
	textMeta, _ := request.Metadata["text_analysis"].(map[string]interface{})

	// Get title, description, content from metadata
	title := getString(scraperMeta, "title", "Untitled")
	description := getString(scraperMeta, "description", "")
	rawContent := getString(textMeta, "content", getString(scraperMeta, "content", ""))
	content := formatContentHTML(rawContent)
	author := getString(scraperMeta, "author", "")

	// Get keywords from tags
	keywords := request.Tags

	// Get base URL from config or request
	baseURL := getBaseURL(r)
	canonicalURL := fmt.Sprintf("%s/content/%s", baseURL, slug)

	// Get OG image if available
	var ogImage string
	if images, ok := scraperMeta["images"].([]interface{}); ok && len(images) > 0 {
		if firstImg, ok := images[0].(map[string]interface{}); ok {
			if imgSlug, ok := firstImg["slug"].(string); ok && imgSlug != "" {
				ogImage = fmt.Sprintf("%s/images/%s", baseURL, imgSlug)
			}
		}
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
		// Skip requests without slugs
		if req.Slug == nil || *req.Slug == "" {
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
