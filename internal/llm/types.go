package llm

import (
	"fmt"
	"strings"
)

// ==================== Message Types ====================

// Message representa uma mensagem no histórico do chat
type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content,omitempty"` // Pode ser string ou []ContentPart
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
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

// ==================== Tool Types ====================

// ToolCall representa uma chamada de ferramenta
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall representa a chamada de uma função
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool representa uma ferramenta disponível para o modelo
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction representa a definição de uma função
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ==================== Request/Response Types ====================

// StreamOptions para incluir uso de tokens no streaming
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// ChatRequest representa a requisição para a API da OpenAI
type ChatRequest struct {
	Model         string         `json:"model"`
	Messages      []Message      `json:"messages"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Temperature   float64        `json:"temperature,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`
	Tools         []Tool         `json:"tools,omitempty"`
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
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
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
	Content          string     `json:"content"`
	Done             bool       `json:"done"`
	Error            string     `json:"error,omitempty"`
	FullResponse     string     `json:"fullResponse,omitempty"`
	PromptTokens     int        `json:"promptTokens,omitempty"`
	CompletionTokens int        `json:"completionTokens,omitempty"`
	TotalTokens      int        `json:"totalTokens,omitempty"`
	Model            string     `json:"model,omitempty"`
	ToolCalls        []ToolCall `json:"toolCalls,omitempty"`
	ToolResults      []string   `json:"toolResults,omitempty"`
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
	Model       string  `json:"model"`
	MaxTokens   int     `json:"maxTokens"`
	Temperature float64 `json:"temperature"`
	UseTools    bool    `json:"useTools"`
}

// SettingsInput representa os parâmetros de entrada para salvar configurações
// Nota: Configurações de voz TTS foram movidas para VoiceProfiles no banco de dados
type SettingsInput struct {
	APIKey           string           `json:"api_key"`
	APIBaseURL       string           `json:"api_base_url"`
	BraveAPIKey      string           `json:"brave_api_key,omitempty"`
	ChatParams       ModelParams      `json:"chat_params"`
	EmbeddingsParams EmbeddingsParams `json:"embeddings_params"`
	ImageModel       string           `json:"image_model,omitempty"`
	STTParams        STTParams        `json:"stt_params,omitempty"`
	ChatDefaults     ChatDefaults     `json:"chat_defaults,omitempty"`
}

// ModelParams representa parâmetros do modelo de chat
type ModelParams struct {
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	TopP        float64 `json:"top_p,omitempty"`
}

// EmbeddingsParams representa parâmetros do modelo de embeddings
type EmbeddingsParams struct {
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions,omitempty"`
}

// STTParams representa parâmetros de transcrição
type STTParams struct {
	Provider      string `json:"provider,omitempty"`
	RecordingMode string `json:"recording_mode,omitempty"`
}

// ChatDefaults representa preferências padrão do chat
type ChatDefaults struct {
	UseTools             bool `json:"use_tools,omitempty"`
	ShowInternalMessages bool `json:"show_internal_messages,omitempty"`
}

// ==================== Helper Functions ====================

// StrPtr retorna um ponteiro para uma string
func StrPtr(s string) *string {
	return &s
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
