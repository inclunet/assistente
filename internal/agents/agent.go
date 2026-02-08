package agents

import (
	"context"

	"assistente/internal/llm"
	"assistente/internal/mcp"
)

// Re-exporta tipos de llm para compatibilidade com código existente
type Tool = llm.Tool
type ToolFunction = llm.ToolFunction
type ToolCall = llm.ToolCall
type FunctionCall = llm.FunctionCall

// Re-exporta tipos de mcp para compatibilidade
type MCPTool = mcp.Tool
type MCPToolResult = mcp.ToolResult
type MCPContent = mcp.Content
type MCPServerInfo = mcp.ServerInfo
type MCPTransport = mcp.Transport
type MCPReconnectable = mcp.Reconnectable
type MCPSamplingMessage = mcp.SamplingMessage
type MCPSamplingRequest = mcp.SamplingRequest
type MCPSamplingResult = mcp.SamplingResult
type MCPResource = mcp.Resource
type MCPResourceTemplate = mcp.ResourceTemplate
type MCPResourceContents = mcp.ResourceContents
type MCPPrompt = mcp.Prompt
type MCPPromptResult = mcp.PromptResult

// LLMClient interface para chamar modelos LLM
type LLMClient interface {
	// ChatWithTools envia mensagens para o LLM e processa tool calls
	// Retorna a resposta final após executar todas as tools necessárias
	ChatWithTools(ctx context.Context, model, systemPrompt string, userMessage string, tools []llm.Tool, toolExecutor func(llm.ToolCall) (string, error)) (string, error)

	// ChatWithToolsAndSaver é como ChatWithTools mas salva mensagens internas via callback
	ChatWithToolsAndSaver(ctx context.Context, model, systemPrompt string, userMessage string, tools []llm.Tool, toolExecutor func(llm.ToolCall) (string, error), agentName string, saver llm.MessageSaver) (string, error)
}

// MCPNativeLLMClient interface para LLMs que suportam MCP nativamente
// Implementado por clientes que podem passar tools MCP diretamente sem conversão
type MCPNativeLLMClient interface {
	LLMClient

	// ChatWithMCPTools envia uma mensagem com tools no formato MCP nativo
	// toolExecutor recebe o nome da tool e argumentos já parseados
	ChatWithMCPTools(ctx context.Context, model, systemPrompt string, userMessage string, tools []mcp.Tool, toolExecutor func(name string, args map[string]interface{}) (string, error)) (string, error)
}

// Agent define a interface para agentes inteligentes
type Agent interface {
	// Identificação
	GetName() string        // Identificador único (ex: "faq", "memory")
	GetDisplayName() string // Nome para exibição (ex: "FAQ Manager")
	GetDescription() string // Descrição para o orquestrador saber quando usar
	GetType() string        // Tipo: "internal", "http", "mcp"

	// Configuração (getters)
	GetModel() string        // Modelo LLM do agente
	GetSystemPrompt() string // System prompt especializado
	IsEnabled() bool         // Se o agente está habilitado

	// Configuração (setters - para aplicar configs do banco)
	SetModel(model string)
	SetSystemPrompt(prompt string)
	SetDisplayName(name string)
	SetDescription(desc string)
	SetEnabled(enabled bool)

	// Execução inteligente
	// Recebe tarefa em linguagem natural, usa seu LLM para decidir
	// quais tools usar, executa, e retorna resultado formatado
	Execute(ctx context.Context, task string) (string, error)

	// Tools internas (usadas pelo LLM do agente, não expostas ao orquestrador)
	GetTools() []Tool
	CanHandle(toolName string) bool // Verifica se pode executar a tool (legado)
	ExecuteTool(toolCall ToolCall) (string, error)
}

// ConversationContextSetter é uma interface para agentes que suportam salvar mensagens internas
// Todos os agentes que herdam BaseAgent automaticamente implementam essa interface
type ConversationContextSetter interface {
	SetConversationContext(conversationID uint, saver llm.MessageSaver)
}

// DelegationDescriptionProvider é uma interface opcional para agentes que fornecem
// descrições otimizadas para o orquestrador decidir quando delegar.
// Agentes que não implementam usam GetDescription() padrão.
type DelegationDescriptionProvider interface {
	// GetDelegationDescription retorna descrição formatada com:
	// - CAPABILITIES: o que o agente pode fazer
	// - DELEGATE WHEN: critérios para o orquestrador delegar
	// - DO NOT DELEGATE: quando não delegar
	GetDelegationDescription() string
}

// BaseAgent fornece implementação base para campos comuns
type BaseAgent struct {
	Name           string
	DisplayName    string
	Description    string
	AgentType      string
	Model          string
	SystemPrompt   string
	Enabled        bool
	LLM            LLMClient        // Cliente LLM para o agente usar
	ConversationID uint             // ID da conversa atual (para salvar mensagens internas)
	MessageSaver   llm.MessageSaver // Callback para salvar mensagens internas
}

// SetConversationContext define o contexto da conversa para salvar mensagens internas
func (b *BaseAgent) SetConversationContext(conversationID uint, saver llm.MessageSaver) {
	b.ConversationID = conversationID
	b.MessageSaver = saver
}

func (b *BaseAgent) GetName() string {
	return b.Name
}

func (b *BaseAgent) GetDisplayName() string {
	return b.DisplayName
}

func (b *BaseAgent) GetDescription() string {
	return b.Description
}

func (b *BaseAgent) GetType() string {
	return b.AgentType
}

func (b *BaseAgent) GetModel() string {
	return b.Model
}

func (b *BaseAgent) GetSystemPrompt() string {
	return b.SystemPrompt
}

func (b *BaseAgent) IsEnabled() bool {
	return b.Enabled
}

func (b *BaseAgent) SetModel(model string) {
	b.Model = model
}

func (b *BaseAgent) SetSystemPrompt(prompt string) {
	b.SystemPrompt = prompt
}

func (b *BaseAgent) SetDisplayName(name string) {
	b.DisplayName = name
}

func (b *BaseAgent) SetDescription(desc string) {
	b.Description = desc
}

func (b *BaseAgent) SetEnabled(enabled bool) {
	b.Enabled = enabled
}

// truncate trunca uma string para exibição em logs
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
