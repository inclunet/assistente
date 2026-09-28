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

// bingSearchEndpoint é o endpoint fixo da Bing Web Search API v7.
const bingSearchEndpoint = "https://api.bing.microsoft.com/v7.0/search"

// bingCredentialHint orienta o cadastro da chave no credmanager.
const bingCredentialHint = "cadastre a chave da Bing Web Search API no credmanager para o domínio api.bing.microsoft.com (tipo bearer com a subscription key, ou custom com header Ocp-Apim-Subscription-Key)"

// errNoBingCredential sinaliza ausência de credencial Bing: a tool avança
// para o próximo provedor da cadeia em vez de falhar.
var errNoBingCredential = fmt.Errorf("sem credencial Bing (%s)", bingCredentialHint)

// bingProvider consulta a Bing Web Search API v7 oficial. A chave vem sempre
// do credmanager, resolvida por URL contra o endpoint fixo acima — nunca de
// variável de ambiente, flag ou argumento da tool.
type bingProvider struct {
	credMgr *credentials.Manager
	// endpointOverride permite apontar para um servidor fake em testes.
	endpointOverride string
}

func (p *bingProvider) Name() string { return "Bing" }

func (p *bingProvider) endpoint() string {
	if p.endpointOverride != "" {
		return p.endpointOverride
	}
	return bingSearchEndpoint
}

// bingKey extrai a subscription key aceita no header Ocp-Apim-Subscription-Key
// a partir do AuthConfig resolvido: bearer usa Token; custom usa o header
// Ocp-Apim-Subscription-Key (case-insensitive); demais tipos não servem.
func bingKey(auth *credentials.AuthConfig) string {
	if auth == nil {
		return ""
	}
	if auth.Type == "bearer" {
		return trimBearerPrefix(strings.TrimSpace(auth.Token))
	}
	if auth.Type == "custom" {
		for key, val := range auth.Headers {
			if strings.EqualFold(key, "Ocp-Apim-Subscription-Key") {
				return strings.TrimSpace(val)
			}
		}
	}
	return ""
}

func (p *bingProvider) Search(ctx context.Context, client *httpclient.Client, query string, offset, maxResults int) ([]SearchResult, error) {
	if p.credMgr == nil {
		return nil, errNoBingCredential
	}
	auth, err := p.credMgr.ResolveForURLWithContext(ctx, p.endpoint())
	if err != nil {
		// Falha operacional (comando/keyring, expiração, descriptografia):
		// propaga em vez de avançar na cadeia, para não ocultar o problema.
		return nil, fmt.Errorf("falha ao resolver credencial Bing: %w", err)
	}
	key := bingKey(auth)
	if key == "" {
		return nil, errNoBingCredential
	}

	// Paginação nativa da API (count/offset), mesmo contrato da tool.
	// safeSearch moderado como padrão conservador.
	searchURL := fmt.Sprintf("%s?q=%s&count=%d&offset=%d&safeSearch=Moderate",
		p.endpoint(), url.QueryEscape(query), maxResults, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar requisição: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Ocp-Apim-Subscription-Key", key)
	// Resolução única via WithManualAuth: o interceptor não deve resolver nem
	// injetar nada. Além do custo/efeitos de fontes dinâmicas, o Bing rejeita
	// com 401 (AuthorizationRedundancy) múltiplos métodos de autenticação na
	// mesma request — um `Authorization: Bearer` do interceptor junto da
	// subscription key quebraria toda chamada com credencial bearer.
	for hdr, val := range auth.Headers {
		if !strings.EqualFold(hdr, "Ocp-Apim-Subscription-Key") {
			req.Header.Set(hdr, val)
		}
	}

	resp, err := client.Do(httpclient.WithManualAuth(ctx), req)
	if err != nil {
		return nil, fmt.Errorf("erro na requisição: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, &bingStatusError{StatusCode: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	return parseBingResponse(body, maxResults)
}

// bingStatusError preserva o status HTTP para a camada da tool decidir entre
// avançar na cadeia (auth/quota) e erro propagado.
type bingStatusError struct {
	StatusCode int
}

func (e *bingStatusError) Error() string {
	return fmt.Sprintf("Bing HTTP %d", e.StatusCode)
}

// bingFallbackable indica se o status justifica avançar na cadeia: 401/403
// (chave inválida/sem acesso, incluindo redundância de auth) e 429 (quota
// esgotada). Demais erros são propagados como erro da tool.
func bingFallbackable(status int) bool {
	return status == http.StatusUnauthorized ||
		status == http.StatusForbidden ||
		status == http.StatusTooManyRequests
}

// bingSearchResponse espelha o subconjunto usado da Bing Web Search API v7:
// webPages.value[] com name/url/snippet. WebPages é ponteiro, mas nil significa
// zero resultados (a API omite o bloco quando não há matches — semântica
// documentada), não resposta incompatível.
type bingSearchResponse struct {
	WebPages *struct {
		Value []struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"value"`
		TotalEstimatedMatches int `json:"totalEstimatedMatches"`
	} `json:"webPages"`
}

// parseBingResponse converte a resposta JSON do Bing para []SearchResult,
// limitada a maxResults. JSON inválido resulta em erro — nunca em resultados
// fabricados. Bloco webPages ausente é lista vazia válida. Campos são
// aparados antes da validação, então nomes/URLs em branco são descartados.
func parseBingResponse(body []byte, maxResults int) ([]SearchResult, error) {
	var parsed bingSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("resposta Bing inválida: %w", err)
	}
	if parsed.WebPages == nil {
		return []SearchResult{}, nil
	}
	results := make([]SearchResult, 0, len(parsed.WebPages.Value))
	for _, r := range parsed.WebPages.Value {
		name := strings.TrimSpace(r.Name)
		resultURL := strings.TrimSpace(r.URL)
		if name == "" || resultURL == "" {
			continue
		}
		results = append(results, SearchResult{
			Title:   name,
			URL:     resultURL,
			Snippet: strings.TrimSpace(r.Snippet),
		})
		if len(results) >= maxResults {
			break
		}
	}
	return results, nil
}
