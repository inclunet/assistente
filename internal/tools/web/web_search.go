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
	// Search executa a busca a partir de offset (0-based) e retorna até maxResults
	// resultados. Usa cliente HTTP centralizado com auth/retry automático.
	Search(ctx context.Context, client *httpclient.Client, query string, offset, maxResults int) ([]SearchResult, error)
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

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *WebSearch) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "web", Class: "web_lookup", Package: "web", Risk: "network"}
}

func (t *WebSearch) Description() string {
	return `Searches the web and returns ranked links with titles and snippets. Use when you need to discover sources or do not yet know the target URL; for example {"query":"Go context cancellation documentation","max_results":5}. Do not use to read a known page (use web_fetch), call an API with HTTP controls (use http_request), or parse a known RSS/Atom/JSON feed (use feed_read). Returns paginated JSON with query, provider, offset, count, has_more, and results; while has_more is true, request the next page with offset = previous offset + count. Risk: sends the query to an external search provider and requires network access. If unavailable, discover and load it with tool_catalog when the profile permits on-demand tools.`
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
				"description": "Número máximo de resultados por página. Padrão: 8 (máx: 20).",
				"minimum": 1,
				"maximum": 20,
				"default": 8
			},
			"offset": {
				"type": "integer",
				"description": "Deslocamento 0-based para paginação. Para a próxima página, use offset = offset anterior + count. Padrão: 0.",
				"minimum": 0,
				"default": 0
			}
		},
		"required": ["query"],
		"additionalProperties": false
	}`)
}

type webSearchArgs struct {
	Query      string `json:"query"`
	MaxResults *int   `json:"max_results,omitempty"`
	Offset     *int   `json:"offset,omitempty"`
}

// webSearchJSONOutput é o contrato JSON canônico da tool: objeto estável consumível
// tanto por LLMs quanto por json.Unmarshal (jobs/output maps).
type webSearchJSONOutput struct {
	Query    string `json:"query"`
	Provider string `json:"provider"`
	Offset   int    `json:"offset"`
	Count    int    `json:"count"`
	// HasMore é heurístico (página veio "cheia"): por ser scraping, não há total de
	// resultados disponível. Use offset+count para buscar a próxima página.
	HasMore bool           `json:"has_more"`
	Results []SearchResult `json:"results"`
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

	offset := 0
	if a.Offset != nil {
		if *a.Offset < 0 {
			return tools.ToolResult{
				Content: fmt.Sprintf("Parâmetro 'offset' inválido: %d (deve ser >= 0)", *a.Offset),
				IsError: true,
			}, nil
		}
		offset = *a.Offset
	}

	results, err := t.provider.Search(ctx, t.client, a.Query, offset, maxResults)
	if err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro na busca (%s): %v", t.provider.Name(), err),
			IsError: true,
		}, nil
	}

	// JSON canônico: serve tanto LLMs quanto consumo programático (jobs). Mesmo sem
	// resultados devolve estrutura válida (results: []), evitando que o chamador
	// precise tratar texto.
	if results == nil {
		results = []SearchResult{}
	}
	// Heurística de paginação: a página veio "cheia" => provavelmente há mais.
	hasMore := len(results) >= maxResults
	out := webSearchJSONOutput{
		Query:    a.Query,
		Provider: t.provider.Name(),
		Offset:   offset,
		Count:    len(results),
		HasMore:  hasMore,
		Results:  results,
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao serializar resultados: %v", err),
			IsError: true,
		}, nil
	}

	// Structured=true: o executor não trunca este JSON canônico (truncar o
	// corromperia); se exceder o limite efetivo, falha de forma explícita.
	return tools.ToolResult{
		Content:    string(encoded),
		Structured: true,
		Metadata: map[string]any{
			"results":  len(results),
			"provider": t.provider.Name(),
			"query":    a.Query,
			"offset":   offset,
			"has_more": hasMore,
		},
	}, nil
}

// ==================== DuckDuckGo Provider ====================

// duckDuckGoProvider usa a API HTML do DuckDuckGo (sem necessidade de API key).
type duckDuckGoProvider struct{}

func (p *duckDuckGoProvider) Name() string { return "DuckDuckGo" }

func (p *duckDuckGoProvider) Search(ctx context.Context, client *httpclient.Client, query string, offset, maxResults int) ([]SearchResult, error) {
	// Usa a DuckDuckGo HTML lite como fallback universal (sem API key). O parâmetro
	// "s" desloca o início dos resultados (paginação). Não há total de resultados
	// exposto, por isso o has_more é heurístico na camada da tool.
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
	if offset > 0 {
		searchURL += fmt.Sprintf("&s=%d", offset)
	}

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
	defer func() { _ = resp.Body.Close() }()

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
