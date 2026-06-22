package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

func (p *OpenAIProvider) GetModels(ctx context.Context) ([]string, error) {
	// Tenta via SDK primeiro
	models, err := p.getModelsSDK(ctx)
	if err == nil {
		return models, nil
	}

	// Se o SDK falhou (inclusive por panic capturado), tenta fallback HTTP direto
	log.Printf("[OpenAIProvider] SDK falhou ao listar modelos: %v — tentando fallback HTTP", err)
	return p.getModelsHTTP(ctx)
}

// getModelsSDK lista modelos usando a SDK openai-go.
func (p *OpenAIProvider) getModelsSDK(ctx context.Context) (models []string, retErr error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[OpenAIProvider] PANIC no SDK Models.List: %v", r)
			retErr = fmt.Errorf("panic no SDK: %v", r)
		}
	}()

	page, err := p.client.Models.List(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, fmt.Errorf("models_endpoint_not_supported")
		}
		return nil, fmt.Errorf("erro ao listar modelos: %w", err)
	}
	if page == nil {
		return nil, fmt.Errorf("resposta vazia do servidor ao listar modelos")
	}

	for _, m := range page.Data {
		models = append(models, m.ID)
	}
	sort.Strings(models)
	return models, nil
}

// getModelsHTTP lista modelos via HTTP direto (fallback quando o SDK falha).
func (p *OpenAIProvider) getModelsHTTP(ctx context.Context) ([]string, error) {
	baseURL := strings.TrimSuffix(p.provider.BaseURL, "/")
	modelsURL := baseURL + "/models"
	if !strings.Contains(baseURL, "/v1") {
		modelsURL = baseURL + "/v1/models"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	client := newHTTPClientForProvider(p.provider, p.credMgr)
	client.Timeout = 15 * time.Second

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao provedor: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("models_endpoint_not_supported")
	}
	if resp.StatusCode != http.StatusOK {
		// Preserva o body do upstream na mensagem de erro. Sem isso,
		// status 400/403/etc. viravam caixa preta — o usuário e os
		// logs ficavam sem o motivo real informado pelo provedor (ex.:
		// chave revogada, team_id faltando, header customizado exigido
		// pelo gateway).
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		summary := summarizeHTTPError(resp.StatusCode, errBody)
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("API Key inválida ou não autorizada (%s)", summary)
		}
		return nil, fmt.Errorf("erro ao listar modelos: %s", summary)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resposta: %w", err)
	}

	type modelEntry struct {
		ID string `json:"id"`
	}
	type modelsResponse struct {
		Data []modelEntry `json:"data"`
	}

	var parsed modelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		var arr []modelEntry
		if err2 := json.Unmarshal(body, &arr); err2 != nil {
			return nil, fmt.Errorf("resposta inválida do servidor")
		}
		parsed.Data = arr
	}

	var models []string
	for _, m := range parsed.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	sort.Strings(models)
	return models, nil
}
