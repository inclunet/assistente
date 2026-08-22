package llm

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ==================== Message Types ====================

// Message representa uma mensagem no histórico do chat
type Message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content,omitempty"`      // Pode ser string ou []ContentPart
	Thinking   string      `json:"thinking,omitempty"`     // Ollama thinking/reasoning
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`   // Tool calls solicitadas pelo assistant
	ToolCallID string      `json:"tool_call_id,omitempty"` // Para role="tool": vincula ao call
	// MessageID e TurnContextTarget são metadados internos da request LLM.
	// Eles não são enviados ao provider nem persistidos; servem para associar
	// contexto transitório ao turno correto mesmo em retries/branches.
	MessageID         string `json:"-"`
	TurnContextTarget bool   `json:"-"`
	// SystemCacheControlPrefixLen marca quantos bytes iniciais do system prompt
	// formam o prefixo estável elegível a cache_control explícito. É metadado
	// interno do backend e só providers com suporte físico devem consumi-lo.
	SystemCacheControlPrefixLen int `json:"-"`
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

// EnrichedToolCall estende ToolCall com metadata de execução para persistência no DB (AEP-0039 Fase 5).
// Usado apenas na serialização para o banco; ToolCall regular é usado nas chamadas à API do LLM.
type EnrichedToolCall struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"`
	Function    FunctionCall `json:"function"`
	Result      string       `json:"result,omitempty"`       // Resultado compacto para export/histórico
	Origin      string       `json:"origin,omitempty"`       // "builtin" | "mcp_bridge" | "mcp_native"
	ServerLabel string       `json:"server_label,omitempty"` // Label do servidor MCP
	Iteration   int          `json:"iteration"`              // Iteração do agentic loop (0-based)
	DurationMs  int64        `json:"duration_ms,omitempty"`  // Duração da execução em milissegundos
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
	case []ContentPart:
		// Partes tipadas: é assim que o builder monta a mensagem do turno
		// quando ela é multimodal. Sem este caso, o texto sairia pelo
		// fmt.Sprintf do default, como despejo da estrutura.
		var texts []string
		for _, part := range v {
			if part.Type == "text" && part.Text != "" {
				texts = append(texts, part.Text)
			}
		}
		return strings.Join(texts, "\n")
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
	CacheReadTokens  int `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
	CacheMissTokens  int `json:"cache_miss_tokens,omitempty"`
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
	CacheReadTokens  int    `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int    `json:"cacheWriteTokens,omitempty"`
	CacheMissTokens  int    `json:"cacheMissTokens,omitempty"`
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
	Model           string  `json:"model"`
	MaxTokens       int     `json:"maxTokens"`
	MaxTokensMode   string  `json:"maxTokensMode,omitempty"` // "legacy" (max_tokens) ou "completion_tokens" (max_completion_tokens)
	Temperature     float64 `json:"temperature"`
	TopP            float64 `json:"topP,omitempty"`
	ReasoningEffort string  `json:"reasoningEffort,omitempty"` // off, low, medium, high
	ProfileSlug     string  `json:"profileSlug,omitempty"`     // Perfil específico (canais). Vazio = perfil ativo global
	// ConversationID é a conversa do turno. Provider HTTP não precisa dele —
	// o histórico vai na request —, mas o agente ACP guarda o histórico na
	// sessão da conversa, e é por aqui que ele encontra qual é (AEP-0084 D4).
	// Metadado interno do backend, como os demais campos sem serialização.
	ConversationID string `json:"-"`
	// MaxContextMessages limita mensagens carregadas no histórico deste turno.
	// 0 = usar o valor do perfil (GetMaxContextMessages). Usado por canais via
	// ChannelConfig.max_history.
	MaxContextMessages int `json:"maxContextMessages,omitempty"`
	// AllowAssistantPrefill habilita a continuação explícita de resposta via trailing assistant.
	// Default: false (AEP-0064 Fase 6). Só deve ser usado em retry explícito.
	AllowAssistantPrefill bool `json:"allowAssistantPrefill,omitempty"`
	// ContinueViaUserMessage habilita o fallback de continuação para providers/modelos
	// que NÃO suportam assistant prefill (Issue #124). Em vez de injetar um trailing
	// assistant, a continuação é montada como uma mensagem de usuário do tipo
	// "continue a partir deste texto: ...". É definido pelo backend (use case) quando
	// a continuação está habilitada no perfil mas o provider não suporta prefill.
	// Mutuamente exclusivo com AllowAssistantPrefill.
	ContinueViaUserMessage bool            `json:"continueViaUserMessage,omitempty"`
	MaxAgenticIterations   int             `json:"maxAgenticIterations,omitempty"` // 0 = usar default (25), >0 = limite customizado
	ResponseTimeout        int             `json:"responseTimeout,omitempty"`      // Timeout em segundos (2ª camada de proteção)
	RateLimitEnabled       *bool           `json:"-"`                              // Política de rate limit resolvida do perfil; nil usa o default.
	RateLimitRPM           int             `json:"-"`                              // Taxa sustentada resolvida do perfil.
	RateLimitBurst         int             `json:"-"`                              // Rajada instantânea resolvida do perfil.
	ContextWindow          int             `json:"contextWindow,omitempty"`        // Tamanho da janela de contexto do modelo (0 = sem limite). AEP-0039 Fase 4.
	TabType                string          `json:"tabType,omitempty"`              // Tipo da aba de origem ("editor", "chat", etc.)
	ActiveFilePath         string          `json:"activeFilePath,omitempty"`       // Caminho do arquivo ativo (editor tabs)
	SurfaceStateJSON       string          `json:"surfaceStateJson,omitempty"`     // Espelho serializado de WorkspaceTab.state
	SurfaceContextJSON     string          `json:"surfaceContextJson,omitempty"`   // Contexto transitório do envio atual
	SurfaceSessionKey      string          `json:"surfaceSessionKey,omitempty"`    // Identidade explícita da sessão visual que originou o turno
	SurfaceID              string          `json:"surfaceId,omitempty"`            // Identidade estável da superfície de origem
	SurfaceType            string          `json:"surfaceType,omitempty"`          // page | embedded | modal | external
	SurfaceTabID           string          `json:"surfaceTabId,omitempty"`         // Workspace tab que hospeda a superfície, quando existir
	PromptCacheKey         string          `json:"-"`                              // Hint seguro derivado pelo backend para prompt_cache_key
	ExplicitCacheControl   bool            `json:"-"`                              // Permite cache_control explícito em providers compatíveis
	DebugDump              DebugDumpConfig `json:"-"`                              // Dump local de requests/responses LLM para diagnóstico

	// OnNativeMCPUnsupported é um hook opcional, definido pelo backend (use case),
	// chamado quando uma request com MCP nativo falha com erro de não-suporte
	// (ex.: 400 unknown variant "mcp"). Permite à camada superior auto-ajustar e
	// PERSISTIR o perfil para adapter (Profile.Chat.NativeMCP nil→false), sem que o
	// provider conheça a camada de perfis (AEP-0021). Não é serializado.
	OnNativeMCPUnsupported func() `json:"-"`

	// NativeMCPFallback, quando presente, indica que o caller (loop agêntico) é capaz
	// de re-tentar o MESMO turno em modo adapter (MCP como bridge/function tools).
	// Ao detectar o erro de não-suporte a MCP nativo, o provider apenas marca o
	// fallback (Trigger) e aborta sem emitir, deixando o caller re-montar as tools no
	// modo adapter e re-tentar — assim as tools MCP continuam disponíveis já neste
	// turno. Não é serializado. Ver AEP-0021.
	NativeMCPFallback *NativeMCPAdapterFallback `json:"-"`

	// OnPromptCacheHintUnsupported é chamado quando o provider rejeita
	// explicitamente prompt_cache_key. Permite degradar este turno sem o hint e
	// persistir Profile.Chat.PromptCache.ProviderHints=false sem acoplar provider
	// à camada de perfis.
	OnPromptCacheHintUnsupported func() `json:"-"`
	// PromptCacheHintFallback é compartilhado entre cópias de ChatParams durante o
	// mesmo turno/loop agêntico para impedir reenvio do hint após rejeição explícita.
	PromptCacheHintFallback *PromptCacheHintFallback `json:"-"`
}

type DebugDumpConfig struct {
	Enabled       bool
	DumpRequests  bool
	DumpResponses bool
	// MaxFiles retém até N dumps/runs por conversa (diretórios de snapshot), não arquivos individuais.
	// Zero ou negativo usa o default interno.
	MaxFiles       int
	ProfileSlug    string
	ConversationID string
	TurnID         string
}

type PromptCacheHintFallback struct {
	mu       sync.Mutex
	disabled bool
}

func (f *PromptCacheHintFallback) Disable() {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.disabled = true
	f.mu.Unlock()
}

func (f *PromptCacheHintFallback) Disabled() bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.disabled
}

// NativeMCPAdapterFallback carrega as alternativas em modo ADAPTER (provider sem
// MCP servers nativos + tools com bridges) para que o loop agêntico re-tente o
// mesmo turno quando o modelo/endpoint rejeitar MCP nativo. É preenchido pela camada
// de chat (que conhece o registro de tools); o provider apenas sinaliza via Trigger.
type NativeMCPAdapterFallback struct {
	// Streamer é o provider em modo adapter (BASE, sem WithMCPServers nativo).
	Streamer Streamer
	// ToolDefs são as tools iniciais COM as bridge tools MCP (não removidas).
	ToolDefs []ToolDefinition
	// ResolveToolDefs reconstrói o conjunto ACUMULADO de tools em modo adapter
	// (mantém bridges), usado nas iterações seguintes do loop agêntico após
	// expansão de catálogo. Recebe as tools já ATIVAS no turno e os novos nomes
	// selecionados, e devolve o conjunto acumulado final (orçado pelo ToolPlanner).
	ResolveToolDefs func(active []ToolDefinition, names []string) []ToolDefinition

	mu        sync.Mutex
	triggered bool
	consumed  bool
}

// Trigger marca que o fallback nativo→adapter deve ocorrer. Idempotente e thread-safe.
func (f *NativeMCPAdapterFallback) Trigger() {
	if f == nil {
		return
	}
	f.mu.Lock()
	f.triggered = true
	f.mu.Unlock()
}

// Consume retorna true UMA única vez, quando o fallback foi disparado e ainda não
// foi consumido. O caller usa isso para trocar streamer/tools e re-tentar o turno
// exatamente uma vez (evita loop infinito).
func (f *NativeMCPAdapterFallback) Consume() bool {
	if f == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.triggered && !f.consumed {
		f.consumed = true
		return true
	}
	return false
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
