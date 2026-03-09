package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/tools"
	httpclient "assistente/internal/tools/http"
)

// WebSearch realiza buscas na web usando uma API de busca.
// Suporta múltiplos provedores via configuração (padrão: DuckDuckGo HTML, sem API key).
// Usa cliente HTTP centralizado com auth/retry automático.
type WebSearch struct {
	client   *httpclient.Client
	provider SearchProvider
}

// SearchProvider define a interface para provedores de busca.
type SearchProvider interface {
	// Search executa a busca e retorna resultados formatados.
	// Usa cliente HTTP centralizado com auth/retry automático.
	Search(ctx context.Context, client *httpclient.Client, query string, maxResults int) ([]SearchResult, error)
	// Name retorna o nome do provedor para logs.
	Name() string
}

// SearchResult representa um resultado individual de busca.
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// NewWebSearch cria uma nova instância de WebSearch com o provedor DuckDuckGo padrão.
func NewWebSearch(credMgr *credentials.Manager) *WebSearch {
	if credMgr == nil {
		credMgr = credentials.NewManager(nil)
	}
	client := httpclient.New(&httpclient.Config{
		CredentialManager: credMgr,
	}, map[string]string{})
	return &WebSearch{
		client:   client,
		provider: &duckDuckGoProvider{},
	}
}

// NewWebSearchWithProvider cria WebSearch com um provedor customizado (ex: Google, Bing).
func NewWebSearchWithProvider(credMgr *credentials.Manager, provider SearchProvider) *WebSearch {
	if credMgr == nil {
		credMgr = credentials.NewManager(nil)
	}
	client := httpclient.New(&httpclient.Config{
		CredentialManager: credMgr,
	}, map[string]string{})
	return &WebSearch{
		client:   client,
		provider: provider,
	}
}

func (t *WebSearch) Name() string { return "web_search" }

func (t *WebSearch) Description() string {
	return "Searches the web and returns titles, URLs, and snippets. Use to discover relevant links; to read content, call web_fetch on a chosen URL."
}

func (t *WebSearch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "Termos de busca. Seja específico para melhores resultados."
			},
			"max_results": {
				"type": "integer",
				"description": "Número máximo de resultados. Padrão: 8."
			}
		},
		"required": ["query"],
		"additionalProperties": false
	}`)
}

type webSearchArgs struct {
	Query      string `json:"query"`
	MaxResults *int   `json:"max_results,omitempty"`
}

const searchDefaultMaxResults = 8

func (t *WebSearch) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a webSearchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Query == "" {
		return tools.ToolResult{Content: "Parâmetro 'query' é obrigatório", IsError: true}, nil
	}

	maxResults := searchDefaultMaxResults
	if a.MaxResults != nil && *a.MaxResults > 0 {
		maxResults = *a.MaxResults
		if maxResults > 20 {
			maxResults = 20
		}
	}

	results, err := t.provider.Search(ctx, t.client, a.Query, maxResults)
	if err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro na busca (%s): %v", t.provider.Name(), err),
			IsError: true,
		}, nil
	}

	if len(results) == 0 {
		return tools.ToolResult{
			Content:  fmt.Sprintf("Nenhum resultado encontrado para '%s'", a.Query),
			Metadata: map[string]any{"results": 0, "provider": t.provider.Name()},
		}, nil
	}

	// Formata resultados
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Busca: '%s' (%d resultados via %s)\n\n", a.Query, len(results), t.provider.Name()))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		sb.WriteString(fmt.Sprintf("   %s\n", r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", r.Snippet))
		}
		sb.WriteString("\n")
	}

	return tools.ToolResult{
		Content: sb.String(),
		Metadata: map[string]any{
			"results":  len(results),
			"provider": t.provider.Name(),
			"query":    a.Query,
		},
	}, nil
}

// ==================== DuckDuckGo Provider ====================

// duckDuckGoProvider usa a API HTML do DuckDuckGo (sem necessidade de API key).
type duckDuckGoProvider struct{}

func (p *duckDuckGoProvider) Name() string { return "DuckDuckGo" }

func (p *duckDuckGoProvider) Search(ctx context.Context, client *httpclient.Client, query string, maxResults int) ([]SearchResult, error) {
	// Usa a DuckDuckGo HTML lite como fallback universal (sem API key)
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")

	resp, err := client.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	return parseDuckDuckGoHTML(string(body), maxResults), nil
}

// parseDuckDuckGoHTML extrai resultados da página HTML lite do DuckDuckGo.
// Estrutura: <a class="result__a" href="...">título</a> + <a class="result__snippet">snippet</a>
func parseDuckDuckGoHTML(body string, maxResults int) []SearchResult {
	var results []SearchResult

	// Busca por blocos de resultado
	remaining := body
	for len(results) < maxResults {
		// Encontra link do resultado
		linkStart := strings.Index(remaining, `class="result__a"`)
		if linkStart == -1 {
			break
		}

		// Extrai href
		hrefStart := strings.LastIndex(remaining[:linkStart], `href="`)
		if hrefStart == -1 {
			remaining = remaining[linkStart+17:]
			continue
		}
		hrefStart += 6
		hrefEnd := strings.Index(remaining[hrefStart:], `"`)
		if hrefEnd == -1 {
			remaining = remaining[linkStart+17:]
			continue
		}
		rawURL := remaining[hrefStart : hrefStart+hrefEnd]

		// DuckDuckGo HTML usa redirect URLs, extrai a URL real
		resultURL := extractDDGURL(rawURL)

		// Extrai título (texto dentro do <a>)
		titleStart := strings.Index(remaining[linkStart:], ">")
		if titleStart == -1 {
			remaining = remaining[linkStart+17:]
			continue
		}
		titleStart += linkStart + 1
		titleEnd := strings.Index(remaining[titleStart:], "</a>")
		if titleEnd == -1 {
			remaining = remaining[linkStart+17:]
			continue
		}
		title := stripTagsSimple(remaining[titleStart : titleStart+titleEnd])
		title = strings.TrimSpace(title)

		// Extrai snippet
		snippet := ""
		snippetStart := strings.Index(remaining[linkStart:], `class="result__snippet"`)
		if snippetStart != -1 {
			snippetStart += linkStart
			snipContentStart := strings.Index(remaining[snippetStart:], ">")
			if snipContentStart != -1 {
				snipContentStart += snippetStart + 1
				snipContentEnd := strings.Index(remaining[snipContentStart:], "</a>")
				if snipContentEnd == -1 {
					snipContentEnd = strings.Index(remaining[snipContentStart:], "</span>")
				}
				if snipContentEnd != -1 {
					snippet = stripTagsSimple(remaining[snipContentStart : snipContentStart+snipContentEnd])
					snippet = strings.TrimSpace(snippet)
				}
			}
		}

		if title != "" && resultURL != "" {
			results = append(results, SearchResult{
				Title:   title,
				URL:     resultURL,
				Snippet: snippet,
			})
		}

		remaining = remaining[linkStart+17:]
	}

	return results
}

// extractDDGURL extrai a URL real de um redirect URL do DuckDuckGo.
func extractDDGURL(rawURL string) string {
	// DuckDuckGo HTML usa: //duckduckgo.com/l/?uddg=ENCODED_URL&rut=...
	if strings.Contains(rawURL, "uddg=") {
		parts := strings.SplitN(rawURL, "uddg=", 2)
		if len(parts) == 2 {
			encodedURL := parts[1]
			// Remove parâmetros extras após &
			if idx := strings.Index(encodedURL, "&"); idx != -1 {
				encodedURL = encodedURL[:idx]
			}
			decoded, err := url.QueryUnescape(encodedURL)
			if err == nil {
				return decoded
			}
		}
	}

	// Se não é redirect, usa como está
	if strings.HasPrefix(rawURL, "http") {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}

	return rawURL
}
