package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"assistente/internal/config"
)

// Client encapsula a lógica de comunicação com a API LLM
type Client struct {
	cfg *config.Config
}

// NewClient cria um novo cliente LLM
func NewClient(cfg *config.Config) *Client {
	return &Client{cfg: cfg}
}

// GetModels retorna a lista de modelos disponíveis na API
func GetModels(cfg *config.Config) ([]string, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("API Key não configurada")
	}

	endpoint := BuildEndpoint(cfg.APIBaseURL, "models")
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := SharedHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erro na conexão: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erro na API (%d): %s", resp.StatusCode, string(body))
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
func SendMessageSync(cfg *config.Config, messages []Message, params ChatParams) (string, error) {
	if cfg.APIKey == "" {
		return "", fmt.Errorf("API Key não configurada")
	}

	reqBody := ChatRequest{
		Model:       params.Model,
		Messages:    messages,
		MaxTokens:   params.MaxTokens,
		Temperature: params.Temperature,
		Stream:      false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := BuildEndpoint(cfg.APIBaseURL, "chat/completions")
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := SharedHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("erro na API: %s", string(body))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.GetContentAsString(), nil
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
	// OnError é chamado quando ocorre um erro
	OnError(err string)
	// OnDone é chamado quando o streaming termina
	OnDone(fullResponse string, usage Usage, model string)
}

// StreamChat realiza o streaming da resposta
func StreamChat(ctx context.Context, cfg *config.Config, messages []Message, params ChatParams, handler StreamHandler) {
	// Usa modelo padrão se não especificado
	model := params.Model
	if model == "" {
		model = cfg.DefaultModel
		if model == "" {
			handler.OnError("Nenhum modelo especificado e nenhum modelo padrão configurado")
			return
		}
	}

	reqBody := ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   params.MaxTokens,
		Temperature: params.Temperature,
		Stream:      true,
		StreamOptions: &StreamOptions{
			IncludeUsage: true,
		},
	}

	// Só inclui top_p se for diferente do padrão (1.0) para evitar erros com alguns modelos/proxies
	if params.TopP > 0 && params.TopP != 1.0 {
		reqBody.TopP = &params.TopP
	}

	// Habilita thinking se configurado no perfil (necessário para Ollama)
	if params.EnableThinking {
		reqBody.Think = BoolPtr(true)
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		handler.OnError("Erro ao preparar requisição: " + err.Error())
		return
	}

	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	endpoint := BuildEndpoint(cfg.APIBaseURL, "chat/completions")
	req, err := http.NewRequestWithContext(reqCtx, "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		handler.OnError("Erro ao criar requisição: " + err.Error())
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := SharedHTTPClient.Do(req)
	if err != nil {
		handler.OnError("Erro na conexão: " + err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		handler.OnError(fmt.Sprintf("Erro na API (%d): %s", resp.StatusCode, bodyStr))
		return
	}

	reader := bufio.NewReader(resp.Body)
	var fullResponse strings.Builder
	var fullReasoning strings.Builder // Acumula reasoning/thinking
	var lastUsage Usage
	var lastModel string
	var isThinking bool                // Estado para detectar thinking em tags
	var thinkingBuffer strings.Builder // Buffer para conteúdo dentro de <thinking>

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
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
			// Notifica fim do reasoning se houver
			if fullReasoning.Len() > 0 {
				handler.OnThinkingDone(fullReasoning.String())
			}

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

			// Processa reasoning_content (DeepSeek, Qwen, etc)
			if choice.Delta.ReasoningContent != "" {
				reasoning := choice.Delta.ReasoningContent
				fullReasoning.WriteString(reasoning)
				handler.OnThinking(reasoning)
			}

			// Processa thinking do Ollama
			if choice.Delta.Thinking != "" {
				thinking := choice.Delta.Thinking
				fullReasoning.WriteString(thinking)
				handler.OnThinking(thinking)
			}

			// Processa thinking na mensagem completa (Ollama non-streaming ou chunk final)
			if choice.Message.Thinking != "" {
				thinking := choice.Message.Thinking
				// Só adiciona se não foi processado via Delta
				if !strings.Contains(fullReasoning.String(), thinking) {
					fullReasoning.WriteString(thinking)
					handler.OnThinking(thinking)
				}
			}

			// Processa conteúdo
			if choice.Delta.Content != "" {
				content := choice.Delta.Content

				// Detecta tags <thinking> no conteúdo (Claude via OpenRouter, etc)
				content = processThinkingTags(content, &isThinking, &thinkingBuffer, &fullReasoning, handler)

				// Se ainda sobrou conteúdo após extrair thinking
				if content != "" {
					fullResponse.WriteString(content)
					handler.OnChunk(content)
				}
			}
		}
	}
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
