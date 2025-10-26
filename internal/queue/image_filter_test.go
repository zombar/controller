package queue

import "testing"

func TestShouldSkipURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		// Should skip - Image URLs
		{"JPEG image", "https://example.com/photo.jpg", true},
		{"PNG image", "https://example.com/image.png", true},
		{"GIF image", "https://example.com/animation.gif", true},

		// Should skip - Non-HTTP/HTTPS schemes
		{"mailto link", "mailto:user@example.com", true},
		{"tel link", "tel:+1234567890", true},
		{"telephone link", "telephone:555-1234", true},
		{"ftp link", "ftp://ftp.example.com/file.txt", true},
		{"data URI", "data:text/html,<h1>Hello</h1>", true},
		{"javascript link", "javascript:void(0)", true},
		{"file link", "file:///path/to/file", true},
		{"about link", "about:blank", true},
		{"blob link", "blob:https://example.com/blob-id", true},

		// Should NOT skip - Valid HTTP/HTTPS URLs
		{"HTTP URL", "http://example.com/article", false},
		{"HTTPS URL", "https://example.com/page", false},
		{"HTTPS with path", "https://example.com/news/article", false},
		{"HTTPS with query", "https://example.com/page?id=123", false},
		{"HTTPS with hash", "https://example.com/page#section", false},

		// Should skip - Invalid URLs
		{"Invalid URL", "not-a-valid-url", true},
		{"Empty URL", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldSkipURL(tt.url)
			if result != tt.expected {
				t.Errorf("shouldSkipURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestIsImageURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		// Image URLs
		{"JPEG image", "https://example.com/photo.jpg", true},
		{"JPEG uppercase", "https://example.com/photo.JPG", true},
		{"PNG image", "https://example.com/image.png", true},
		{"GIF image", "https://example.com/animation.gif", true},
		{"WebP image", "https://example.com/modern.webp", true},
		{"SVG image", "https://example.com/vector.svg", true},
		{"BMP image", "https://example.com/bitmap.bmp", true},
		{"ICO icon", "https://example.com/favicon.ico", true},
		{"TIFF image", "https://example.com/scan.tiff", true},
		{"Image with query", "https://example.com/photo.jpg?size=large", true},
		{"Image with hash", "https://example.com/photo.png#fragment", true},

		// Non-image URLs
		{"HTML page", "https://example.com/article.html", false},
		{"Plain URL", "https://example.com/page", false},
		{"PDF document", "https://example.com/document.pdf", false},
		{"Video file", "https://example.com/video.mp4", false},
		{"JavaScript file", "https://example.com/script.js", false},
		{"CSS file", "https://example.com/style.css", false},
		{"Text file", "https://example.com/readme.txt", false},
		{"Root path", "https://example.com/", false},
		{"Path with jpg in name", "https://example.com/jpg-guide", false},
		{"Path with png in dir", "https://example.com/png/article", false},

		// Edge cases
		{"Invalid URL", "not-a-url", false},
		{"Empty URL", "", false},
		{"URL without extension", "https://example.com/resource", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isImageURL(tt.url)
			if result != tt.expected {
				t.Errorf("isImageURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}
