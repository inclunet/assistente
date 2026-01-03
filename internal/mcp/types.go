package mcp

import "encoding/json"

// JSONRPCRequest representa uma requisição JSON-RPC 2.0
type JSONRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      *int64      `json:"id,omitempty"` // Ponteiro para omitir em notificações
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCNotification representa uma notificação JSON-RPC 2.0 (sem ID, sem resposta)
type JSONRPCNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// JSONRPCResponse representa uma resposta JSON-RPC 2.0
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"` // Ponteiro para detectar notificações (sem ID)
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError representa um erro JSON-RPC
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ServerInfo contém informações sobre o servidor MCP
type ServerInfo struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	ProtocolVersion string `json:"protocolVersion"`
}

// Tool representa uma ferramenta disponível no servidor MCP
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolResult representa o resultado de uma chamada de tool
type ToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content representa um item de conteúdo MCP
type Content struct {
	Type     string           `json:"type"` // "text", "image", "resource"
	Text     string           `json:"text,omitempty"`
	MimeType string           `json:"mimeType,omitempty"`
	Data     string           `json:"data,omitempty"`     // base64 para imagens
	Resource *ResourceContent `json:"resource,omitempty"` // Para tipo "resource"
}

// ResourceContent representa conteúdo de resource embutido
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // base64
}

// ==================== Resources ====================

// Resource representa um recurso disponível no servidor MCP
type Resource struct {
	URI         string `json:"uri"`  // URI único do recurso
	Name        string `json:"name"` // Nome para exibição
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceTemplate representa um template de recurso
type ResourceTemplate struct {
	URITemplate string `json:"uriTemplate"` // Template URI com placeholders
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourceContents representa o conteúdo de um recurso lido
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // base64 para binários
}

// ==================== Prompts ====================

// Prompt representa um prompt disponível no servidor MCP
type Prompt struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Arguments   []PromptArg `json:"arguments,omitempty"`
}

// PromptArg representa um argumento de prompt
type PromptArg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// PromptMessage representa uma mensagem em um prompt expandido
type PromptMessage struct {
	Role    string  `json:"role"` // "user", "assistant"
	Content Content `json:"content"`
}

// PromptResult representa o resultado de prompts/get
type PromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages"`
}

// ==================== Sampling ====================

// SamplingMessage representa uma mensagem para sampling
type SamplingMessage struct {
	Role    string  `json:"role"` // "user", "assistant"
	Content Content `json:"content"`
}

// SamplingRequest representa uma requisição de sampling
type SamplingRequest struct {
	Messages         []SamplingMessage      `json:"messages"`
	ModelPreferences *ModelPreferences      `json:"modelPreferences,omitempty"`
	SystemPrompt     string                 `json:"systemPrompt,omitempty"`
	IncludeContext   string                 `json:"includeContext,omitempty"` // "none", "thisServer", "allServers"
	Temperature      *float64               `json:"temperature,omitempty"`
	MaxTokens        int                    `json:"maxTokens"`
	StopSequences    []string               `json:"stopSequences,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// ModelPreferences define preferências de modelo para sampling
type ModelPreferences struct {
	Hints                []ModelHint `json:"hints,omitempty"`
	CostPriority         *float64    `json:"costPriority,omitempty"`         // 0-1
	SpeedPriority        *float64    `json:"speedPriority,omitempty"`        // 0-1
	IntelligencePriority *float64    `json:"intelligencePriority,omitempty"` // 0-1
}

// ModelHint é uma dica sobre qual modelo preferir
type ModelHint struct {
	Name string `json:"name,omitempty"`
}

// SamplingResult representa o resultado de sampling/createMessage
type SamplingResult struct {
	Role       string  `json:"role"`
	Content    Content `json:"content"`
	Model      string  `json:"model"`
	StopReason string  `json:"stopReason,omitempty"` // "endTurn", "stopSequence", "maxTokens"
}

// ==================== Transport Interface ====================

// Transport define a interface para transportes MCP (stdio, HTTP/SSE)
type Transport interface {
	// Connect conecta ao servidor MCP
	Connect() error

	// Close encerra a conexão
	Close() error

	// IsConnected verifica se está conectado
	IsConnected() bool

	// GetTools retorna as ferramentas disponíveis
	GetTools() []Tool

	// GetServerInfo retorna informações do servidor
	GetServerInfo() *ServerInfo

	// CallTool executa uma ferramenta
	CallTool(name string, arguments map[string]interface{}) (*ToolResult, error)

	// ==================== Resources ====================

	// ListResources retorna os recursos disponíveis
	ListResources() ([]Resource, error)

	// ListResourceTemplates retorna os templates de recursos
	ListResourceTemplates() ([]ResourceTemplate, error)

	// ReadResource lê o conteúdo de um recurso
	ReadResource(uri string) (*ResourceContents, error)

	// ==================== Prompts ====================

	// ListPrompts retorna os prompts disponíveis
	ListPrompts() ([]Prompt, error)

	// GetPrompt obtém um prompt expandido com argumentos
	GetPrompt(name string, arguments map[string]string) (*PromptResult, error)

	// ==================== Sampling ====================

	// CreateMessage solicita ao servidor que crie uma mensagem via LLM
	CreateMessage(request *SamplingRequest) (*SamplingResult, error)
}

// Reconnectable define interface opcional para transportes que suportam reconexão
type Reconnectable interface {
	Transport
	// Ping verifica se a conexão ainda está ativa
	Ping() error
	// Reconnect tenta reconectar ao servidor
	Reconnect() error
}






