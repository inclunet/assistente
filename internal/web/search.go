package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

// =============================================================================
// BRAVE SEARCH API
// =============================================================================

// BraveSearch implements web search using the Brave Search API.
type BraveSearch struct {
	apiKey     string
	httpClient *http.Client
}

// NewBraveSearch creates a new Brave Search engine with the provided API key.
func NewBraveSearch(apiKey string) *BraveSearch {
	return &BraveSearch{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsConfigured returns true if the API key is set.
func (b *BraveSearch) IsConfigured() bool {
	return b.apiKey != ""
}

// BraveAPIResponse represents the Brave Search API response.
type BraveAPIResponse struct {
	Query struct {
		Original string `json:"original"`
	} `json:"query"`
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Age         string `json:"age,omitempty"`
		} `json:"results"`
	} `json:"web"`
}

// Search performs a search using the Brave Search API.
func (b *BraveSearch) Search(ctx context.Context, query string, maxResults int) (*SearchResults, error) {
	if b.apiKey == "" {
		return nil, fmt.Errorf("Brave Search API key not configured. Set BRAVE_SEARCH_API_KEY environment variable")
	}

	if maxResults <= 0 {
		maxResults = 10
	}
	if maxResults > 20 {
		maxResults = 20 // Brave API max is 20
	}

	fmt.Printf("🔍 [Brave] Iniciando busca para: %q\n", query)
	startTime := time.Now()

	// Build request URL
	apiURL := fmt.Sprintf(
		"https://api.search.brave.com/res/v1/web/search?q=%s&count=%d",
		url.QueryEscape(query),
		maxResults,
	)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.apiKey)

	// Execute request
	resp, err := b.httpClient.Do(req)
	if err != nil {
		fmt.Printf("❌ [Brave] Erro na requisição: %v\n", err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ [Brave] API retornou status %d: %s\n", resp.StatusCode, string(body))
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp BraveAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to SearchResults
	results := make([]SearchResult, 0, len(apiResp.Web.Results))
	for i, r := range apiResp.Web.Results {
		if i >= maxResults {
			break
		}
		results = append(results, SearchResult{
			Title:    r.Title,
			URL:      r.URL,
			Snippet:  r.Description,
			Position: i + 1,
		})
	}

	fmt.Printf("✅ [Brave] %d resultados em %v\n", len(results), time.Since(startTime))

	return &SearchResults{
		Query:        query,
		Results:      results,
		TotalResults: len(results),
	}, nil
}

// =============================================================================
// DUCKDUCKGO SEARCH (FALLBACK)
// =============================================================================

// SearchEngine defines the interface for web search providers.
type SearchEngine interface {
	Search(ctx context.Context, query string, maxResults int) (*SearchResults, error)
}

// DuckDuckGoSearch implements web search using DuckDuckGo HTML.
type DuckDuckGoSearch struct {
	pool      *BrowserPool
	region    string // e.g., "br-pt", "us-en"
	extractor *ContentExtractor
}

// NewDuckDuckGoSearch creates a new DuckDuckGo search engine.
func NewDuckDuckGoSearch(pool *BrowserPool) *DuckDuckGoSearch {
	return &DuckDuckGoSearch{
		pool:      pool,
		region:    "br-pt",
		extractor: NewContentExtractor(),
	}
}

// SetRegion sets the search region.
func (d *DuckDuckGoSearch) SetRegion(region string) {
	d.region = region
}

// Search performs a search on DuckDuckGo and returns results.
// Uses interactive navigation for more reliable results.
func (d *DuckDuckGoSearch) Search(_ context.Context, query string, maxResults int) (*SearchResults, error) {
	if maxResults <= 0 {
		maxResults = 10
	}

	fmt.Printf("🔍 [DuckDuckGo] Iniciando busca para: %q\n", query)

	startTime := time.Now()

	// Get browser context (this may start the browser if not running)
	browserCtx, err := d.pool.GetContext()
	if err != nil {
		fmt.Printf("❌ [DuckDuckGo] Erro ao obter contexto: %v\n", err)
		return nil, fmt.Errorf("failed to get browser context: %w", err)
	}

	// Mark operation as active to prevent idle timer from closing browser
	d.pool.BeginOperation()
	defer d.pool.EndOperation()

	fmt.Printf("✅ [DuckDuckGo] Contexto obtido em %v\n", time.Since(startTime))

	// Check if context is already canceled
	select {
	case <-browserCtx.Done():
		fmt.Printf("❌ [DuckDuckGo] Contexto já está cancelado: %v\n", browserCtx.Err())
		return nil, fmt.Errorf("browser context already canceled: %w", browserCtx.Err())
	default:
	}

	// Create timeout context
	opCtx, cancel := context.WithTimeout(browserCtx, 90*time.Second)
	defer cancel()

	// Navigate to DuckDuckGo and perform interactive search
	fmt.Printf("🔍 [DuckDuckGo] Navegando para duckduckgo.com...\n")

	var html string
	err = chromedp.Run(opCtx,
		// Go to DuckDuckGo homepage
		chromedp.Navigate("https://duckduckgo.com/"),
		chromedp.WaitVisible(`input[name="q"]`, chromedp.ByQuery),

		// Type search query and submit
		chromedp.SendKeys(`input[name="q"]`, query+"\n", chromedp.ByQuery),

		// Wait for results to load
		chromedp.Sleep(3*time.Second),

		// Get the HTML
		chromedp.OuterHTML("html", &html),
	)

	if err != nil {
		fmt.Printf("❌ [DuckDuckGo] Erro na busca interativa: %v\n", err)
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	fmt.Printf("✅ [DuckDuckGo] HTML obtido em %v (%d bytes)\n", time.Since(startTime), len(html))

	// Parse results
	results, err := d.parseResults(html, maxResults)
	if err != nil {
		return nil, fmt.Errorf("failed to parse results: %w", err)
	}

	fmt.Printf("✅ [DuckDuckGo] Parseados %d resultados em %v\n", len(results), time.Since(startTime))

	return &SearchResults{
		Query:        query,
		Results:      results,
		TotalResults: len(results),
	}, nil
}

// parseResults parses DuckDuckGo HTML search results.
func (d *DuckDuckGoSearch) parseResults(html string, maxResults int) ([]SearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var results []SearchResult

	fmt.Printf("🔍 [Parser] Analisando HTML (%d bytes)\n", len(html))

	// Modern DuckDuckGo uses article elements or li elements for results
	// Try to find result containers

	// Method 1: Look for article elements with data-testid
	doc.Find("article[data-testid='result']").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		// Find link and title
		link := s.Find("a[data-testid='result-title-a']")
		if link.Length() == 0 {
			link = s.Find("h2 a")
		}
		if link.Length() == 0 {
			link = s.Find("a").First()
		}

		href, _ := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		resultURL := d.extractRealURL(href)

		// Find snippet
		snippet := ""
		snippetEl := s.Find("[data-testid='result-snippet']")
		if snippetEl.Length() == 0 {
			snippetEl = s.Find(".result__snippet")
		}
		if snippetEl.Length() > 0 {
			snippet = strings.TrimSpace(snippetEl.Text())
		}

		if resultURL != "" && !strings.Contains(resultURL, "duckduckgo.com") {
			results = append(results, SearchResult{
				Title:    title,
				URL:      resultURL,
				Snippet:  snippet,
				Position: len(results) + 1,
			})
		}
	})

	if len(results) > 0 {
		fmt.Printf("✅ [Parser] Método 1 (article) encontrou %d resultados\n", len(results))
		return results, nil
	}

	// Method 2: Look for li elements with data-layout="organic"
	doc.Find("li[data-layout='organic']").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		link := s.Find("a").First()
		href, _ := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		resultURL := d.extractRealURL(href)

		snippet := ""
		s.Find("span").Each(func(j int, span *goquery.Selection) {
			text := strings.TrimSpace(span.Text())
			if len(text) > len(snippet) && len(text) > 50 {
				snippet = text
			}
		})

		if resultURL != "" && !strings.Contains(resultURL, "duckduckgo.com") {
			results = append(results, SearchResult{
				Title:    title,
				URL:      resultURL,
				Snippet:  snippet,
				Position: len(results) + 1,
			})
		}
	})

	if len(results) > 0 {
		fmt.Printf("✅ [Parser] Método 2 (li organic) encontrou %d resultados\n", len(results))
		return results, nil
	}

	// Method 3: Look for divs with result class
	doc.Find("div.result, div.web-result").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		link := s.Find("a.result__a, a.result__url, h2 a").First()
		href, _ := link.Attr("href")
		title := strings.TrimSpace(link.Text())
		resultURL := d.extractRealURL(href)

		snippet := strings.TrimSpace(s.Find(".result__snippet").Text())

		if resultURL != "" && !strings.Contains(resultURL, "duckduckgo.com") {
			results = append(results, SearchResult{
				Title:    title,
				URL:      resultURL,
				Snippet:  snippet,
				Position: len(results) + 1,
			})
		}
	})

	if len(results) > 0 {
		fmt.Printf("✅ [Parser] Método 3 (div.result) encontrou %d resultados\n", len(results))
		return results, nil
	}

	// Method 4: Generic fallback - find all external links
	fmt.Printf("⚠️ [Parser] Tentando fallback genérico...\n")

	seenURLs := make(map[string]bool)
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		href, exists := s.Attr("href")
		if !exists {
			return
		}

		resultURL := d.extractRealURL(href)

		// Skip invalid or internal links
		if resultURL == "" ||
			!strings.HasPrefix(resultURL, "http") ||
			strings.Contains(resultURL, "duckduckgo.com") ||
			strings.Contains(resultURL, "duck.co") ||
			strings.Contains(resultURL, "javascript:") {
			return
		}

		// Skip duplicates
		if seenURLs[resultURL] {
			return
		}

		title := strings.TrimSpace(s.Text())
		if title == "" || len(title) < 5 || len(title) > 200 {
			return
		}

		// Skip navigation/UI links
		lowerTitle := strings.ToLower(title)
		if strings.Contains(lowerTitle, "more results") ||
			strings.Contains(lowerTitle, "next page") ||
			strings.Contains(lowerTitle, "settings") {
			return
		}

		seenURLs[resultURL] = true
		results = append(results, SearchResult{
			Title:    title,
			URL:      resultURL,
			Position: len(results) + 1,
		})
	})

	if len(results) > 0 {
		fmt.Printf("✅ [Parser] Fallback encontrou %d resultados\n", len(results))
	} else {
		// Debug: Show page title to understand what page we're on
		pageTitle := doc.Find("title").Text()
		fmt.Printf("⚠️ [Parser] Nenhum resultado. Página: %q\n", strings.TrimSpace(pageTitle))

		// Show some structure info
		fmt.Printf("⚠️ [Parser] Estrutura: articles=%d, divs=%d, links=%d\n",
			doc.Find("article").Length(),
			doc.Find("div").Length(),
			doc.Find("a").Length())
	}

	return results, nil
}

// extractRealURL extracts the actual URL from a DuckDuckGo redirect URL.
func (d *DuckDuckGoSearch) extractRealURL(href string) string {
	// DuckDuckGo HTML version typically has direct URLs
	// but they might be URL-encoded or have tracking params

	// If it's already a direct URL
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}

	// If it's a redirect URL (//duckduckgo.com/l/?uddg=...)
	if strings.Contains(href, "uddg=") {
		parsed, err := url.Parse(href)
		if err != nil {
			return ""
		}

		uddg := parsed.Query().Get("uddg")
		if uddg != "" {
			decoded, err := url.QueryUnescape(uddg)
			if err != nil {
				return uddg
			}
			return decoded
		}
	}

	// Try to decode as URL-encoded
	decoded, err := url.QueryUnescape(href)
	if err != nil {
		return href
	}

	return decoded
}

// WebSearcher provides a unified interface for web searching.
type WebSearcher struct {
	primary  SearchEngine
	fallback SearchEngine
}

// NewWebSearcher creates a new web searcher with DuckDuckGo only.
func NewWebSearcher(pool *BrowserPool) *WebSearcher {
	return NewWebSearcherWithConfig(pool, "")
}

// NewWebSearcherWithConfig creates a new web searcher with optional Brave API key.
// Uses Brave Search API if configured, falls back to DuckDuckGo otherwise.
func NewWebSearcherWithConfig(pool *BrowserPool, braveAPIKey string) *WebSearcher {
	duck := NewDuckDuckGoSearch(pool)

	if braveAPIKey != "" {
		brave := NewBraveSearch(braveAPIKey)
		fmt.Printf("🔍 [WebSearcher] Usando Brave Search API como motor principal\n")
		return &WebSearcher{
			primary:  brave,
			fallback: duck,
		}
	}

	fmt.Printf("⚠️ [WebSearcher] Brave API key não configurada, usando DuckDuckGo\n")
	return &WebSearcher{
		primary:  duck,
		fallback: nil,
	}
}

// SetEngine sets the primary search engine to use.
func (w *WebSearcher) SetEngine(engine SearchEngine) {
	w.primary = engine
}

// Search performs a web search using the primary engine with fallback.
func (w *WebSearcher) Search(ctx context.Context, query string, maxResults int) (*SearchResults, error) {
	results, err := w.primary.Search(ctx, query, maxResults)
	if err != nil && w.fallback != nil {
		fmt.Printf("⚠️ [WebSearcher] Motor principal falhou, tentando fallback: %v\n", err)
		return w.fallback.Search(ctx, query, maxResults)
	}
	return results, err
}
