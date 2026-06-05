package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/tools"
	httpclient "assistente/internal/tools/http"
)

// FeedRead busca um feed (RSS/Atom/JSON Feed/podcast) e o converte para um JSON
// canônico. Usa o cliente HTTP centralizado, então a autenticação por domínio
// (bearer/basic/custom/oauth2) é aplicada automaticamente via credmanager — o
// modelo nunca precisa lidar com tokens.
type FeedRead struct {
	client            *httpclient.Client
	allowPrivateHosts bool // habilitado apenas em testes
}

// NewFeedRead cria a tool com o cliente HTTP centralizado.
func NewFeedRead(credMgr *credentials.Manager) *FeedRead {
	if credMgr == nil {
		credMgr = credentials.NewManager(nil)
	}
	client := httpclient.New(&httpclient.Config{
		CredentialManager: credMgr,
	}, map[string]string{})
	return &FeedRead{client: client}
}

func (t *FeedRead) Name() string { return "feed_read" }

func (t *FeedRead) Description() string {
	return "Fetches a feed URL (RSS, Atom, JSON Feed, or podcast) and returns it as canonical JSON: feed metadata plus a normalized list of items (title, link, dates in RFC3339, summary, enclosures). Podcast feeds include iTunes metadata (duration, episode/season, audio enclosures). Authentication is applied automatically per domain when a credential is registered. Use this instead of web_fetch when the URL is a feed and you want structured items to process with other tools/LLM."
}

func (t *FeedRead) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "Feed URL (must start with http:// or https://). Accepts RSS, Atom, JSON Feed and podcast feeds."
			},
			"max_items": {
				"type": "integer",
				"description": "Maximum number of items to return (default 20). Use a small number to keep the payload focused."
			},
			"include_content": {
				"type": "boolean",
				"description": "If true, includes the full item body (content) besides the summary. Default false."
			},
			"strip_html": {
				"type": "boolean",
				"description": "If true (default), converts HTML in summary/content to plain text."
			},
			"since": {
				"type": "string",
				"description": "RFC3339 timestamp; only items published after this instant are returned (items without a parseable date are kept)."
			}
		},
		"required": ["url"],
		"additionalProperties": false
	}`)
}

type feedReadArgs struct {
	URL            string `json:"url"`
	MaxItems       *int   `json:"max_items,omitempty"`
	IncludeContent *bool  `json:"include_content,omitempty"`
	StripHTML      *bool  `json:"strip_html,omitempty"`
	Since          string `json:"since,omitempty"`
}

const (
	feedDefaultMaxItems  = 20
	feedMaxResponseBody  = 10 * 1024 * 1024 // 10MB
	feedDefaultUserAgent = "Assistente/1.0 (Tool FeedRead; +https://github.com)"
)

func (t *FeedRead) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a feedReadArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	a.URL = strings.TrimSpace(a.URL)
	if a.URL == "" {
		return tools.ToolResult{Content: "Parâmetro 'url' é obrigatório", IsError: true}, nil
	}

	parsedURL, err := url.Parse(a.URL)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("URL inválida: %v", err), IsError: true}, nil
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return tools.ToolResult{Content: "URL deve usar http:// ou https://", IsError: true}, nil
	}
	if !t.allowPrivateHosts && isPrivateHost(parsedURL.Hostname()) {
		return tools.ToolResult{Content: "Acesso a hosts locais/privados não é permitido", IsError: true}, nil
	}

	opts := parseOptions{
		MaxItems:       feedDefaultMaxItems,
		IncludeContent: false,
		StripHTML:      true,
	}
	if a.MaxItems != nil && *a.MaxItems > 0 {
		opts.MaxItems = *a.MaxItems
	}
	if a.IncludeContent != nil {
		opts.IncludeContent = *a.IncludeContent
	}
	if a.StripHTML != nil {
		opts.StripHTML = *a.StripHTML
	}
	if s := strings.TrimSpace(a.Since); s != "" {
		ts, perr := time.Parse(time.RFC3339, s)
		if perr != nil {
			return tools.ToolResult{Content: fmt.Sprintf("'since' deve ser RFC3339: %v", perr), IsError: true}, nil
		}
		opts.Since = &ts
	}

	req, err := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao criar requisição: %v", err), IsError: true}, nil
	}
	req.Header.Set("User-Agent", feedDefaultUserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/feed+json, application/json, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")

	resp, err := t.client.Do(ctx, req)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar feed: %v", err), IsError: true}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return tools.ToolResult{
			Content: fmt.Sprintf("HTTP %s para %s", resp.Status, a.URL),
			IsError: true,
		}, nil
	}

	limited := io.LimitReader(resp.Body, feedMaxResponseBody)
	canonical, err := parseFeed(limited, opts)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao parsear feed: %v", err), IsError: true}, nil
	}

	// Marshal compacto (sem indentação): o contrato da tool é JSON canônico
	// consumível por json.Unmarshal, não por leitura humana, e reduz bytes.
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao serializar feed: %v", err), IsError: true}, nil
	}

	// O executor trunca resultados acima de tools.DefaultMaxResultSize e anexa um
	// aviso textual, o que transformaria a saída em JSON inválido e quebraria o
	// contrato de "JSON canônico". Em vez de devolver JSON corrompido, falhamos
	// de forma explícita pedindo um payload menor.
	if len(encoded) > tools.DefaultMaxResultSize {
		return tools.ToolResult{
			Content: fmt.Sprintf(
				"Feed serializado tem %d bytes, acima do limite de %d. Reduza 'max_items' ou desabilite 'include_content' para obter um payload menor.",
				len(encoded), tools.DefaultMaxResultSize,
			),
			IsError: true,
		}, nil
	}

	return tools.ToolResult{
		Content: string(encoded),
		Metadata: map[string]any{
			"url":        a.URL,
			"feed_type":  canonical.FeedType,
			"item_count": canonical.ItemCount,
			"is_podcast": canonical.IsPodcast,
			"status":     resp.StatusCode,
		},
	}, nil
}

// isPrivateHost bloqueia hosts locais/privados (defesa básica contra SSRF).
// Usa net.ParseIP para cobrir corretamente os ranges privados (10/8,
// 172.16/12, 192.168/16), loopback e link-local, sem o falso positivo de um
// prefix match ingênuo (ex.: 172.2.x.x é público).
//
// Limitação conhecida: só inspeciona IPs literais e "localhost". Um hostname
// que resolve via DNS para um IP privado (ex.: "internal.example") NÃO é
// bloqueado, pois net.ParseIP retorna nil para nomes — e não fazemos resolução
// de DNS aqui (evita lookups extras e o TOCTOU entre o check e o dial). Portanto
// isto não é proteção anti-SSRF completa, apenas uma barreira contra URLs que
// já apontam diretamente para endereços locais/privados.
func isPrivateHost(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}
