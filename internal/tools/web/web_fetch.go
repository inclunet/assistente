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

	"golang.org/x/net/html"
)

// WebFetch busca o conteúdo de uma URL e extrai texto legível.
// Remove HTML, scripts, estilos e retorna conteúdo limpo em texto/markdown.
// Usa cliente HTTP centralizado com suporte a autenticação automática.
type WebFetch struct {
	client            *httpclient.Client // Cliente HTTP centralizado com auth/retry
	allowPrivateHosts bool               // Para testes com httptest (padrão: false)
}

// NewWebFetch cria uma nova instância de WebFetch.
func NewWebFetch(credMgr *credentials.Manager) *WebFetch {
	if credMgr == nil {
		credMgr = credentials.NewManager(nil) // Cria manager vazio se não fornecido
	}
	// Usar cliente HTTP centralizado com retry policy
	client := httpclient.New(&httpclient.Config{
		CredentialManager: credMgr,
	}, map[string]string{})
	t := &WebFetch{
		client: client,
	}
	// net/http segue redirects automaticamente; sem isto uma URL pública poderia
	// redirecionar para um host privado (ex.: 127.0.0.1, 169.254.169.254) e burlar
	// o bloqueio anti-SSRF. Aplica o guard compartilhado no client desta tool.
	if bc := client.GetBaseClient(); bc != nil {
		bc.CheckRedirect = httpclient.RedirectGuard(httpclient.DefaultMaxRedirects, func() bool { return t.allowPrivateHosts })
		// Barreira anti-SSRF definitiva: valida o IP REAL pós-resolução de DNS no
		// DialContext, cobrindo DNS rebinding, formas numéricas não-padrão e os
		// redirects (que reusam este transport).
		httpclient.SetTransportGuard(bc, func() bool { return t.allowPrivateHosts })
	}
	return t
}

// SetNetworkAuthorizer instala o authorizer anti-SSRF (consentimento + allowlist)
// no cliente HTTP desta tool.
func (t *WebFetch) SetNetworkAuthorizer(a httpclient.NetworkAuthorizer) {
	t.client.SetNetworkAuthorizer(a)
}

func (t *WebFetch) Name() string { return "web_fetch" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *WebFetch) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "web", Class: "web_lookup", Package: "web", Risk: "network"}
}

func (t *WebFetch) Description() string {
	return "Fetches a URL (http/https) and extracts readable content. Strips HTML/scripts/styles and returns text (or raw/markdown). Blocks local/private hosts. Use after finding a specific link."
}

func (t *WebFetch) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "URL completa para buscar (deve começar com http:// ou https://)"
			},
			"max_length": {
				"type": "integer",
				"description": "Tamanho máximo do conteúdo retornado em caracteres. Padrão: 50000."
			},
			"extract_mode": {
				"type": "string",
				"enum": ["text", "raw", "markdown"],
				"description": "Modo de extração: 'text' (padrão) extrai texto limpo, 'raw' retorna HTML bruto, 'markdown' tenta converter para markdown básico."
			}
		},
		"required": ["url"],
		"additionalProperties": false
	}`)
}

type webFetchArgs struct {
	URL         string `json:"url"`
	MaxLength   *int   `json:"max_length,omitempty"`
	ExtractMode string `json:"extract_mode,omitempty"`
}

// Limites de segurança
const (
	fetchDefaultMaxLength = 50000
	fetchMaxResponseBody  = 10 * 1024 * 1024 // 10MB max download
)

func (t *WebFetch) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a webFetchArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.URL == "" {
		return tools.ToolResult{Content: "Parâmetro 'url' é obrigatório", IsError: true}, nil
	}

	// Valida URL
	parsedURL, err := url.Parse(a.URL)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("URL inválida: %v", err), IsError: true}, nil
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return tools.ToolResult{Content: "URL deve usar http:// ou https://", IsError: true}, nil
	}

	// Hosts locais/privados/CGNAT/etc. são barrados pela política anti-SSRF na
	// barreira pós-DNS do cliente centralizado (client.Do). Com authorizer
	// configurado, abre o fluxo de consentimento/allowlist e reexecuta; sem
	// authorizer, devolve um erro acionável. Não barramos aqui de forma seca.

	maxLength := fetchDefaultMaxLength
	if a.MaxLength != nil && *a.MaxLength > 0 {
		maxLength = *a.MaxLength
	}

	mode := "text"
	if a.ExtractMode != "" {
		mode = a.ExtractMode
	}

	// Faz a requisição
	req, err := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao criar requisição: %v", err), IsError: true}, nil
	}

	// User-Agent amigável
	req.Header.Set("User-Agent", "Assistente/1.0 (Tool WebFetch; +https://github.com)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.7")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")

	// Usa cliente HTTP centralizado (com auth/retry automático)
	resp, err := t.client.Do(ctx, req)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar URL: %v", err), IsError: true}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	// Verifica status
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return tools.ToolResult{
			Content: fmt.Sprintf("HTTP %d %s para %s", resp.StatusCode, resp.Status, a.URL),
			IsError: true,
		}, nil
	}

	// Lê o body com limite
	limitedReader := io.LimitReader(resp.Body, fetchMaxResponseBody)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler resposta: %v", err), IsError: true}, nil
	}

	contentType := resp.Header.Get("Content-Type")
	content := string(body)

	// Extrai conteúdo baseado no modo e content-type
	var extracted string
	switch {
	case mode == "raw":
		extracted = content
	case strings.Contains(contentType, "text/plain") || strings.Contains(contentType, "application/json"):
		// Texto puro ou JSON — retorna direto
		extracted = content
	case strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml"):
		if mode == "markdown" {
			extracted = htmlToMarkdown(content)
		} else {
			extracted = htmlToText(content)
		}
	default:
		// Tenta extrair como HTML por padrão
		extracted = htmlToText(content)
	}

	// Trunca se necessário
	truncated := false
	if len(extracted) > maxLength {
		extracted = extracted[:maxLength]
		truncated = true
	}

	// Header informativo
	header := fmt.Sprintf("URL: %s\nStatus: %d | Content-Type: %s | Tamanho: %d chars\n",
		a.URL, resp.StatusCode, contentType, len(extracted))
	if truncated {
		header += fmt.Sprintf("(TRUNCADO: limite de %d caracteres)\n", maxLength)
	}
	header += "\n"

	return tools.ToolResult{
		Content: header + extracted,
		Metadata: map[string]any{
			"url":          a.URL,
			"status":       resp.StatusCode,
			"content_type": contentType,
			"length":       len(extracted),
			"truncated":    truncated,
		},
	}, nil
}

// ==================== HTML Processing ====================

// htmlToText extrai texto legível de HTML, removendo scripts, estilos e tags.
func htmlToText(rawHTML string) string {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		// Fallback: strip tags manualmente
		return stripTagsSimple(rawHTML)
	}

	var sb strings.Builder
	extractText(doc, &sb, false)

	// Limpa whitespace excessivo
	text := sb.String()
	text = collapseWhitespace(text)
	return strings.TrimSpace(text)
}

// htmlToMarkdown extrai conteúdo de HTML convertendo para markdown básico.
func htmlToMarkdown(rawHTML string) string {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return stripTagsSimple(rawHTML)
	}

	var sb strings.Builder
	extractMarkdown(doc, &sb)

	text := sb.String()
	text = collapseWhitespace(text)
	return strings.TrimSpace(text)
}

// extractText percorre o DOM e extrai texto, ignorando script/style/noscript.
func extractText(n *html.Node, sb *strings.Builder, inBlock bool) {
	if n.Type == html.ElementNode {
		tag := strings.ToLower(n.Data)
		// Ignora elementos não-textuais
		if tag == "script" || tag == "style" || tag == "noscript" || tag == "svg" || tag == "head" {
			return
		}
		// Elementos de bloco adicionam quebra de linha
		if isBlockElement(tag) {
			sb.WriteString("\n")
		}
	}

	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			sb.WriteString(text)
			sb.WriteString(" ")
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, sb, inBlock)
	}

	if n.Type == html.ElementNode {
		tag := strings.ToLower(n.Data)
		if isBlockElement(tag) {
			sb.WriteString("\n")
		}
	}
}

// extractMarkdown percorre o DOM e converte para markdown básico.
func extractMarkdown(n *html.Node, sb *strings.Builder) {
	if n.Type == html.ElementNode {
		tag := strings.ToLower(n.Data)
		if tag == "script" || tag == "style" || tag == "noscript" || tag == "svg" || tag == "head" {
			return
		}

		switch tag {
		case "h1":
			sb.WriteString("\n# ")
		case "h2":
			sb.WriteString("\n## ")
		case "h3":
			sb.WriteString("\n### ")
		case "h4":
			sb.WriteString("\n#### ")
		case "h5":
			sb.WriteString("\n##### ")
		case "h6":
			sb.WriteString("\n###### ")
		case "p":
			sb.WriteString("\n\n")
		case "br":
			sb.WriteString("\n")
		case "li":
			sb.WriteString("\n- ")
		case "strong", "b":
			sb.WriteString("**")
		case "em", "i":
			sb.WriteString("*")
		case "code":
			sb.WriteString("`")
		case "pre":
			sb.WriteString("\n```\n")
		case "a":
			// Será tratado no fechamento
		case "blockquote":
			sb.WriteString("\n> ")
		case "hr":
			sb.WriteString("\n---\n")
		default:
			if isBlockElement(tag) {
				sb.WriteString("\n")
			}
		}
	}

	if n.Type == html.TextNode {
		text := n.Data
		// Preserva whitespace em <pre>
		parent := n.Parent
		if parent != nil && parent.Type == html.ElementNode && strings.ToLower(parent.Data) == "pre" {
			sb.WriteString(text)
		} else {
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				sb.WriteString(trimmed)
				sb.WriteString(" ")
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractMarkdown(c, sb)
	}

	// Tags de fechamento
	if n.Type == html.ElementNode {
		tag := strings.ToLower(n.Data)
		switch tag {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			sb.WriteString("\n")
		case "strong", "b":
			sb.WriteString("**")
		case "em", "i":
			sb.WriteString("*")
		case "code":
			sb.WriteString("`")
		case "pre":
			sb.WriteString("\n```\n")
		case "a":
			href := getAttr(n, "href")
			if href != "" {
				// Fecha o link no formato markdown: [texto](url)
				// Como o texto já foi escrito, vamos apenas adicionar o URL
				_, _ = fmt.Fprintf(sb, " (%s) ", href)
			}
		case "p", "div", "section", "article":
			sb.WriteString("\n")
		}
	}
}

// ==================== Helpers ====================

// isBlockElement retorna true para elementos HTML de bloco.
func isBlockElement(tag string) bool {
	blocks := map[string]bool{
		"div": true, "p": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "ul": true, "ol": true,
		"li": true, "table": true, "tr": true, "td": true, "th": true,
		"blockquote": true, "pre": true, "hr": true, "br": true,
		"section": true, "article": true, "aside": true, "nav": true,
		"header": true, "footer": true, "main": true, "figure": true,
		"figcaption": true, "details": true, "summary": true,
	}
	return blocks[tag]
}

// getAttr retorna o valor de um atributo HTML.
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// stripTagsSimple remove tags HTML de forma simples (fallback).
func stripTagsSimple(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			sb.WriteRune(' ')
			continue
		}
		if !inTag {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// collapseWhitespace reduz múltiplas quebras de linha e espaços.
func collapseWhitespace(s string) string {
	// Reduz 3+ newlines para 2
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	// Reduz espaços múltiplos (mas não newlines)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Colapsa múltiplos espaços em um
		words := strings.Fields(line)
		lines[i] = strings.Join(words, " ")
	}
	return strings.Join(lines, "\n")
}
