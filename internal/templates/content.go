package templates

import (
	"bytes"
	"fmt"
	"html/template"
	"math/rand"
	"strings"
	"time"
)

// ContentPageData contains data for rendering a content page
type ContentPageData struct {
	Title            string
	Description      string
	Content          string
	Author           string
	Keywords         []string
	PublishedDate    string
	ModifiedDate     string
	CanonicalURL     string
	OGImage          string
	JSONLDSchema     string
	BaseURL          string
	WebInterfaceURL  string
	BestImageSlug    string   // Best scored image for mid-article insertion
	RequestID        string   // Request ID for linking to admin interface
	ScraperBaseURL   string   // Scraper service URL for image serving
	SourceURL        string   // Original source URL for the article
}

// contentTemplate defines the HTML template for a content page
const contentTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>{{.Title}}</title>

	<!-- Meta Tags -->
	<meta name="description" content="{{.Description}}">
	{{if .Keywords}}
	<meta name="keywords" content="{{join .Keywords ", "}}">
	{{end}}
	{{if .Author}}
	<meta name="author" content="{{.Author}}">
	{{end}}
	{{if .CanonicalURL}}
	<link rel="canonical" href="{{.CanonicalURL}}">
	{{end}}

	<!-- Open Graph Tags -->
	<meta property="og:type" content="article">
	<meta property="og:title" content="{{.Title}}">
	<meta property="og:description" content="{{.Description}}">
	{{if .CanonicalURL}}
	<meta property="og:url" content="{{.CanonicalURL}}">
	{{end}}
	{{if .OGImage}}
	<meta property="og:image" content="{{.OGImage}}">
	{{end}}

	<!-- Twitter Card Tags -->
	<meta name="twitter:card" content="summary_large_image">
	<meta name="twitter:title" content="{{.Title}}">
	<meta name="twitter:description" content="{{.Description}}">
	{{if .OGImage}}
	<meta name="twitter:image" content="{{.OGImage}}">
	{{end}}

	<!-- JSON-LD Structured Data -->
	{{if .JSONLDSchema}}
	<script type="application/ld+json">
{{.JSONLDSchema}}
	</script>
	{{end}}

	<!-- Bootstrap CSS -->
	<link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.2/dist/css/bootstrap.min.css" rel="stylesheet">

	<style>
		:root {
			--purple-primary: #6A0DAD;
			--purple-dark: #3d0766;
			--purple-darker: #2d0550;
		}
		body {
			background: linear-gradient(180deg,
				#0d0d0d 0%,
				#1a1a1a 50%,
				#0d0d0d 100%
			);
			background-attachment: fixed;
			min-height: 100vh;
			padding-bottom: 2rem;
		}
		.container {
			margin-top: 2rem;
		}
		.content-container {
			background-color: #f8f9fa;
			border-radius: 0;
			box-shadow:
				0 0 20px rgba(167, 139, 250, 0.02),
				0 0 40px rgba(167, 139, 250, 0.015),
				0 8px 24px rgba(139, 92, 246, 0.02),
				0 4px 12px rgba(139, 92, 246, 0.015);
			padding: 2rem;
			max-width: 800px;
			margin: 0 auto;
			border: none;
		}
		h1 {
			color: #212529;
			border-bottom: 3px solid var(--purple-primary);
			padding-bottom: 0.5rem;
			margin-bottom: 1.5rem;
		}
		.meta {
			color: #6c757d;
			font-size: 0.9rem;
			margin-bottom: 1.5rem;
		}
		.meta time {
			font-weight: 500;
		}
		.content {
			margin-top: 2rem;
			line-height: 1.8;
			color: #212529;
		}
		.content p {
			margin-bottom: 1rem;
		}
		.keywords {
			margin: 1.5rem 0;
		}
		.keyword {
			display: inline-block;
			background-color: #e9ecef;
			color: #495057;
			padding: 0.25rem 0.75rem;
			margin: 0.25rem;
			border-radius: 0.375rem;
			font-size: 0.875rem;
			font-weight: 500;
		}
		.navbar {
			background: linear-gradient(135deg,
				var(--purple-darker) 0%,
				var(--purple-dark) 50%,
				var(--purple-primary) 100%
			) !important;
			box-shadow:
				0 0 30px rgba(167, 139, 250, 0.14),
				0 0 50px rgba(167, 139, 250, 0.084),
				0 8px 24px rgba(139, 92, 246, 0.112);
		}
		.navbar-brand {
			display: flex;
			align-items: center;
			color: white !important;
			text-decoration: none;
		}
		.purple-title .title-main {
			font-size: 2rem;
			font-weight: 600;
			line-height: 1.2;
			text-shadow:
				0 0 10px rgba(135, 206, 250, 0.6),
				0 0 20px rgba(135, 206, 250, 0.4),
				0 0 30px rgba(135, 206, 250, 0.3),
				0 0 40px rgba(135, 206, 250, 0.15),
				0 0 2px rgba(255, 255, 255, 0.54);
		}
		.purple-title .subtitle {
			font-size: 0.75rem;
			font-weight: bold;
			color: rgba(255, 255, 255, 0.65);
			text-transform: uppercase;
		}
		footer {
			margin-top: 3rem;
			padding-top: 2rem;
			border-top: 1px solid #dee2e6;
			color: #6c757d;
			text-align: center;
			font-size: 0.875rem;
		}
		footer a {
			color: var(--purple-primary);
			text-decoration: none;
			font-weight: 600;
		}
		footer a:hover {
			color: var(--purple-dark);
			text-decoration: underline;
		}
		.original-link-box {
			display: flex;
			align-items: flex-start;
			gap: 1rem;
			padding: 1rem 1.25rem;
			margin: 2rem 0;
			background-color: #d1ecf1;
			border: 1px solid #bee5eb;
			border-radius: 0.375rem;
			color: #0c5460;
		}
		.original-link-icon {
			font-size: 1.5rem;
			line-height: 1;
			flex-shrink: 0;
		}
		.original-link-content {
			flex: 1;
		}
		.original-link-content strong {
			display: block;
			margin-bottom: 0.25rem;
			color: #0c5460;
		}
		.original-link {
			color: #0c5460;
			text-decoration: underline;
			font-weight: 600;
		}
		.original-link:hover {
			color: #062c33;
		}
	</style>
</head>
<body>
	<!-- Navigation -->
	<nav class="navbar navbar-dark">
		<div class="container">
			<a href="{{.WebInterfaceURL}}?doc={{.RequestID}}" class="navbar-brand mb-0 purple-title" style="text-decoration: none;">
				<div style="display: flex; flex-direction: column;">
					<span class="title-main">PurpleTab</span>
					<span class="subtitle">For The Truth Seekers</span>
				</div>
			</a>
		</div>
	</nav>

	<!-- Main Content -->
	<div class="container">
		<div class="content-container">
			<article>
				<h1>{{.Title}}</h1>

				{{if or .Author .PublishedDate}}
				<div class="meta">
					{{if .Author}}<span>By <strong>{{.Author}}</strong></span>{{end}}
					{{if and .Author .PublishedDate}} • {{end}}
					{{if .PublishedDate}}<time datetime="{{.PublishedDate}}">{{.PublishedDate}}</time>{{end}}
				</div>
				{{end}}

				{{if .Keywords}}
				<div class="keywords">
					{{range .Keywords}}
					<span class="keyword">{{.}}</span>
					{{end}}
				</div>
				{{end}}

				<div class="content">
					{{.Content | safeHTML}}
				</div>

				{{if .SourceURL}}
				<div class="original-link-box">
					<div class="original-link-icon">ℹ️</div>
					<div class="original-link-content">
						<strong>{{randomPhrase}}</strong>
						<a href="{{.SourceURL}}" target="_blank" rel="noopener noreferrer" class="original-link">
							View the original article
						</a>
					</div>
				</div>
				{{end}}
			</article>

			<footer>
				<p class="mb-0">Powered by <a href="{{.WebInterfaceURL}}?doc={{.RequestID}}">PurpleTab</a></p>
			</footer>
		</div>
	</div>
</body>
</html>`

// Humorous and encouraging phrases for the original article link
var originalArticlePhrases = []string{
	"Feeling adventurous? 🚀",
	"Want to see where this all began? 🔍",
	"Curious about the source? 🤔",
	"Ready for the full story? 📖",
	"Let's visit the OG article! 🎯",
	"Time to see the original masterpiece! 🎨",
	"Brave enough for the source? 💪",
	"Want the unfiltered version? 🎭",
	"Let's check out the mothership! 🛸",
	"Feeling like an investigator? 🕵️",
	"Ready to go down the rabbit hole? 🐰",
	"Shall we visit the primary source? 📚",
	"Fancy seeing where it all started? 🌟",
	"Up for some fact-checking? ✅",
	"Let's see what the original author said! 👀",
}

// Initialize random seed
func init() {
	rand.Seed(time.Now().UnixNano())
}

// getRandomPhrase returns a random phrase from the list
func getRandomPhrase() string {
	return originalArticlePhrases[rand.Intn(len(originalArticlePhrases))]
}

// RenderContentPage renders a content page with SEO optimizations
func RenderContentPage(data ContentPageData) (string, error) {
	// Create template with custom functions
	funcMap := template.FuncMap{
		"join": strings.Join,
		"safeHTML": func(s string) template.HTML {
			return template.HTML(s)
		},
		"randomPhrase": getRandomPhrase,
	}

	tmpl, err := template.New("content").Funcs(funcMap).Parse(contentTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}
