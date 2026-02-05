package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"assistente/internal/llm"
)

// WebSearchAgent é um agente para busca na web usando modelos de busca da OpenAI
// Usa modelos especializados como gpt-4o-search-preview ou gpt-5-search-api
type WebSearchAgent struct {
	BaseAgent
	APIKey     string
	APIBaseURL string
}

// WebSearchAgentConfig configuração para criar o agente de busca
type WebSearchAgentConfig struct {
	APIKey     string
	APIBaseURL string
	Model      string // "gpt-4o-search-preview", "gpt-4o-mini-search-preview", "gpt-5-search-api"
}

// NewWebSearchAgent cria um novo agente de busca web
func NewWebSearchAgent(llmClient LLMClient, cfg WebSearchAgentConfig) *WebSearchAgent {
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://api.openai.com/v1"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-search-preview"
	}

	return &WebSearchAgent{
		BaseAgent: BaseAgent{
			Name:         "web_search",
			DisplayName:  "Web Search",
			Description:  webSearchAgentDescription(),
			AgentType:    "internal",
			Model:        cfg.Model,
			SystemPrompt: webSearchAgentSystemPrompt(),
			Enabled:      true,
			LLM:          llmClient,
		},
		APIKey:     cfg.APIKey,
		APIBaseURL: cfg.APIBaseURL,
	}
}

// webSearchAgentDescription retorna a descrição para delegação do orquestrador
func webSearchAgentDescription() string {
	return NewDelegationDescription("Web Search", "Searches the web for current information using OpenAI's search-enabled models.").
		Capabilities(
			"Search the web for up-to-date information",
			"Find current news, events, and recent developments",
			"Look up real-time data (weather, stocks, sports scores)",
			"Research topics with cited sources",
			"Answer questions requiring current information",
		).
		DelegateWhen(
			"User asks about current events or recent news",
			"User needs up-to-date information not in training data",
			"User asks 'search for', 'look up', 'find online', 'what's the latest'",
			"Question requires real-time data (weather, prices, scores)",
			"User wants to verify facts from current sources",
			"User asks about recent releases, updates, or announcements",
		).
		DontDelegateWhen(
			"User wants to navigate to a specific URL (use Web Navigator)",
			"User wants to interact with a website (use Web Navigator)",
			"Information is likely in FAQs or memories",
			"Question is about general knowledge that doesn't need current data",
			"User is asking about local files (use File Manager)",
		).
		Build()
}

// webSearchAgentSystemPrompt retorna o system prompt do agente
func webSearchAgentSystemPrompt() string {
	return `You are a web search specialist. Your role is to find current, accurate information from the web.

## GUIDELINES:

1. **Be Specific**: When searching, be precise about what information is needed.

2. **Cite Sources**: Always include sources/URLs when providing information from search results.

3. **Current Information**: Focus on the most recent and relevant information.

4. **Accuracy**: Cross-reference information when possible. If uncertain, say so.

5. **Summarize**: Present information in a clear, organized manner.

## RESPONSE FORMAT:

- Start with a direct answer to the question
- Include relevant details and context
- Always cite sources with URLs
- Mention the date/recency of information when relevant
- If information is conflicting or uncertain, explain the discrepancy

## LANGUAGE:

Respond in the same language as the user's question.`
}

// GetDelegationDescription retorna descrição otimizada para o orquestrador
func (a *WebSearchAgent) GetDelegationDescription() string {
	return a.Description
}

// GetTools retorna as ferramentas do agente
// O WebSearchAgent não usa tools internas - ele usa diretamente um modelo de busca
func (a *WebSearchAgent) GetTools() []Tool {
	return []Tool{}
}

// CanHandle verifica se o agente pode executar uma ferramenta
func (a *WebSearchAgent) CanHandle(toolName string) bool {
	return toolName == "web_search"
}

// ExecuteTool executa uma ferramenta (não usado neste agente)
func (a *WebSearchAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	return "", fmt.Errorf("WebSearchAgent não usa tools internas")
}

// Execute executa uma busca na web
// A busca é feita diretamente chamando um modelo de busca da OpenAI
func (a *WebSearchAgent) Execute(ctx context.Context, task string) (string, error) {
	log.Printf("🔍 [WEB_SEARCH] Execute chamado com task: %s", task)
	log.Printf("🔍 [WEB_SEARCH] Usando modelo: %s", a.Model)

	if a.APIKey == "" {
		return "", fmt.Errorf("API key não configurada para WebSearchAgent")
	}

	// Chama diretamente o modelo de busca da OpenAI
	result, err := a.callSearchModel(ctx, task)
	if err != nil {
		log.Printf("🔍 [WEB_SEARCH] Erro na busca: %v", err)
		return "", fmt.Errorf("erro na busca web: %w", err)
	}

	log.Printf("🔍 [WEB_SEARCH] Busca concluída com sucesso")
	return result, nil
}

// callSearchModel faz uma chamada ao modelo de busca da OpenAI
func (a *WebSearchAgent) callSearchModel(ctx context.Context, query string) (string, error) {
	// Prepara a requisição
	messages := []llm.Message{
		{
			Role:    "system",
			Content: a.SystemPrompt,
		},
		{
			Role:    "user",
			Content: query,
		},
	}

	reqBody := llm.ChatRequest{
		Model:    a.Model,
		Messages: messages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("erro ao serializar requisição: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(a.APIBaseURL, "/"))
	log.Printf("🔍 [WEB_SEARCH] Chamando: %s", url)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("erro ao criar request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.APIKey)

	resp, err := llm.SharedHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro na requisição HTTP: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("🔍 [WEB_SEARCH] Erro da API: %s", string(body))
		return "", fmt.Errorf("API retornou status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp llm.ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("resposta vazia do modelo de busca")
	}

	// Extrai o texto da resposta
	content := chatResp.Choices[0].Message.Content
	if contentStr, ok := content.(string); ok {
		return contentStr, nil
	}

	// Se o content for um array (multimodal), tenta extrair texto
	if contentArr, ok := content.([]interface{}); ok {
		var textParts []string
		for _, part := range contentArr {
			if partMap, ok := part.(map[string]interface{}); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						textParts = append(textParts, text)
					}
				}
			}
		}
		if len(textParts) > 0 {
			return strings.Join(textParts, "\n"), nil
		}
	}

	return "", fmt.Errorf("formato de resposta inesperado")
}

// Verifica que WebSearchAgent implementa Agent
var _ Agent = (*WebSearchAgent)(nil)
