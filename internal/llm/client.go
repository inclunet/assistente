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
	"sync"

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
	// OnError é chamado quando ocorre um erro
	OnError(err string)
	// OnDone é chamado quando o streaming termina
	OnDone(fullResponse string, usage Usage, model string)
	// OnToolCalls é chamado quando há tool calls para processar
	OnToolCalls(toolCalls []ToolCall, usage Usage, model string)
	// OnToolsExecuting é chamado quando ferramentas estão sendo executadas
	OnToolsExecuting(toolNames []string)
	// OnToolResults é chamado com resultados das ferramentas
	OnToolResults(results []string, usage Usage, model string)
	// GetTools retorna as ferramentas disponíveis
	GetTools() []Tool
	// ExecuteTool executa uma ferramenta
	ExecuteTool(tc ToolCall) (string, error)
	// SupportsVision verifica se o modelo suporta visão
	SupportsVision(model string) (bool, error)
	// SetVisionSupport define se o modelo suporta visão
	SetVisionSupport(model string, supports bool)
	// GetImageModel retorna o modelo para processar imagens
	GetImageModel() string
	// GenerateImageDescription gera descrição de imagem
	GenerateImageDescription(imageBase64, model string) (string, error)
}

// StreamChat realiza o streaming da resposta
func StreamChat(ctx context.Context, cfg *config.Config, messages []Message, params ChatParams, handler StreamHandler) {
	// Verifica se há imagens nas mensagens
	hasImages := HasImages(messages)

	if hasImages {
		// Verifica se o modelo atual suporta visão
		supportsVision, _ := handler.SupportsVision(params.Model)

		if !supportsVision {
			// Modelo não suporta visão, tenta usar modelo auxiliar
			imageModel := handler.GetImageModel()

			if imageModel != "" && imageModel != params.Model {
				fmt.Printf("🖼️ [VISION] Modelo %s não suporta visão, usando %s para processar imagens\n", params.Model, imageModel)

				// Processa imagens com modelo auxiliar
				processedMessages := ProcessImagesWithAuxiliaryModel(messages, imageModel, handler)
				streamChatWithTools(ctx, cfg, processedMessages, params, handler, 0)
				return
			}

			fmt.Printf("🖼️ [VISION] Tentando enviar imagens com modelo %s (capacidade desconhecida)\n", params.Model)
		}
	}

	streamChatWithTools(ctx, cfg, messages, params, handler, 0)
}

// streamChatWithTools realiza o streaming com suporte a tools
func streamChatWithTools(ctx context.Context, cfg *config.Config, messages []Message, params ChatParams, handler StreamHandler, depth int) {
	fmt.Printf("🔧 [DEBUG] streamChatWithTools - depth=%d, useTools=%v, model=%s\n", depth, params.UseTools, params.Model)

	// Previne loops infinitos
	if depth > 100 {
		handler.OnError("Limite de chamadas de ferramentas atingido (100 iterações)")
		return
	}

	// Usa modelo padrão se não especificado
	model := params.Model
	if model == "" {
		model = cfg.DefaultModel
		if model == "" {
			handler.OnError("Nenhum modelo especificado e nenhum modelo padrão configurado")
			return
		}
		fmt.Printf("🔧 [DEBUG] Usando modelo padrão: %s\n", model)
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

	// Adiciona tools se habilitado
	if params.UseTools {
		reqBody.Tools = handler.GetTools()
		fmt.Printf("🔧 [DEBUG] Tools habilitadas - %d ferramentas carregadas\n", len(reqBody.Tools))
	}

	fmt.Printf("🔧 [DEBUG] Enviando %d mensagens para a API\n", len(messages))

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

	// #region agent log
	fmt.Printf("🔧 [CLIENT] Fazendo requisição - ResponseHeaderTimeout atual: %v\n", GetResponseHeaderTimeout())
	// #endregion

	resp, err := SharedHTTPClient.Do(req)
	if err != nil {
		// #region agent log
		fmt.Printf("🔧 [CLIENT] ERRO na requisição: %v\n", err)
		// #endregion
		handler.OnError("Erro na conexão: " + err.Error())
		return
	}
	defer resp.Body.Close()

	fmt.Printf("🔧 [DEBUG] Resposta recebida - status=%d\n", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)

		// Detecta erro de visão para aprender capacidades do modelo
		hasImages := HasImages(messages)
		if hasImages && (strings.Contains(bodyStr, "image") || strings.Contains(bodyStr, "vision") || strings.Contains(bodyStr, "multimodal")) {
			fmt.Printf("🖼️ [VISION] Modelo %s não suporta visão (erro da API)\n", params.Model)
			handler.SetVisionSupport(params.Model, false)

			// Tenta usar modelo auxiliar
			imageModel := handler.GetImageModel()
			if imageModel != "" && imageModel != params.Model {
				fmt.Printf("🖼️ [VISION] Tentando novamente com modelo auxiliar %s\n", imageModel)
				processedMessages := ProcessImagesWithAuxiliaryModel(messages, imageModel, handler)
				streamChatWithTools(ctx, cfg, processedMessages, params, handler, depth)
				return
			}
		}

		handler.OnError(fmt.Sprintf("Erro na API (%d): %s", resp.StatusCode, bodyStr))
		return
	}

	// Se chegou aqui com imagens, o modelo suporta visão
	if HasImages(messages) {
		handler.SetVisionSupport(params.Model, true)
	}

	reader := bufio.NewReader(resp.Body)
	var fullResponse strings.Builder
	var lastUsage Usage
	var lastModel string
	var toolCalls []ToolCall
	var currentToolCall *ToolCall
	var finishReason string

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
			if currentToolCall != nil {
				toolCalls = append(toolCalls, *currentToolCall)
				currentToolCall = nil
			}

			fmt.Printf("🔧 [DEBUG] Stream [DONE] - finishReason=%s, toolCalls=%d\n", finishReason, len(toolCalls))

			hasToolCalls := len(toolCalls) > 0
			isToolFinish := finishReason == "tool_calls" || finishReason == "function_call"

			if hasToolCalls && (isToolFinish || finishReason == "") {
				fmt.Printf("🔧 [DEBUG] Processando %d tool calls...\n", len(toolCalls))
				processToolCalls(ctx, cfg, messages, toolCalls, params, handler, depth, lastUsage, lastModel)
				return
			} else if hasToolCalls {
				fmt.Printf("⚠️ [DEBUG] Tool calls detectadas mas finishReason=%s, processando mesmo assim...\n", finishReason)
				processToolCalls(ctx, cfg, messages, toolCalls, params, handler, depth, lastUsage, lastModel)
				return
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

			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}

			// Processa tool calls
			if len(choice.Delta.ToolCalls) > 0 {
				for _, tc := range choice.Delta.ToolCalls {
					if tc.ID != "" {
						if currentToolCall != nil {
							toolCalls = append(toolCalls, *currentToolCall)
						}
						currentToolCall = &ToolCall{
							ID:   tc.ID,
							Type: tc.Type,
							Function: FunctionCall{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						}
					} else if currentToolCall != nil {
						currentToolCall.Function.Arguments += tc.Function.Arguments
					}
				}
			}

			// Processa conteúdo
			if choice.Delta.Content != "" {
				content := choice.Delta.Content
				fullResponse.WriteString(content)
				handler.OnChunk(content)
			}
		}
	}

	if currentToolCall != nil {
		toolCalls = append(toolCalls, *currentToolCall)
	}
}

// processToolCalls processa as chamadas de ferramentas
func processToolCalls(ctx context.Context, cfg *config.Config, messages []Message, toolCalls []ToolCall, params ChatParams, handler StreamHandler, depth int, usage Usage, model string) {
	fmt.Printf("🔧 [DEBUG] processToolCalls - depth=%d, toolCalls=%d\n", depth, len(toolCalls))

	// Notifica sobre as tool calls para acumulação no histórico
	handler.OnToolCalls(toolCalls, usage, model)

	toolNames := make([]string, len(toolCalls))
	for i, tc := range toolCalls {
		toolNames[i] = tc.Function.Name
	}
	handler.OnToolsExecuting(toolNames)

	// Cria mensagem do assistant com tool calls
	assistantMsg := Message{
		Role:      "assistant",
		ToolCalls: toolCalls,
	}
	newMessages := append(messages, assistantMsg)

	// Executa tools em paralelo se houver múltiplas
	var toolResults []string
	if len(toolCalls) == 1 {
		// Uma única tool: executa diretamente (sem overhead)
		tc := toolCalls[0]
		fmt.Printf("🔧 [STREAM] Executando tool: %s\n", tc.Function.Name)
		result, err := handler.ExecuteTool(tc)
		if err != nil {
			result = fmt.Sprintf("Erro ao executar ferramenta: %v", err)
		}
		toolResults = append(toolResults, result)
		newMessages = append(newMessages, Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    StrPtr(result),
		})
	} else {
		// Múltiplas tools: executa em paralelo
		fmt.Printf("⚡ [STREAM] Executando %d tools em paralelo\n", len(toolCalls))

		type indexedResult struct {
			index      int
			toolCallID string
			result     string
		}
		results := make([]indexedResult, len(toolCalls))
		var wg sync.WaitGroup

		for i, tc := range toolCalls {
			wg.Add(1)
			go func(idx int, toolCall ToolCall) {
				defer wg.Done()
				fmt.Printf("  🔧 [STREAM] [%d/%d] Executando: %s\n", idx+1, len(toolCalls), toolCall.Function.Name)
				result, err := handler.ExecuteTool(toolCall)
				if err != nil {
					result = fmt.Sprintf("Erro ao executar ferramenta: %v", err)
				}
				results[idx] = indexedResult{index: idx, toolCallID: toolCall.ID, result: result}
			}(i, tc)
		}
		wg.Wait()

		// Adiciona resultados na ordem original
		for _, r := range results {
			toolResults = append(toolResults, r.result)
			newMessages = append(newMessages, Message{
				Role:       "tool",
				ToolCallID: r.toolCallID,
				Content:    StrPtr(r.result),
			})
		}
	}

	handler.OnToolResults(toolResults, usage, model)

	// Continua a conversa
	streamChatWithTools(ctx, cfg, newMessages, params, handler, depth+1)
}

// ProcessImagesWithAuxiliaryModel processa imagens usando modelo auxiliar
func ProcessImagesWithAuxiliaryModel(messages []Message, imageModel string, handler StreamHandler) []Message {
	result := make([]Message, len(messages))
	copy(result, messages)

	for i, msg := range result {
		parts, ok := msg.Content.([]interface{})
		if !ok {
			continue
		}

		var newParts []interface{}
		var imageDescriptions []string

		for _, part := range parts {
			partMap, ok := part.(map[string]interface{})
			if !ok {
				newParts = append(newParts, part)
				continue
			}

			if partMap["type"] == "image_url" {
				if imgUrl, ok := partMap["image_url"].(map[string]interface{}); ok {
					if url, ok := imgUrl["url"].(string); ok {
						desc, err := handler.GenerateImageDescription(url, imageModel)
						if err != nil {
							fmt.Printf("⚠️ [VISION] Erro ao descrever imagem: %v\n", err)
							desc = "[Imagem não pôde ser processada]"
						} else {
							fmt.Printf("✅ [VISION] Imagem descrita: %s\n", desc)
							handler.SetVisionSupport(imageModel, true)
						}
						imageDescriptions = append(imageDescriptions, desc)
					}
				}
			} else {
				newParts = append(newParts, part)
			}
		}

		// Adiciona descrições como texto
		if len(imageDescriptions) > 0 {
			descText := "[Descrição das imagens anexadas: " + strings.Join(imageDescriptions, "; ") + "]"
			newParts = append(newParts, map[string]interface{}{
				"type": "text",
				"text": descText,
			})
		}

		// Simplifica se só sobrou texto
		if len(newParts) == 1 {
			if textPart, ok := newParts[0].(map[string]interface{}); ok {
				if textPart["type"] == "text" {
					result[i].Content = textPart["text"]
					continue
				}
			}
		}

		// Concatena tudo como texto
		var textContent strings.Builder
		for _, part := range newParts {
			if partMap, ok := part.(map[string]interface{}); ok {
				if partMap["type"] == "text" {
					if textContent.Len() > 0 {
						textContent.WriteString("\n")
					}
					textContent.WriteString(fmt.Sprintf("%v", partMap["text"]))
				}
			}
		}
		result[i].Content = textContent.String()
	}

	return result
}

// GenerateImageDescription gera uma descrição para uma imagem
func GenerateImageDescription(cfg *config.Config, imageBase64 string, model string, getModels func() ([]string, error)) (string, error) {
	// Se não especificou modelo, tenta encontrar um com visão
	if model == "" {
		visionModels := []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "claude-3-5-sonnet", "claude-3-opus", "gemini-1.5-pro", "gemini-1.5-flash"}
		models, _ := getModels()
		for _, vm := range visionModels {
			for _, m := range models {
				if strings.Contains(strings.ToLower(m), strings.ToLower(vm)) {
					model = m
					break
				}
			}
			if model != "" {
				break
			}
		}
		if model == "" {
			return "", fmt.Errorf("nenhum modelo com suporte a visão encontrado")
		}
	}

	messages := []Message{
		{
			Role: "user",
			Content: []ContentPart{
				{
					Type: "text",
					Text: "Descreva esta imagem de forma concisa em uma frase curta para ser usada como texto alternativo (alt text) para acessibilidade. Responda apenas com a descrição, sem explicações adicionais.",
				},
				{
					Type: "image_url",
					ImageURL: &ImageURL{
						URL:    imageBase64,
						Detail: "low",
					},
				},
			},
		},
	}

	requestBody := ChatRequest{
		Model:       model,
		Messages:    messages,
		MaxTokens:   100,
		Temperature: 0.3,
		Stream:      false,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("erro ao serializar requisição: %v", err)
	}

	req, err := http.NewRequest("POST", cfg.APIBaseURL+"/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("erro ao criar requisição: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := SharedHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro na requisição: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("erro da API (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("erro ao decodificar resposta: %v", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("nenhuma resposta do modelo")
	}

	description := strings.TrimSpace(result.Choices[0].Message.Content)
	return description, nil
}
