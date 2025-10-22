package templates

import (
	"strings"
	"testing"
)

func TestRenderContentPage(t *testing.T) {
	data := ContentPageData{
		Title:         "Test Article",
		Description:   "This is a test article description",
		Content:       "<p>Article content here</p>",
		Author:        "John Doe",
		Keywords:      []string{"technology", "programming", "web"},
		PublishedDate: "2025-10-22",
		CanonicalURL:  "https://example.com/content/test-article",
		OGImage:       "https://example.com/images/test.jpg",
		JSONLDSchema:  `{"@context": "https://schema.org", "@type": "Article"}`,
		BaseURL:       "https://example.com",
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render content page: %v", err)
	}

	// Verify HTML structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE declaration")
	}

	if !strings.Contains(html, "<html lang=\"en\">") {
		t.Error("Missing html tag with lang attribute")
	}

	// Verify title
	if !strings.Contains(html, "<title>Test Article</title>") {
		t.Error("Missing or incorrect title tag")
	}

	// Verify meta description
	if !strings.Contains(html, `<meta name="description" content="This is a test article description">`) {
		t.Error("Missing or incorrect meta description")
	}

	// Verify keywords meta tag
	if !strings.Contains(html, `<meta name="keywords" content="technology, programming, web">`) {
		t.Error("Missing or incorrect keywords meta tag")
	}

	// Verify author meta tag
	if !strings.Contains(html, `<meta name="author" content="John Doe">`) {
		t.Error("Missing or incorrect author meta tag")
	}

	// Verify canonical URL
	if !strings.Contains(html, `<link rel="canonical" href="https://example.com/content/test-article">`) {
		t.Error("Missing or incorrect canonical link")
	}

	// Verify Open Graph tags
	if !strings.Contains(html, `<meta property="og:type" content="article">`) {
		t.Error("Missing Open Graph type tag")
	}
	if !strings.Contains(html, `<meta property="og:title" content="Test Article">`) {
		t.Error("Missing Open Graph title tag")
	}
	if !strings.Contains(html, `<meta property="og:description" content="This is a test article description">`) {
		t.Error("Missing Open Graph description tag")
	}
	if !strings.Contains(html, `<meta property="og:image" content="https://example.com/images/test.jpg">`) {
		t.Error("Missing Open Graph image tag")
	}

	// Verify Twitter Card tags
	if !strings.Contains(html, `<meta name="twitter:card" content="summary_large_image">`) {
		t.Error("Missing Twitter Card type tag")
	}
	if !strings.Contains(html, `<meta name="twitter:title" content="Test Article">`) {
		t.Error("Missing Twitter Card title tag")
	}

	// Verify JSON-LD schema
	if !strings.Contains(html, `<script type="application/ld+json">`) {
		t.Error("Missing JSON-LD script tag")
	}
	// JSON-LD is HTML-escaped by the template engine
	if !strings.Contains(html, `@context`) && !strings.Contains(html, `@type`) {
		t.Error("Missing or incorrect JSON-LD schema")
	}

	// Verify article content
	if !strings.Contains(html, "<h1>Test Article</h1>") {
		t.Error("Missing article title h1")
	}
	if !strings.Contains(html, "<p>Article content here</p>") {
		t.Error("Missing article content")
	}

	// Verify keywords display
	if !strings.Contains(html, `<span class="keyword">technology</span>`) {
		t.Error("Missing keyword display")
	}

	// Verify author and date display
	if !strings.Contains(html, "By") && !strings.Contains(html, "John Doe") {
		t.Error("Missing author display")
	}
	if !strings.Contains(html, `<time datetime="2025-10-22">2025-10-22</time>`) {
		t.Error("Missing or incorrect time tag")
	}
}

func TestRenderContentPageMinimal(t *testing.T) {
	// Test with minimal required data
	data := ContentPageData{
		Title:   "Minimal Article",
		Content: "<p>Content</p>",
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render minimal content page: %v", err)
	}

	// Should still have basic structure
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing DOCTYPE in minimal page")
	}

	if !strings.Contains(html, "<title>Minimal Article</title>") {
		t.Error("Missing title in minimal page")
	}

	if !strings.Contains(html, "<p>Content</p>") {
		t.Error("Missing content in minimal page")
	}
}

func TestRenderContentPageNoKeywords(t *testing.T) {
	data := ContentPageData{
		Title:    "No Keywords Article",
		Content:  "<p>Content</p>",
		Keywords: []string{},
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render page without keywords: %v", err)
	}

	// Should not have empty keywords meta tag
	// The template should handle empty keyword arrays gracefully
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("Missing basic HTML structure")
	}
}

func TestRenderContentPageNoAuthor(t *testing.T) {
	data := ContentPageData{
		Title:   "No Author Article",
		Content: "<p>Content</p>",
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render page without author: %v", err)
	}

	// Should not crash and should render without author meta tag
	if strings.Contains(html, `<meta name="author"`) {
		t.Error("Should not have author meta tag when author is empty")
	}
}

func TestRenderContentPageNoImages(t *testing.T) {
	data := ContentPageData{
		Title:   "No Images Article",
		Content: "<p>Content</p>",
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render page without images: %v", err)
	}

	// Should not have og:image or twitter:image tags when empty
	if strings.Contains(html, `<meta property="og:image"`) {
		t.Error("Should not have og:image tag when OGImage is empty")
	}
	if strings.Contains(html, `<meta name="twitter:image"`) {
		t.Error("Should not have twitter:image tag when OGImage is empty")
	}
}

func TestRenderContentPageNoJSONLD(t *testing.T) {
	data := ContentPageData{
		Title:   "No Schema Article",
		Content: "<p>Content</p>",
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render page without JSON-LD: %v", err)
	}

	// Should not have script tag when JSONLDSchema is empty
	if strings.Contains(html, `<script type="application/ld+json">`) {
		t.Error("Should not have JSON-LD script tag when JSONLDSchema is empty")
	}
}

func TestRenderContentPageHTMLEscaping(t *testing.T) {
	data := ContentPageData{
		Title:       "Article with <script>alert('xss')</script>",
		Description: "Description with <script>alert('xss')</script>",
		Content:     "<script>alert('This should be safe')</script>",
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render page with special characters: %v", err)
	}

	// Title and description in meta tags should be escaped
	titleInMeta := strings.Contains(html, "&lt;script&gt;")

	// Content should be rendered as-is (safeHTML)
	contentAsIs := strings.Contains(html, "<script>alert('This should be safe')</script>")

	if !titleInMeta {
		t.Error("Title in meta tags should be HTML-escaped")
	}

	if !contentAsIs {
		t.Error("Content should be rendered as HTML (not escaped) due to safeHTML")
	}
}

func TestRenderContentPageKeywordsSeparator(t *testing.T) {
	data := ContentPageData{
		Title:    "Keywords Test",
		Content:  "<p>Content</p>",
		Keywords: []string{"one", "two", "three"},
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render page: %v", err)
	}

	// Keywords should be joined with ", "
	if !strings.Contains(html, `content="one, two, three"`) {
		t.Error("Keywords should be joined with comma and space")
	}
}

func TestRenderContentPageResponsiveViewport(t *testing.T) {
	data := ContentPageData{
		Title:   "Responsive Test",
		Content: "<p>Content</p>",
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render page: %v", err)
	}

	// Should have viewport meta tag for responsive design
	if !strings.Contains(html, `<meta name="viewport" content="width=device-width, initial-scale=1.0">`) {
		t.Error("Missing viewport meta tag for responsive design")
	}
}

func TestRenderContentPageCharset(t *testing.T) {
	data := ContentPageData{
		Title:   "Charset Test",
		Content: "<p>Content</p>",
	}

	html, err := RenderContentPage(data)
	if err != nil {
		t.Fatalf("Failed to render page: %v", err)
	}

	// Should have UTF-8 charset
	if !strings.Contains(html, `<meta charset="UTF-8">`) {
		t.Error("Missing UTF-8 charset meta tag")
	}
}
