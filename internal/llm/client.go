package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"assistente/internal/config"
	"assistente/internal/credentials"
	httpclient "assistente/internal/tools/http"
)

// Client encapsula a lógica de comunicação com a API LLM com suporte a streaming
type Client struct {
	provider   *ProviderConfig
	cfg        *config.Config // DEPRECATED: mantido para backward compatibility
	credMgr    *credentials.Manager
	httpClient *httpclient.Client
}

// NewClient cria um novo cliente LLM com streaming usando um ProviderConfig
func NewClient(provider *ProviderConfig, cfg *config.Config, credMgr *credentials.Manager) *Client {
	// Extract domain from provider baseURL for credential pattern matching
	domain := ""
	if u, err := url.Parse(provider.BaseURL); err == nil {
		domain = u.Host
	}

	timeout := time.Duration(provider.Timeout) * time.Second
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	// Use provider's CredentialPattern (hostname) if available, otherwise fall back to extracted domain
	hostname := provider.CredentialPattern
	if hostname == "" {
		hostname = domain
	}

	return &Client{
		provider: provider,
		cfg:      cfg,
		credMgr:  credMgr,
		httpClient: httpclient.New(&httpclient.Config{
			CredentialManager: credMgr,
			Timeout:           timeout,
		}, map[string]string{domain: hostname}), // Map request domain to credential hostname
	}
}

// Removed: getToken() - now handled automatically by httpclient

// GetModels retorna a lista de modelos disponíveis na API
func (c *Client) GetModels(ctx context.Context) ([]string, error) {
	endpoint := BuildEndpoint(c.provider.BaseURL, "models")
	log.Printf("[GetModels] Endpoint: %s", endpoint)
	
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	// NOTE: Authorization header is injected automatically by httpclient via credMgr

	resp, err := c.httpClient.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("erro na conexão: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		
		log.Printf("[GetModels] Status: %d", resp.StatusCode)
		if resp.StatusCode == http.StatusNotFound {
			log.Printf("[GetModels] ⚠️  Endpoint não suportado (404)")
			return nil, fmt.Errorf("models_endpoint_not_supported")
		}

		// Para outros erros (401, 5xx, etc), retorna erro normal
		return nil, fmt.Errorf("%s", summarizeHTTPError(resp.StatusCode, body))
	}

	var modelsResp ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("erro ao decodificar resposta: %v", err)
	}

	// Filtra apenas modelos que suportam chat
	var chatModels []string
	for _, model := range modelsResp.Data {
		id := strings.ToLower(model.ID)
		if strings.Contains(id, "gpt") ||
			strings.Contains(id, "chat") ||
			strings.Contains(id, "llama") ||
			strings.Contains(id, "claude") ||
			strings.Contains(id, "mistral") ||
			strings.Contains(id, "gemma") ||
			strings.Contains(id, "qwen") ||
			strings.Contains(id, "phi") ||
			strings.Contains(id, "deepseek") ||
			strings.Contains(id, "o1") ||
			strings.Contains(id, "o3") {
			chatModels = append(chatModels, model.ID)
		}
	}

	// Se não encontrou nenhum modelo com os filtros, retorna todos
	if len(chatModels) == 0 {
		for _, model := range modelsResp.Data {
			chatModels = append(chatModels, model.ID)
		}
	}

	// Ordena alfabeticamente
	sort.Strings(chatModels)

	return chatModels, nil
}

// SendMessageSync envia uma mensagem sem streaming
func (c *Client) SendMessageSync(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	reqBody := ChatRequest{
		Model:       params.Model,
		Messages:    messages,
		Temperature: params.Temperature,
		Stream:      false,
	}

	// Usar max_completion_tokens ou max_tokens baseado no MaxTokensMode do profile
	// Default: "legacy" (max_tokens) para compatibilidade
	if params.MaxTokensMode == "completion_tokens" {
		reqBody.MaxCompletionTokens = params.MaxTokens
	} else {
		// Default: legacy (max_tokens) para compatibilidade
		reqBody.MaxTokens = params.MaxTokens
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := BuildEndpoint(c.provider.BaseURL, "chat/completions")
	log.Printf("[SendMessageSync] Endpoint: %s | Model: %s | Stream: false", endpoint, params.Model)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	// NOTE: Authorization header is injected automatically by httpclient via credMgr

	resp, err := c.httpClient.Do(ctx, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[SendMessageSync] Status: %d | Model: %s | Body vazio: %v", 
			resp.StatusCode, params.Model, len(body) == 0)
		return "", fmt.Errorf("%s", summarizeHTTPError(resp.StatusCode, body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) > 0 {
		raw := chatResp.Choices[0].Message.GetContentAsString()
		final, _, ok := SplitLocalAIChatML(raw)
		if ok {
			return final, nil
		}
		return raw, nil
	}

	return "", fmt.Errorf("nenhuma resposta recebida")
}

// StreamHandler é uma interface para lidar com eventos de streaming
type StreamHandler interface {
	// OnChunk é chamado quando um chunk de conteúdo é recebido
	OnChunk(content string)
	// OnThinking é chamado quando um chunk de reasoning/thinking é recebido
	OnThinking(content string)
	// OnThinkingDone é chamado quando o reasoning termina
	OnThinkingDone(fullReasoning string)
	// OnToolCalls é chamado quando o LLM solicita execução de ferramentas.
	// fullResponse é o texto acumulado antes das tool calls.
	// Chamado em vez de OnDone quando finish_reason é "tool_calls".
	OnToolCalls(calls []ToolCall, fullResponse string, usage Usage, model string)
	// OnError é chamado quando ocorre um erro
	OnError(err string)
	// OnDone é chamado quando o streaming termina (finish_reason="stop")
	OnDone(fullResponse string, usage Usage, model string)
}

// StreamChat realiza o streaming da resposta.
// O parâmetro variadic tools permite enviar definições de ferramentas ao LLM (opcional).
func (c *Client) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	// Usa modelo padrão se não especificado
	model := params.Model
	if model == "" {
		// Try provider's default model first
		if c.provider.Model != "" {
			model = c.provider.Model
		} else if c.cfg != nil && c.cfg.DefaultModel != "" {
			model = c.cfg.DefaultModel
		}

		if model == "" {
			handler.OnError("Nenhum modelo especificado e nenhum modelo padrão configurado")
			return
		}
	}

	reqBody := ChatRequest{
		Model:       model,
		Messages:    messages,
		Temperature: params.Temperature,
		Stream:      true,
		StreamOptions: &StreamOptions{
			IncludeUsage: true,
		},
	}

	// Usar max_completion_tokens ou max_tokens baseado no MaxTokensMode do profile
	// Default: "legacy" (max_tokens) para compatibilidade
	if params.MaxTokensMode == "completion_tokens" {
		reqBody.MaxCompletionTokens = params.MaxTokens
	} else {
		// Default: legacy (max_tokens) para compatibilidade
		reqBody.MaxTokens = params.MaxTokens
	}

	// Só inclui top_p se for diferente do padrão (1.0) para evitar erros com alguns modelos/proxies
	if params.TopP > 0 && params.TopP != 1.0 {
		reqBody.TopP = &params.TopP
	}

	// Reasoning: "ollama" envia think=true, "low/medium/high" envia reasoning_effort
	switch params.ReasoningEffort {
	case "ollama":
		reqBody.Think = BoolPtr(true)
	case "none", "low", "medium", "high", "max":
		reqBody.ReasoningEffort = params.ReasoningEffort
	}

	// Adiciona ferramentas se fornecidas
	if len(tools) > 0 {
		reqBody.Tools = tools
		reqBody.ToolChoice = "auto"
		if choice, ok := toolChoiceFromContext(ctx); ok {
			reqBody.ToolChoice = choice
		}
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		handler.OnError("Erro ao preparar requisição: " + err.Error())
		return
	}

	endpoint := BuildEndpoint(c.provider.BaseURL, "chat/completions")
	log.Printf("[StreamChat] Endpoint: %s | Model: %s | Stream: true", endpoint, model)

	// Retry do streaming (principalmente para 524/5xx e falhas de rede) —
	// só é seguro fazer retry se ainda não emitimos nada (para evitar duplicação no chat).
	const maxAttempts = 10
	backoff := 500 * time.Millisecond
	maxBackoff := 8 * time.Second

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			handler.OnError("Streaming cancelado: " + ctx.Err().Error())
			return
		default:
		}

		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonBody))
		if err != nil {
			handler.OnError("Erro ao criar requisição: " + err.Error())
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		// NOTE: Authorization header is injected automatically by httpclient via credMgr

		resp, err := c.httpClient.Do(ctx, req)
		if err != nil {
			if attempt < maxAttempts {
				sleepWithJitter(ctx, backoff)
				backoff = nextBackoff(backoff, maxBackoff)
				continue
			}
			handler.OnError("Erro na conexão: " + err.Error())
			return
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			log.Printf("[StreamChat] Tentativa %d/%d - Status: %d | Modelo: %s | Body vazio: %v", 
				attempt, maxAttempts, resp.StatusCode, model, len(body) == 0)

			// Alguns proxies/modelos OpenAI-compat não suportam tool_choice="required".
			// Para não quebrar o perfil do editor, fazemos downgrade para "auto" e tentamos novamente.
			if reqBody.ToolChoice == "required" {
				lower := strings.ToLower(string(body))
				if strings.Contains(lower, "tool_choice") || strings.Contains(lower, "tool choice") {
					reqBody.ToolChoice = "auto"
					if b, err := json.Marshal(reqBody); err == nil {
						jsonBody = b
						if attempt < maxAttempts {
							continue
						}
					}
				}
			}

			if attempt < maxAttempts && shouldRetryHTTPStatus(resp.StatusCode) {
				sleepWithJitter(ctx, backoff)
				backoff = nextBackoff(backoff, maxBackoff)
				continue
			}
			handler.OnError(summarizeHTTPError(resp.StatusCode, body))
			return
		}

		reader := bufio.NewReader(resp.Body)
		var fullResponse strings.Builder
		var fullReasoning strings.Builder // Acumula reasoning/thinking
		var lastUsage Usage
		var lastModel string
		var lastFinishReason string
		var isThinking bool                // Estado para detectar thinking em tags
		var thinkingBuffer strings.Builder // Buffer para conteúdo dentro de <thinking>
		var localAIState localAIChatMLState
		var emittedAnything bool
		receivedDone := false

		// Acumula tool_calls incrementais (LLM envia argumentos em fragmentos)
		toolCallAccumulator := make(map[int]*ToolCall) // index -> ToolCall parcial

		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				_ = resp.Body.Close()
				if attempt < maxAttempts && !emittedAnything {
					sleepWithJitter(ctx, backoff)
					backoff = nextBackoff(backoff, maxBackoff)
					goto nextAttempt
				}
				handler.OnError("Erro ao ler resposta: " + err.Error())
				return
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			if data == "[DONE]" {
				receivedDone = true
				_ = resp.Body.Close()

				// Notifica fim do reasoning se houver
				if fullReasoning.Len() > 0 {
					handler.OnThinkingDone(fullReasoning.String())
				}

				// Verifica se foi finish_reason: "tool_calls"
				if lastFinishReason == "tool_calls" {
					calls := buildToolCallsFromAccumulator(toolCallAccumulator)
					if len(calls) > 0 {
						emittedAnything = true
						handler.OnToolCalls(calls, fullResponse.String(), lastUsage, lastModel)
						return
					}
				}

				emittedAnything = true
				handler.OnDone(fullResponse.String(), lastUsage, lastModel)
				return
			}

			var chatResp ChatResponse
			if err := json.Unmarshal([]byte(data), &chatResp); err != nil {
				continue
			}

			if chatResp.Model != "" {
				lastModel = chatResp.Model
			}

			if chatResp.Usage.TotalTokens > 0 {
				lastUsage = chatResp.Usage
			}

			if len(chatResp.Choices) > 0 {
				choice := chatResp.Choices[0]

				// Captura finish_reason
				if choice.FinishReason != "" {
					lastFinishReason = choice.FinishReason
				}

				// Processa reasoning_content (DeepSeek, Qwen, etc)
				if choice.Delta.ReasoningContent != "" {
					reasoning := choice.Delta.ReasoningContent
					fullReasoning.WriteString(reasoning)
					emittedAnything = true
					handler.OnThinking(reasoning)
				}

				// Processa thinking do Ollama
				if choice.Delta.Thinking != "" {
					thinking := choice.Delta.Thinking
					fullReasoning.WriteString(thinking)
					emittedAnything = true
					handler.OnThinking(thinking)
				}

				// Processa thinking na mensagem completa (Ollama non-streaming ou chunk final)
				if choice.Message.Thinking != "" {
					thinking := choice.Message.Thinking
					// Só adiciona se não foi processado via Delta
					if !strings.Contains(fullReasoning.String(), thinking) {
						fullReasoning.WriteString(thinking)
						emittedAnything = true
						handler.OnThinking(thinking)
					}
				}

				// Processa tool_calls incrementais no delta
				if len(choice.Delta.ToolCalls) > 0 {
					for _, tcDelta := range choice.Delta.ToolCalls {
						acc, exists := toolCallAccumulator[tcDelta.Index]
						if !exists {
							acc = &ToolCall{Type: "function"}
							toolCallAccumulator[tcDelta.Index] = acc
						}
						// Acumula ID (vem no primeiro delta do index)
						if tcDelta.ID != "" {
							acc.ID = tcDelta.ID
						}
						if tcDelta.Type != "" {
							acc.Type = tcDelta.Type
						}
						// Acumula nome e argumentos (argumentos vêm em fragmentos)
						if tcDelta.Function != nil {
							if tcDelta.Function.Name != "" {
								acc.Function.Name = tcDelta.Function.Name
							}
							acc.Function.Arguments += tcDelta.Function.Arguments
						}
					}
				}

				// Processa tool_calls na mensagem completa (resposta não-streaming)
				if len(choice.Message.ToolCalls) > 0 && len(toolCallAccumulator) == 0 {
					for i, tc := range choice.Message.ToolCalls {
						toolCallAccumulator[i] = &ToolCall{
							ID:   tc.ID,
							Type: tc.Type,
							Function: FunctionCall{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						}
					}
				}

				// Processa conteúdo
				if choice.Delta.Content != "" {
					content := choice.Delta.Content

					// LocalAI pode devolver reasoning no próprio content via tokens ChatML (<|channel|>analysis...)
					content = processLocalAIChatML(content, &localAIState, &fullReasoning, handler)

					// Detecta tags <thinking> no conteúdo (Claude via OpenRouter, etc)
					content = processThinkingTags(content, &isThinking, &thinkingBuffer, &fullReasoning, handler)

					// Se ainda sobrou conteúdo após extrair thinking
					if content != "" {
						fullResponse.WriteString(content)
						emittedAnything = true
						handler.OnChunk(content)
					}
				}
			}
		}

		_ = resp.Body.Close()
		if !receivedDone {
			if attempt < maxAttempts && !emittedAnything {
				sleepWithJitter(ctx, backoff)
				backoff = nextBackoff(backoff, maxBackoff)
				continue
			}
			handler.OnError("Streaming finalizou sem sinal [DONE]")
			return
		}

		return

	nextAttempt:
		continue
	}
}

func shouldRetryHTTPStatus(code int) bool {
	if code == 408 || code == 425 || code == 429 {
		return true
	}
	if code >= 500 && code <= 599 {
		return true
	}
	return false
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

func sleepWithJitter(ctx context.Context, base time.Duration) {
	if base <= 0 {
		return
	}

	jitter := time.Duration(rand.Intn(250)) * time.Millisecond
	wait := base + jitter

	t := time.NewTimer(wait)
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// buildToolCallsFromAccumulator converte o acumulador de deltas em tool calls finais
func buildToolCallsFromAccumulator(acc map[int]*ToolCall) []ToolCall {
	if len(acc) == 0 {
		return nil
	}

	// Encontra o maior índice
	maxIdx := 0
	for idx := range acc {
		if idx > maxIdx {
			maxIdx = idx
		}
	}

	// Monta slice ordenado por índice
	calls := make([]ToolCall, 0, len(acc))
	for i := 0; i <= maxIdx; i++ {
		if tc, ok := acc[i]; ok {
			calls = append(calls, *tc)
		}
	}

	return calls
}

// processThinkingTags detecta e extrai conteúdo de tags <thinking> do streaming
// Retorna o conteúdo que NÃO é thinking para ser processado normalmente
func processThinkingTags(content string, isThinking *bool, thinkingBuffer, fullReasoning *strings.Builder, handler StreamHandler) string {
	var result strings.Builder
	i := 0

	for i < len(content) {
		if *isThinking {
			// Procura pelo fim da tag </thinking>
			endIdx := strings.Index(content[i:], "</thinking>")
			if endIdx != -1 {
				// Adiciona conteúdo até o fim da tag ao buffer de thinking
				thinkingContent := content[i : i+endIdx]
				thinkingBuffer.WriteString(thinkingContent)
				fullReasoning.WriteString(thinkingContent)
				handler.OnThinking(thinkingContent)

				*isThinking = false
				i += endIdx + len("</thinking>")
			} else {
				// Tag ainda não fechou, todo o resto é thinking
				thinkingContent := content[i:]
				thinkingBuffer.WriteString(thinkingContent)
				fullReasoning.WriteString(thinkingContent)
				handler.OnThinking(thinkingContent)
				return result.String()
			}
		} else {
			// Procura pelo início da tag <thinking>
			startIdx := strings.Index(content[i:], "<thinking>")
			if startIdx != -1 {
				// Adiciona conteúdo antes da tag ao resultado
				result.WriteString(content[i : i+startIdx])
				*isThinking = true
				thinkingBuffer.Reset()
				i += startIdx + len("<thinking>")
			} else {
				// Sem mais tags, adiciona o resto ao resultado
				result.WriteString(content[i:])
				break
			}
		}
	}

	return result.String()
}
