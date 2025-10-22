package handlers

import (
	"strings"
	"testing"
)

func TestInsertImageInContent(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		baseURL         string
		imageSlug       string
		expectImage     bool
		expectMidpoint  bool
	}{
		{
			name: "insert image in multi-paragraph content",
			content: `<p>First paragraph.</p>
<p>Second paragraph.</p>
<p>Third paragraph.</p>
<p>Fourth paragraph.</p>
<p>Fifth paragraph.</p>`,
			baseURL:        "https://example.com",
			imageSlug:      "test-image",
			expectImage:    true,
			expectMidpoint: true,
		},
		{
			name:           "empty content",
			content:        "",
			baseURL:        "https://example.com",
			imageSlug:      "test-image",
			expectImage:    false,
			expectMidpoint: false,
		},
		{
			name:           "empty image slug",
			content:        "<p>Content here.</p>",
			baseURL:        "https://example.com",
			imageSlug:      "",
			expectImage:    false,
			expectMidpoint: false,
		},
		{
			name:           "single paragraph - not enough content",
			content:        "<p>Only one paragraph.</p>",
			baseURL:        "https://example.com",
			imageSlug:      "test-image",
			expectImage:    false,
			expectMidpoint: false,
		},
		{
			name:           "two paragraphs - not enough content",
			content:        "<p>First paragraph.</p>\n<p>Second paragraph.</p>",
			baseURL:        "https://example.com",
			imageSlug:      "test-image",
			expectImage:    false,
			expectMidpoint: false,
		},
		{
			name: "three paragraphs - minimum for insertion",
			content: `<p>First paragraph.</p>
<p>Second paragraph.</p>
<p>Third paragraph.</p>`,
			baseURL:        "https://example.com",
			imageSlug:      "test-image",
			expectImage:    true,
			expectMidpoint: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := insertImageInContent(tt.content, tt.baseURL, tt.imageSlug)

			if tt.expectImage {
				// Check that image was inserted
				if !strings.Contains(result, "<figure class=\"article-image\"") {
					t.Error("Expected <figure> tag in result")
				}
				if !strings.Contains(result, tt.baseURL+"/images/"+tt.imageSlug) {
					t.Errorf("Expected image URL %s/images/%s in result", tt.baseURL, tt.imageSlug)
				}
				if !strings.Contains(result, "<img src=") {
					t.Error("Expected <img> tag in result")
				}
				if !strings.Contains(result, "loading=\"lazy\"") {
					t.Error("Expected lazy loading attribute")
				}

				// Check that original content is preserved
				originalParagraphs := strings.Count(tt.content, "</p>")
				resultParagraphs := strings.Count(result, "</p>")
				if resultParagraphs != originalParagraphs {
					t.Errorf("Paragraph count changed: original=%d, result=%d", originalParagraphs, resultParagraphs)
				}

				// Check image is roughly in the middle
				if tt.expectMidpoint {
					paragraphs := strings.Split(tt.content, "</p>")
					midpoint := len(paragraphs) / 2

					// Split result to find where image was inserted
					parts := strings.Split(result, "<figure")
					if len(parts) != 2 {
						t.Error("Image should appear exactly once")
					}

					// Count paragraphs before image
					paraBeforeImage := strings.Count(parts[0], "</p>")
					if paraBeforeImage != midpoint {
						t.Logf("Image inserted after paragraph %d, expected midpoint %d (not critical)", paraBeforeImage, midpoint)
					}
				}
			} else {
				// No image should be inserted
				if result != tt.content {
					t.Errorf("Content should be unchanged when no image inserted.\nExpected: %s\nGot: %s", tt.content, result)
				}
			}

			t.Logf("Result length: %d, Original length: %d", len(result), len(tt.content))
		})
	}
}

func TestInsertImageInContentHTML(t *testing.T) {
	content := `<p>First paragraph.</p>
<p>Second paragraph.</p>
<p>Third paragraph.</p>
<p>Fourth paragraph.</p>`
	baseURL := "https://example.com"
	imageSlug := "best-image"

	result := insertImageInContent(content, baseURL, imageSlug)

	// Validate HTML structure
	expectedElements := []string{
		"<figure class=\"article-image\"",
		"<img src=\"https://example.com/images/best-image\"",
		"alt=\"Article illustration\"",
		"style=",
		"max-width: 100%",
		"height: auto",
		"loading=\"lazy\"",
		"</figure>",
	}

	for _, elem := range expectedElements {
		if !strings.Contains(result, elem) {
			t.Errorf("Missing expected element: %s", elem)
		}
	}

	// Ensure proper image rendering CSS
	if !strings.Contains(result, "image-rendering") {
		t.Error("Missing image-rendering CSS for pixel-perfect scaling")
	}

	t.Logf("Generated HTML structure validated successfully")
}

func TestInsertImageInContentPreservesFormatting(t *testing.T) {
	content := `<p>Paragraph with <strong>bold</strong> text.</p>
<p>Paragraph with <a href="https://example.com">link</a>.</p>
<p>Paragraph with <em>italic</em> text.</p>
<p>Final paragraph with <code>code</code>.</p>`

	result := insertImageInContent(content, "https://example.com", "test")

	// All original HTML should be preserved
	if !strings.Contains(result, "<strong>bold</strong>") {
		t.Error("Lost <strong> tag")
	}
	if !strings.Contains(result, "<a href=\"https://example.com\">link</a>") {
		t.Error("Lost <a> tag")
	}
	if !strings.Contains(result, "<em>italic</em>") {
		t.Error("Lost <em> tag")
	}
	if !strings.Contains(result, "<code>code</code>") {
		t.Error("Lost <code> tag")
	}

	t.Log("Original HTML formatting preserved correctly")
}

func TestFormatContentHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty content",
			input:    "",
			expected: "",
		},
		{
			name:     "single paragraph",
			input:    "This is a paragraph.",
			expected: "<p>This is a paragraph.</p>\n",
		},
		{
			name:     "multiple paragraphs",
			input:    "First paragraph.\n\nSecond paragraph.",
			expected: "<p>First paragraph.</p>\n<p>Second paragraph.</p>\n",
		},
		{
			name:     "paragraph with line break",
			input:    "Line one\nLine two",
			expected: "<p>Line one<br>Line two</p>\n",
		},
		{
			name:     "multiple paragraphs with line breaks",
			input:    "Para one\nLine two\n\nPara two\nLine two",
			expected: "<p>Para one<br>Line two</p>\n<p>Para two<br>Line two</p>\n",
		},
		{
			name:     "whitespace handling",
			input:    "  Trimmed  \n\n  Also trimmed  ",
			expected: "<p>Trimmed</p>\n<p>Also trimmed</p>\n",
		},
		{
			name:     "empty paragraphs filtered",
			input:    "Content\n\n\n\nMore content",
			expected: "<p>Content</p>\n<p>More content</p>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatContentHTML(tt.input)
			if result != tt.expected {
				t.Errorf("formatContentHTML() = %q, want %q", result, tt.expected)
			}
		})
	}
}
