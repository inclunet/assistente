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
	httpclient "assistente/internal/tools/http"
)

// braveSearchEndpoint é o endpoint fixo da Brave Search API (web search).
const braveSearchEndpoint = "https://api.search.brave.com/res/v1/web/search"

// braveCredentialHint orienta o cadastro da chave no credmanager.
const braveCredentialHint = "cadastre a chave da Brave Search API no credmanager para o domínio api.search.brave.com (tipo bearer com o token, ou custom com header X-Subscription-Token)"

// errNoBraveCredential sinaliza ausência de credencial Brave: a tool faz
// fallback silencioso para o DuckDuckGo em vez de falhar.
var errNoBraveCredential = fmt.Errorf("sem credencial Brave (%s)", braveCredentialHint)

// braveProvider consulta a Brave Search API oficial. A chave vem sempre do
// credmanager, resolvida por URL contra o endpoint fixo acima — nunca de
// variável de ambiente, flag ou argumento da tool.
type braveProvider struct {
	credMgr *credentials.Manager
	// endpointOverride permite apontar para um servidor fake em testes.
	endpointOverride string
}

func (p *braveProvider) Name() string { return "Brave" }

func (p *braveProvider) endpoint() string {
	if p.endpointOverride != "" {
		return p.endpointOverride
	}
	return braveSearchEndpoint
}

// braveToken extrai o token aceito no header X-Subscription-Token a partir do
// AuthConfig resolvido: bearer usa Token; custom usa o header
// X-Subscription-Token (case-insensitive); demais tipos não servem.
func braveToken(auth *credentials.AuthConfig) string {
	if auth == nil {
		return ""
	}
	if auth.Type == "bearer" {
		token := strings.TrimSpace(auth.Token)
		return strings.TrimPrefix(token, "Bearer ")
	}
	if auth.Type == "custom" {
		for key, val := range auth.Headers {
			if strings.EqualFold(key, "X-Subscription-Token") {
				return strings.TrimSpace(val)
			}
		}
	}
	return ""
}

func (p *braveProvider) Search(ctx context.Context, client *httpclient.Client, query string, offset, maxResults int) ([]SearchResult, error) {
	if p.credMgr == nil {
		return nil, errNoBraveCredential
	}
	auth, err := p.credMgr.ResolveForURLWithContext(ctx, p.endpoint())
	if err != nil {
		// Falha operacional (comando/keyring, expiração, descriptografia):
		// propaga em vez de cair no fallback, para não ocultar o problema.
		return nil, fmt.Errorf("falha ao resolver credencial Brave: %w", err)
	}
	token := braveToken(auth)
	if token == "" {
		return nil, errNoBraveCredential
	}

	searchURL := fmt.Sprintf("%s?q=%s&count=%d&offset=%d",
		p.endpoint(), url.QueryEscape(query), maxResults, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", token)
	// Resolução única: aplica aqui todo o material de auth resolvido e
	// pré-define Authorization para que o interceptor do cliente
	// centralizado não execute uma segunda resolução (fontes dinâmicas como
	// `command` teriam custo/efeitos repetidos e poderiam divergir). O header
	// redundante vai para o mesmo endpoint via TLS e é ignorado pela Brave,
	// que autentica pelo X-Subscription-Token.
	for key, val := range auth.Headers {
		if !strings.EqualFold(key, "X-Subscription-Token") {
			req.Header.Set(key, val)
		}
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &braveStatusError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	return parseBraveResponse(body, maxResults)
}

// braveStatusError preserva o status HTTP para a camada da tool decidir entre
// fallback (401/403/429/422: auth/quota/janela de paginação) e erro propagado.
type braveStatusError struct {
	StatusCode int
}

func (e *braveStatusError) Error() string {
	return fmt.Sprintf("Brave HTTP %d", e.StatusCode)
}

// braveFallbackable indica se o status justifica fallback para o DuckDuckGo:
// 401/403 (chave inválida/sem acesso), 429 (quota esgotada) e 422 (offset
// além da janela da Brave API, cujo offset máximo é 9: páginas profundas do
// contrato externo offset/count são servidas pelo fallback). Demais erros
// são propagados como erro da tool.
func braveFallbackable(status int) bool {
	return status == http.StatusUnauthorized ||
		status == http.StatusForbidden ||
		status == http.StatusTooManyRequests ||
		status == http.StatusUnprocessableEntity
}

// braveWebResponse espelha o subconjunto usado da Brave Search API:
// web.results[] com title/url/description. Web é ponteiro para distinguir
// bloco ausente (resposta incompatível => erro) de lista vazia (sem
// resultados => válido).
type braveWebResponse struct {
	Web *struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

// parseBraveResponse converte a resposta JSON da Brave para []SearchResult,
// limitada a maxResults. JSON inválido ou bloco web ausente resulta em erro —
// nunca em resultados fabricados. Campos são aparados antes da validação,
// então títulos/URLs em branco são descartados.
func parseBraveResponse(body []byte, maxResults int) ([]SearchResult, error) {
	var parsed braveWebResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("resposta Brave inválida: %w", err)
	}
	if parsed.Web == nil {
		return nil, fmt.Errorf("resposta Brave sem bloco web")
	}
	results := make([]SearchResult, 0, len(parsed.Web.Results))
	for _, r := range parsed.Web.Results {
		title := strings.TrimSpace(r.Title)
		resultURL := strings.TrimSpace(r.URL)
		if title == "" || resultURL == "" {
			continue
		}
		results = append(results, SearchResult{
			Title:   title,
			URL:     resultURL,
			Snippet: strings.TrimSpace(r.Description),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
