package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SyncClient implementa um cliente LLM síncrono para uso em agentes
// Diferente do cliente com streaming, este retorna a resposta completa
type SyncClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

// NewSyncClient cria um novo cliente LLM síncrono usando o pool de conexões compartilhado
func NewSyncClient(baseURL, apiKey string) *SyncClient {
	return &SyncClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  SharedHTTPClient, // Usa o pool compartilhado
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

	return *resp.Choices[0].Message.Content, nil
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API retornou status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp syncChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	return &chatResp, nil
}
