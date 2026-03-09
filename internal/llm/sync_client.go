package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

// extractDomain extrai o domínio de uma URL (ex: "https://api.openai.com/v1" -> "api.openai.com")
func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

// SyncClient implementa um cliente LLM síncrono para uso em agentes
// Diferente do cliente com streaming, este retorna a resposta completa
type SyncClient struct {
	provider   *ProviderConfig
	credMgr    *credentials.Manager
	httpClient *httpclient.Client
}

// NewSyncClient cria um novo cliente LLM síncrono usando um ProviderConfig
func NewSyncClient(provider *ProviderConfig, credMgr *credentials.Manager) *SyncClient {
	// Extract domain from provider baseURL for credential pattern matching
	domain := extractDomain(provider.BaseURL)

	timeout := time.Duration(provider.Timeout) * time.Second
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	// Use provider's CredentialPattern (hostname) if available, otherwise fall back to extracted domain
	hostname := provider.CredentialPattern
	if hostname == "" {
		hostname = domain
	}

	return &SyncClient{
		provider: provider,
		credMgr:  credMgr,
		httpClient: httpclient.New(&httpclient.Config{
			CredentialManager: credMgr,
			Timeout:           timeout,
		}, map[string]string{domain: hostname}), // Map request domain to credential hostname
	}
}

// Estruturas internas para request/response da API (formato simplificado)
type syncChatMessage struct {
	Role    string  `json:"role"`
	Content *string `json:"content,omitempty"`
}

type syncChatRequest struct {
	Model    string            `json:"model"`
	Messages []syncChatMessage `json:"messages"`
}

type syncChatResponse struct {
	Choices []struct {
		Message struct {
			Role    string  `json:"role"`
			Content *string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// SimpleChat envia uma mensagem simples ao LLM e retorna a resposta (sem tools)
func (c *SyncClient) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	sysContent := systemPrompt
	userContent := userMessage

	messages := []syncChatMessage{
		{Role: "system", Content: &sysContent},
		{Role: "user", Content: &userContent},
	}

	resp, err := c.sendRequest(ctx, model, messages)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("nenhuma resposta do LLM")
	}

	if resp.Choices[0].Message.Content == nil {
		return "", fmt.Errorf("resposta vazia do LLM")
	}

	raw := *resp.Choices[0].Message.Content
	final, _, ok := SplitLocalAIChatML(raw)
	if ok {
		return final, nil
	}
	return raw, nil
}

func (c *SyncClient) sendRequest(ctx context.Context, model string, messages []syncChatMessage) (*syncChatResponse, error) {
	reqBody := syncChatRequest{
		Model:    model,
		Messages: messages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	endpoint := BuildEndpoint(c.provider.BaseURL, "chat/completions")
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	// NOTE: Authorization header is injected automatically by httpclient via credMgr

	resp, err := c.httpClient.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", summarizeHTTPError(resp.StatusCode, body))
	}

	var chatResp syncChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	return &chatResp, nil
}
