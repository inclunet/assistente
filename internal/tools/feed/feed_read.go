package feed

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	t := &FeedRead{client: client}

	// O net/http segue redirects automaticamente, então só validar a URL inicial
	// não basta: uma URL pública poderia redirecionar para http://127.0.0.1/... e
	// burlar o bloqueio. Usa o guard anti-SSRF compartilhado. O cliente é exclusivo
	// desta tool, então é seguro configurar o CheckRedirect do baseClient aqui.
	if bc := client.GetBaseClient(); bc != nil {
		bc.CheckRedirect = httpclient.RedirectGuard(feedMaxRedirects, func() bool { return t.allowPrivateHosts })
	}

	return t
}

func (t *FeedRead) Name() string { return "feed_read" }

func (t *FeedRead) Description() string {
	return "Fetches a feed URL (RSS, Atom, JSON Feed, or podcast) and returns it as canonical JSON: feed metadata plus a normalized list of items (title, link, dates in RFC3339 when parseable - otherwise the raw feed string is kept, summary, enclosures). Podcast feeds include iTunes metadata (duration, episode/season, audio enclosures). Authentication is applied automatically per domain when a credential is registered. Use this instead of web_fetch when the URL is a feed and you want structured items to process with other tools/LLM."
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
	feedMaxRedirects     = 10
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
	if !t.allowPrivateHosts && httpclient.IsPrivateHost(parsedURL.Hostname()) {
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

	// Só 2xx é tratado como sucesso. Redirects bloqueados pelo CheckRedirect chegam
	// como erro de t.client.Do (caminho "Erro ao acessar feed" acima), não aqui —
	// pois o wrapper httpclient.Client não preserva a resposta quando Do retorna
	// erro. Este bloco cobre o caso de uma resposta 3xx que o net/http não seguiu
	// por conta própria (tipicamente um redirect sem header Location): o body seria
	// a página de redirect, não o feed, então devolvemos um erro explícito.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		dest := strings.TrimSpace(resp.Header.Get("Location"))
		msg := fmt.Sprintf("Redirect HTTP %s não seguido para %s", resp.Status, a.URL)
		if dest != "" {
			msg += fmt.Sprintf(" (destino: %s)", dest)
		}
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
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

	// O executor trunca resultados acima do seu limite efetivo e anexa um aviso
	// textual, o que transformaria a saída em JSON inválido e quebraria o contrato
	// de "JSON canônico". Lemos o limite efetivo do ctx (varia por chamador: jobs
	// usam budget maior que o default) e, em vez de devolver JSON corrompido,
	// falhamos de forma explícita pedindo um payload menor.
	maxResultSize := tools.MaxResultSizeFromContext(ctx)
	if len(encoded) > maxResultSize {
		return tools.ToolResult{
			Content: fmt.Sprintf(
				"Feed serializado tem %d bytes, acima do limite de %d. Reduza 'max_items' ou desabilite 'include_content' para obter um payload menor.",
				len(encoded), maxResultSize,
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
