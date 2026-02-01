package web

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// ContentExtractor extracts meaningful content from HTML using goquery.
type ContentExtractor struct {
	// Selectors to try for main content, in order of preference
	contentSelectors []string

	// Selectors for elements to remove before extraction
	removeSelectors []string
}

// NewContentExtractor creates a new content extractor with default settings.
func NewContentExtractor() *ContentExtractor {
	return &ContentExtractor{
		contentSelectors: []string{
			"article",
			"main",
			"[role='main']",
			".article-content",
			".post-content",
			".entry-content",
			".article-body",
			".story-body",
			".content",
			"#content",
			".main-content",
			"#main-content",
		},
		removeSelectors: []string{
			"script",
			"style",
			"noscript",
			"nav",
			"footer",
			"header",
			"aside",
			".sidebar",
			".menu",
			".navigation",
			".nav",
			".ad",
			".ads",
			".advertisement",
			".social-share",
			".share-buttons",
			".comments",
			".comment-section",
			".related-posts",
			".recommended",
			".newsletter",
			".subscribe",
			".popup",
			".modal",
			"[role='navigation']",
			"[role='banner']",
			"[role='contentinfo']",
			"[aria-hidden='true']",
		},
	}
}

// ExtractContent extracts the main content from HTML.
func (e *ContentExtractor) ExtractContent(html string, baseURL string) (*PageContent, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	// Get title
	title := strings.TrimSpace(doc.Find("title").Text())

	// Remove unwanted elements
	for _, sel := range e.removeSelectors {
		doc.Find(sel).Remove()
	}

	// Try to find main content using selectors
	var content string
	var contentSelection *goquery.Selection

	for _, sel := range e.contentSelectors {
		selection := doc.Find(sel)
		if selection.Length() > 0 {
			text := e.cleanText(selection.Text())
			if len(text) > 200 { // Substantial content
				content = text
				contentSelection = selection
				break
			}
		}
	}

	// Fallback to body
	if content == "" {
		content = e.cleanText(doc.Find("body").Text())
		contentSelection = doc.Find("body")
	}

	// Calculate word count and reading time
	words := len(strings.Fields(content))
	readingTime := words / 200 // Average reading speed
	if readingTime < 1 && words > 0 {
		readingTime = 1
	}

	// Extract excerpt (first ~200 chars)
	excerpt := content
	if len(excerpt) > 200 {
		excerpt = excerpt[:200]
		// Try to end at a word boundary
		if lastSpace := strings.LastIndex(excerpt, " "); lastSpace > 150 {
			excerpt = excerpt[:lastSpace]
		}
		excerpt += "..."
	}

	result := &PageContent{
		URL:                baseURL,
		Title:              title,
		Content:            content,
		Excerpt:            excerpt,
		WordCount:          words,
		ReadingTimeMinutes: readingTime,
	}

	// Extract links if we have a content selection
	if contentSelection != nil {
		result.Links = e.extractLinks(contentSelection, baseURL)
	}

	return result, nil
}

// ExtractLinks extracts all links from the HTML.
func (e *ContentExtractor) ExtractLinks(html string, baseURL string) ([]Link, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	return e.extractLinks(doc.Selection, baseURL), nil
}

// extractLinks extracts links from a goquery selection.
func (e *ContentExtractor) extractLinks(selection *goquery.Selection, baseURL string) []Link {
	var links []Link
	seen := make(map[string]bool)

	parsedBase, _ := url.Parse(baseURL)

	selection.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" || href == "#" || strings.HasPrefix(href, "javascript:") {
			return
		}

		// Resolve relative URL
		absoluteURL := e.resolveURL(baseURL, href)

		// Skip duplicates
		if seen[absoluteURL] {
			return
		}
		seen[absoluteURL] = true

		// Get link text
		text := strings.TrimSpace(s.Text())
		if text == "" {
			// Try alt text from image
			if img := s.Find("img"); img.Length() > 0 {
				text, _ = img.Attr("alt")
			}
		}

		// Get title attribute
		title, _ := s.Attr("title")

		// Determine if external
		external := false
		if parsedBase != nil {
			if parsedLink, err := url.Parse(absoluteURL); err == nil {
				external = parsedLink.Host != "" && parsedLink.Host != parsedBase.Host
			}
		}

		links = append(links, Link{
			URL:      absoluteURL,
			Text:     text,
			Title:    title,
			External: external,
		})
	})

	return links
}

// ExtractText extracts text from a specific selector.
func (e *ContentExtractor) ExtractText(html string, selector string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	selection := doc.Find(selector)
	if selection.Length() == 0 {
		return "", nil
	}

	return e.cleanText(selection.Text()), nil
}

// ExtractAttribute extracts an attribute value from elements matching a selector.
func (e *ContentExtractor) ExtractAttribute(html string, selector string, attribute string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var values []string
	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		if val, exists := s.Attr(attribute); exists {
			values = append(values, val)
		}
	})

	return values, nil
}

// cleanText normalizes whitespace and removes excess spacing.
func (e *ContentExtractor) cleanText(text string) string {
	// Replace multiple whitespace with single space
	re := regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")

	// Trim leading/trailing whitespace
	text = strings.TrimSpace(text)

	return text
}

// resolveURL resolves a potentially relative URL against a base URL.
func (e *ContentExtractor) resolveURL(baseURL, href string) string {
	// If already absolute, return as-is
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}

	ref, err := url.Parse(href)
	if err != nil {
		return href
	}

	return base.ResolveReference(ref).String()
}

// GetMetaContent extracts content from a meta tag.
func (e *ContentExtractor) GetMetaContent(html string, name string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return "", err
	}

	// Try name attribute
	if val, exists := doc.Find("meta[name='" + name + "']").Attr("content"); exists {
		return val, nil
	}

	// Try property attribute (for Open Graph)
	if val, exists := doc.Find("meta[property='" + name + "']").Attr("content"); exists {
		return val, nil
	}

	return "", nil
}

// GetPageInfo extracts basic page information.
func (e *ContentExtractor) GetPageInfo(html string, pageURL string) (*PageInfo, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(doc.Find("title").Text())

	return &PageInfo{
		URL:   pageURL,
		Title: title,
	}, nil
}
