package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
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
	Role       string     `json:"role"`
	Content    *string    `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type syncChatRequest struct {
	Model    string            `json:"model"`
	Messages []syncChatMessage `json:"messages"`
	Tools    []Tool            `json:"tools,omitempty"`
}

type syncChatResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   *string    `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// AgentMessage representa uma mensagem interna de um agente para ser salva
type AgentMessage struct {
	Role       string     // "assistant" ou "tool"
	Content    string     // Conteúdo textual
	ToolCalls  []ToolCall // Tool calls (para role="assistant")
	ToolCallID string     // ID da tool call (para role="tool")
	AgentName  string     // Nome do agente
	Model      string     // Modelo usado
}

// MessageSaver é um callback para salvar mensagens internas de agentes
type MessageSaver func(msg AgentMessage) error

// ChatWithTools envia mensagens para o LLM e processa tool calls automaticamente
// Retorna a resposta final após executar todas as tools necessárias
func (c *SyncClient) ChatWithTools(
	ctx context.Context,
	model, systemPrompt string,
	userMessage string,
	tools []Tool,
	toolExecutor func(ToolCall) (string, error),
) (string, error) {
	return c.ChatWithToolsAndSaver(ctx, model, systemPrompt, userMessage, tools, toolExecutor, "", nil)
}

// ChatWithToolsAndSaver é como ChatWithTools mas salva mensagens internas via callback
func (c *SyncClient) ChatWithToolsAndSaver(
	ctx context.Context,
	model, systemPrompt string,
	userMessage string,
	tools []Tool,
	toolExecutor func(ToolCall) (string, error),
	agentName string,
	saver MessageSaver,
) (string, error) {
	messages := []syncChatMessage{
		{Role: "system", Content: StrPtr(systemPrompt)},
		{Role: "user", Content: StrPtr(userMessage)},
	}

	// Loop para processar tool calls
	maxIterations := 10
	for i := 0; i < maxIterations; i++ {
		response, err := c.sendRequest(ctx, model, messages, tools)
		if err != nil {
			return "", fmt.Errorf("erro ao chamar LLM: %w", err)
		}

		if len(response.Choices) == 0 {
			return "", fmt.Errorf("resposta vazia do LLM")
		}

		choice := response.Choices[0]

		// Se não há tool calls, retorna o conteúdo
		if len(choice.Message.ToolCalls) == 0 {
			if choice.Message.Content != nil {
				return *choice.Message.Content, nil
			}
			return "", nil
		}

		// Adiciona a mensagem do assistente com tool calls
		messages = append(messages, syncChatMessage{
			Role:      "assistant",
			Content:   choice.Message.Content,
			ToolCalls: choice.Message.ToolCalls,
		})

		// Salva mensagem de tool call se tiver saver
		if saver != nil {
			saver(AgentMessage{
				Role:      "assistant",
				ToolCalls: choice.Message.ToolCalls,
				AgentName: agentName,
				Model:     model,
			})
		}

		// Executa tool calls em paralelo para melhor performance
		toolResults := executeToolsParallel(choice.Message.ToolCalls, toolExecutor)

		// Adiciona resultados na ordem original das tool calls
		for _, tr := range toolResults {
			messages = append(messages, syncChatMessage{
				Role:       "tool",
				Content:    StrPtr(tr.result),
				ToolCallID: tr.toolCallID,
			})

			// Salva resultado da tool se tiver saver
			if saver != nil {
				// Encontra o nome da tool
				toolName := ""
				for _, tc := range choice.Message.ToolCalls {
					if tc.ID == tr.toolCallID {
						toolName = tc.Function.Name
						break
					}
				}
				saver(AgentMessage{
					Role:       "tool",
					Content:    tr.result,
					ToolCallID: tr.toolCallID,
					AgentName:  toolName,
					Model:      model,
				})
			}
		}
	}

	return "", fmt.Errorf("máximo de iterações atingido")
}

func (c *SyncClient) sendRequest(ctx context.Context, model string, messages []syncChatMessage, tools []Tool) (*syncChatResponse, error) {
	reqBody := syncChatRequest{
		Model:    model,
		Messages: messages,
	}

	if len(tools) > 0 {
		reqBody.Tools = tools
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

// toolResult armazena o resultado de uma execução de tool
type toolResult struct {
	index      int
	toolCallID string
	result     string
}

// executeToolsParallel executa múltiplas tools em paralelo
// Retorna os resultados na mesma ordem das tool calls originais
func executeToolsParallel(toolCalls []ToolCall, executor func(ToolCall) (string, error)) []toolResult {
	if len(toolCalls) == 0 {
		return nil
	}

	// Se houver apenas uma tool, executa diretamente (sem overhead de goroutine)
	if len(toolCalls) == 1 {
		tc := toolCalls[0]
		fmt.Printf("  🔧 [LLM] Executando tool: %s\n", tc.Function.Name)
		result, err := executor(tc)
		if err != nil {
			result = fmt.Sprintf("Erro: %v", err)
			fmt.Printf("  ❌ [LLM] Tool falhou: %s\n", result)
		} else {
			fmt.Printf("  ✅ [LLM] Tool retornou: %s\n", result)
		}
		return []toolResult{{index: 0, toolCallID: tc.ID, result: result}}
	}

	// Múltiplas tools: executa em paralelo
	fmt.Printf("  ⚡ [LLM] Executando %d tools em paralelo\n", len(toolCalls))

	results := make([]toolResult, len(toolCalls))
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, toolCall ToolCall) {
			defer wg.Done()

			fmt.Printf("  🔧 [LLM] [%d/%d] Executando tool: %s\n", idx+1, len(toolCalls), toolCall.Function.Name)

			result, err := executor(toolCall)
			if err != nil {
				result = fmt.Sprintf("Erro: %v", err)
			}

			results[idx] = toolResult{
				index:      idx,
				toolCallID: toolCall.ID,
				result:     result,
			}
		}(i, tc)
	}

	wg.Wait()
	return results
}
