package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

// tavilySearchEndpoint é o endpoint fixo da Tavily Search API.
const tavilySearchEndpoint = "https://api.tavily.com/search"

// tavilyMaxWindow é o maior max_results aceito pela Tavily por chamada. A
// API não tem parâmetro de offset: páginas do contrato externo além desta
// janela caem no próximo provedor da cadeia, sem gastar créditos.
const tavilyMaxWindow = 20

// tavilyCredentialHint orienta o cadastro da chave no credmanager.
const tavilyCredentialHint = "cadastre a chave da Tavily Search API no credmanager para o domínio api.tavily.com (tipo bearer com o token)"

// errNoTavilyCredential sinaliza ausência de credencial Tavily: a tool avança
// para o próximo provedor da cadeia em vez de falhar.
var errNoTavilyCredential = fmt.Errorf("sem credencial Tavily (%s)", tavilyCredentialHint)

// errTavilyWindowExceeded sinaliza offset além da janela servível pela
// Tavily: a tool avança para o próximo provedor sem consumir créditos.
var errTavilyWindowExceeded = fmt.Errorf("offset além da janela Tavily (máx %d resultados por chamada)", tavilyMaxWindow)

// tavilyProvider consulta a Tavily Search API (busca otimizada para agentes).
// A chave vem sempre do credmanager, resolvida por URL contra o endpoint
// fixo acima — nunca de variável de ambiente, flag ou argumento da tool.
type tavilyProvider struct {
	credMgr *credentials.Manager
	// endpointOverride permite apontar para um servidor fake em testes.
	endpointOverride string
}

func (p *tavilyProvider) Name() string { return "Tavily" }

func (p *tavilyProvider) endpoint() string {
	if p.endpointOverride != "" {
		return p.endpointOverride
	}
	return tavilySearchEndpoint
}

// tavilyToken extrai o token Bearer a partir do AuthConfig resolvido: bearer
// usa Token; custom usa o header Authorization (case-insensitive, com ou sem
// prefixo "Bearer ", em qualquer caixa); demais tipos não servem.
func tavilyToken(auth *credentials.AuthConfig) string {
	if auth == nil {
		return ""
	}
	if auth.Type == "bearer" {
		return trimBearerPrefix(strings.TrimSpace(auth.Token))
	}
	if auth.Type == "custom" {
		for key, val := range auth.Headers {
			if strings.EqualFold(key, "Authorization") {
				return trimBearerPrefix(strings.TrimSpace(val))
			}
		}
	}
	return ""
}

// tavilySearchRequest é o corpo POST enviado à Tavily. search_depth básico
// (1 crédito) é suficiente para descoberta; conteúdo profundo é papel do
// web_fetch sobre as URLs escolhidas.
type tavilySearchRequest struct {
	Query       string `json:"query"`
	SearchDepth string `json:"search_depth"`
	MaxResults  int    `json:"max_results"`
}

func (p *tavilyProvider) Search(ctx context.Context, client *httpclient.Client, query string, offset, maxResults int) ([]SearchResult, error) {
	if p.credMgr == nil {
		return nil, errNoTavilyCredential
	}
	// Short-circuit antes de resolver a credencial: offset além da janela
	// nunca chama a API, então não deve executar fontes dinâmicas
	// (comando/keyring) nem gastar nada.
	if offset >= tavilyMaxWindow {
		return nil, errTavilyWindowExceeded
	}
	auth, err := p.credMgr.ResolveForURLWithContext(ctx, p.endpoint())
	if err != nil {
		// Falha operacional (comando/keyring, expiração, descriptografia):
		// propaga em vez de avançar na cadeia, para não ocultar o problema.
		return nil, fmt.Errorf("falha ao resolver credencial Tavily: %w", err)
	}
	token := tavilyToken(auth)
	if token == "" {
		return nil, errNoTavilyCredential
	}

	// Sem offset na API: serve a janela inicial pedindo offset+maxResults
	// (limitado ao teto) e fatia localmente.
	want := offset + maxResults
	if want > tavilyMaxWindow {
		want = tavilyMaxWindow
	}
	payload, err := json.Marshal(tavilySearchRequest{
		Query:       query,
		SearchDepth: "basic",
		MaxResults:  want,
	})
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar requisição: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	// Resolução única: aplica aqui o material de auth resolvido e pré-define
	// Authorization para que o interceptor do cliente centralizado não
	// execute uma segunda resolução (fontes dinâmicas como `command` teriam
	// custo/efeitos repetidos e poderiam divergir). O header vai para o
	// endpoint fixo via TLS.
	req.Header.Set("Authorization", "Bearer "+token)
	for key, val := range auth.Headers {
		if !strings.EqualFold(key, "Authorization") {
			req.Header.Set(key, val)
		}
	}

	resp, err := client.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &tavilyStatusError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	return parseTavilyResponse(body, offset, maxResults)
}

// tavilyStatusError preserva o status HTTP para a camada da tool decidir
// entre avançar na cadeia (auth/quota/limite de plano) e erro propagado.
type tavilyStatusError struct {
	StatusCode int
}

func (e *tavilyStatusError) Error() string {
	return fmt.Sprintf("Tavily HTTP %d", e.StatusCode)
}

// tavilyFallbackable indica se o status justifica avançar na cadeia:
// 401/403 (chave inválida/sem acesso), 429 (rate limit) e 432/433 (limite
// de plano/pay-as-you-go esgotado). Demais erros são propagados como erro
// da tool.
func tavilyFallbackable(status int) bool {
	return status == http.StatusUnauthorized ||
		status == http.StatusForbidden ||
		status == http.StatusTooManyRequests ||
		status == 432 ||
		status == 433
}

// tavilySearchResponse espelha o subconjunto usado da Tavily Search API:
// results[] com title/url/content. Results é ponteiro para distinguir bloco
// ausente (resposta incompatível => erro) de lista vazia (sem resultados =>
// válido). O campo answer é ignorado de propósito: resposta opinativa do
// provedor não entra no contrato canônico de descoberta.
type tavilySearchResponse struct {
	Results *[]struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

// parseTavilyResponse converte a resposta JSON da Tavily para []SearchResult,
// aplicando o offset do contrato externo sobre a janela retornada e
// limitando a maxResults. JSON inválido ou bloco results ausente resulta em
// erro — nunca em resultados fabricados. Campos são aparados antes da
// validação, então títulos/URLs em branco são descartados.
func parseTavilyResponse(body []byte, offset, maxResults int) ([]SearchResult, error) {
	var parsed tavilySearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("resposta Tavily inválida: %w", err)
	}
	if parsed.Results == nil {
		return nil, fmt.Errorf("resposta Tavily sem bloco results")
	}
	results := make([]SearchResult, 0, maxResults)
	skipped := 0
	for _, r := range *parsed.Results {
		title := strings.TrimSpace(r.Title)
		resultURL := strings.TrimSpace(r.URL)
		if title == "" || resultURL == "" {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		results = append(results, SearchResult{
			Title:   title,
			URL:     resultURL,
			Snippet: strings.TrimSpace(r.Content),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
