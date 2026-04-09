package llm

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ==================== Message Types ====================

// Message representa uma mensagem no histórico do chat
type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content,omitempty"`      // Pode ser string ou []ContentPart
	Thinking   string      `json:"thinking,omitempty"`     // Ollama thinking/reasoning
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // Tool calls solicitadas pelo assistant
	ToolCallID string      `json:"tool_call_id,omitempty"` // Para role="tool": vincula ao call
}

// ToolCall representa uma chamada de ferramenta solicitada pelo LLM
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // sempre "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall contém nome e argumentos de uma chamada de função
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCallDelta representa um delta incremental de tool_call durante streaming.
// O LLM envia os argumentos em fragmentos que precisam ser acumulados.
type ToolCallDelta struct {
	Index    int            `json:"index"`
	ID       string         `json:"id,omitempty"`
	Type     string         `json:"type,omitempty"`
	Function *FunctionDelta `json:"function,omitempty"`
}

// FunctionDelta representa o delta incremental da função
type FunctionDelta struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ToolDefinition define uma ferramenta para enviar ao LLM no campo "tools"
type ToolDefinition struct {
	Type     string             `json:"type"` // sempre "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition contém a especificação completa de uma função
type FunctionDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ContentPart representa uma parte do conteúdo multimodal
type ContentPart struct {
	Type     string    `json:"type"` // "text" ou "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL representa uma imagem em base64 ou URL
type ImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"` // "auto", "low", "high"
}

// GetContentAsString retorna o conteúdo como string (para compatibilidade)
func (m *Message) GetContentAsString() string {
	if m.Content == nil {
		return ""
	}
	switch v := m.Content.(type) {
	case string:
		return v
	case *string:
		if v == nil {
			return ""
		}
		return *v
	case []interface{}:
		// Concatena apenas as partes de texto
		var texts []string
		for _, part := range v {
			if partMap, ok := part.(map[string]interface{}); ok {
				if partMap["type"] == "text" {
					if text, ok := partMap["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "\n")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ==================== Request/Response Types ====================

// StreamOptions para incluir uso de tokens no streaming
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// ChatRequest representa a requisição para a API da OpenAI
type ChatRequest struct {
	Model               string           `json:"model"`
	Messages            []Message        `json:"messages"`
	MaxTokens           int              `json:"max_tokens,omitempty"`            // Modelos antigos (GPT-3.5, GPT-4, etc)
	MaxCompletionTokens int              `json:"max_completion_tokens,omitempty"` // Modelos novos (GPT-4o, o1, etc)
	Temperature         float64          `json:"temperature,omitempty"`
	TopP                *float64         `json:"top_p,omitempty"` // Ponteiro para omitir quando nil
	Stream              bool             `json:"stream"`
	StreamOptions       *StreamOptions   `json:"stream_options,omitempty"`
	Think               *bool            `json:"think,omitempty"`            // Ollama: habilita reasoning/thinking
	ReasoningEffort     string           `json:"reasoning_effort,omitempty"` // OpenAI/LiteLLM: low, medium, high
	Tools               []ToolDefinition `json:"tools,omitempty"`            // Ferramentas disponíveis para o LLM
	ToolChoice          interface{}      `json:"tool_choice,omitempty"`      // "auto", "none", "required" ou objeto
}

// ChatChoice representa uma escolha na resposta
type ChatChoice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message,omitempty"`
	Delta        Delta   `json:"delta,omitempty"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

// Delta representa um delta no streaming
type Delta struct {
	Role             string          `json:"role,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"` // DeepSeek, Qwen reasoning
	Thinking         string          `json:"thinking,omitempty"`          // Ollama thinking/reasoning
	ToolCalls        []ToolCallDelta `json:"tool_calls,omitempty"`        // Tool calls incrementais
}

// Usage representa o uso de tokens
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse representa a resposta da API
type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage,omitempty"`
}

// StreamChunk representa um chunk do streaming
type StreamChunk struct {
	Content          string `json:"content"`
	Done             bool   `json:"done"`
	Error            string `json:"error,omitempty"`
	FullResponse     string `json:"fullResponse,omitempty"`
	PromptTokens     int    `json:"promptTokens,omitempty"`
	CompletionTokens int    `json:"completionTokens,omitempty"`
	TotalTokens      int    `json:"totalTokens,omitempty"`
	Model            string `json:"model,omitempty"`
}

// Model representa um modelo da API
type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelsResponse representa a resposta do endpoint /models
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// ChatParams contém os parâmetros para uma requisição de chat
type ChatParams struct {
	Model                string  `json:"model"`
	MaxTokens            int     `json:"maxTokens"`
	MaxTokensMode        string  `json:"maxTokensMode,omitempty"` // "legacy" (max_tokens) ou "completion_tokens" (max_completion_tokens)
	Temperature          float64 `json:"temperature"`
	TopP                 float64 `json:"topP,omitempty"`
	ReasoningEffort      string  `json:"reasoningEffort,omitempty"`      // off, low, medium, high
	ProfileSlug          string  `json:"profileSlug,omitempty"`          // Perfil específico (canais). Vazio = perfil ativo global
	MaxAgenticIterations int     `json:"maxAgenticIterations,omitempty"` // 0 = usar default (25), >0 = limite customizado
	ResponseTimeout      int     `json:"responseTimeout,omitempty"`      // Timeout em segundos (2ª camada de proteção)
	TabType              string  `json:"tabType,omitempty"`              // Tipo da aba de origem ("editor", "chat", etc.)
	ActiveFilePath       string  `json:"activeFilePath,omitempty"`       // Caminho do arquivo ativo (editor tabs)
}

// ==================== Helper Functions ====================

// StrPtr retorna um ponteiro para uma string
func StrPtr(s string) *string {
	return &s
}

// BoolPtr retorna um ponteiro para um bool
func BoolPtr(b bool) *bool {
	return &b
}

// BuildEndpoint constrói o endpoint completo a partir da URL base
func BuildEndpoint(baseURL, path string) string {
	// Remove barra final da URL base se existir
	baseURL = strings.TrimSuffix(baseURL, "/")
	// Remove barra inicial do path se existir
	path = strings.TrimPrefix(path, "/")
	return baseURL + "/" + path
}

// HasImages verifica se a lista de mensagens contém imagens
func HasImages(messages []Message) bool {
	for _, msg := range messages {
		if parts, ok := msg.Content.([]interface{}); ok {
			for _, part := range parts {
				if partMap, ok := part.(map[string]interface{}); ok {
					if partMap["type"] == "image_url" {
						return true
					}
				}
			}
		}
	}
	return false
}
