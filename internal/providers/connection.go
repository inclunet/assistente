package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// ConnectionProbeResult contém o resultado detalhado de uma sondagem de conexão a um endpoint LLM.
// É usado pelo wizard de boas-vindas e pelo fluxo de validação de provedores.
type ConnectionProbeResult struct {
	URLReachable    bool
	AuthOK          bool
	ModelsAvailable bool
	Models          []string
	// ErrorType é uma tag semântica para o tipo de erro:
	//   "url_invalid", "url_unreachable", "auth_required", "auth_invalid", "server_error"
	ErrorType   string
	ErrorDetail string
}

// ProbeConnection sonda um endpoint LLM realizando uma requisição GET /models.
// Classifica a resposta em URLReachable, AuthOK, ModelsAvailable e retorna modelos se disponíveis.
// apiKey pode ser vazio — neste caso a função apenas verifica acessibilidade da URL.
func (s *Service) ProbeConnection(ctx context.Context, baseURL, apiKey string) ConnectionProbeResult {
	result := ConnectionProbeResult{}

	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		result.ErrorType = "url_invalid"
		result.ErrorDetail = "URL inválida. Deve começar com http:// ou https:// e conter um endereço válido."
		return result
	}

	modelsEndpoint := strings.TrimSuffix(baseURL, "/") + "/models"
	client := &http.Client{Timeout: 15 * time.Second}
	defer client.CloseIdleConnections()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsEndpoint, nil)
	if err != nil {
		result.ErrorType = "url_unreachable"
		result.ErrorDetail = "Não foi possível preparar a requisição de teste."
		return result
	}

	if apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		result.ErrorType = "url_unreachable"
		result.ErrorDetail = fmt.Sprintf("Não foi possível conectar ao servidor. Verifique se a URL está correta e o servidor está ativo.\n\nDetalhes: %v", err)
		return result
	}
	defer func() { _ = resp.Body.Close() }()

	result.URLReachable = true
	body, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusOK:
		result.AuthOK = true
		var modelsResp struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &modelsResp); err == nil && len(modelsResp.Data) > 0 {
			result.ModelsAvailable = true
			for _, m := range modelsResp.Data {
				result.Models = append(result.Models, m.ID)
			}
			sort.Strings(result.Models)
		}

	case resp.StatusCode == http.StatusUnauthorized:
		if apiKey != "" {
			result.ErrorType = "auth_invalid"
			result.ErrorDetail = "A API Key informada foi rejeitada pelo servidor (401 Unauthorized). Verifique se a chave está correta."
		} else {
			result.ErrorType = "auth_required"
			result.ErrorDetail = "Este servidor requer uma API Key para autenticação."
		}

	case resp.StatusCode == http.StatusForbidden:
		result.ErrorType = "auth_invalid"
		result.ErrorDetail = "Acesso negado (403 Forbidden). A API Key pode não ter permissões suficientes."

	case resp.StatusCode == http.StatusNotFound:
		result.AuthOK = true
		result.ModelsAvailable = false

	case resp.StatusCode >= 500:
		result.ErrorType = "server_error"
		result.ErrorDetail = fmt.Sprintf("O servidor retornou erro %d. O servidor pode estar com problemas temporários.", resp.StatusCode)

	default:
		result.AuthOK = true
		result.ModelsAvailable = false
	}

	return result
}

// ValidateURL verifica se uma URL é bem formada e o servidor está acessível.
// Aceita qualquer resposta HTTP (inclusive 401) — rejeita apenas se o servidor estiver inacessível.
func ValidateURL(ctx context.Context, baseURL string) error {
	parsedURL, err := url.Parse(baseURL)
	if err != nil || parsedURL.Host == "" {
		return fmt.Errorf("URL inválida. Deve conter um endereço de servidor válido")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("URL deve começar com http:// ou https://")
	}

	testURL := strings.TrimSuffix(baseURL, "/") + "/"
	client := &http.Client{Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return fmt.Errorf("não foi possível preparar requisição de teste")
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("não foi possível conectar ao servidor. Verifique se a URL está correta e o servidor está ativo")
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("o servidor retornou erro %d. Pode estar com problemas temporários", resp.StatusCode)
	}
	return nil
}
