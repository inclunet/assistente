package llm

import (
	"context"

	"assistente/internal/credentials"
)

// Streamer é a interface mínima para streaming de chat.
// ChatProvider (SDKs oficiais) e *Client (legado, fallback) a satisfazem.
// Usado como parâmetro de runAgenticLoop e no routing de sendMessageInternal.
type Streamer interface {
	StreamChat(ctx context.Context, messages []Message, params ChatParams,
		handler StreamHandler, tools ...ToolDefinition)
}

// MCPServerConfig descreve um MCP server HTTP remoto para resolução nativa server-side.
// Usado por ChatProvider.WithMCPServers() para passar servidores ao LLM provider.
type MCPServerConfig struct {
	Name         string            `json:"name"`
	URL          string            `json:"url"`
	AuthToken    string            `json:"auth_token,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	ToolNames    []string          `json:"tool_names,omitempty"`    // todas as tools do server (namespaced, para dedup bridge)
	AllowedTools []string          `json:"allowed_tools,omitempty"` // tools permitidas pelo perfil (nomes originais do MCP server); vazio = todas
}

// ChatProvider abstrai a comunicação com LLMs via SDKs oficiais.
// Cada implementação (OpenAI, Anthropic, Google) encapsula o SDK correspondente.
// ChatProvider é um superset de Streamer com suporte a MCP, chat síncrono e listagem de modelos.
type ChatProvider interface {
	Streamer

	// SendChat envia uma mensagem sem streaming e retorna a resposta completa.
	SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error)

	// GetModels retorna a lista de modelos disponíveis no provider.
	GetModels(ctx context.Context) ([]string, error)

	// SimpleChat é um atalho para enviar system+user e obter a resposta (sem tools).
	SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error)

	// SupportsNativeMCP indica se este provider implementa MCP connector nativo real.
	// Retorna true somente se WithMCPServers produz efeito operacional (altera a request ao LLM).
	SupportsNativeMCP() bool

	// WithMCPServers retorna uma cópia do provider configurada com MCP servers HTTP remotos.
	// Quando SupportsNativeMCP() é true, os servers são incorporados na request ao LLM
	// (ex: Anthropic mcp_servers, OpenAI Responses type:mcp).
	// Quando false, retorna o provider inalterado (os servers devem usar adapter/bridge).
	WithMCPServers(servers []MCPServerConfig) ChatProvider
}

// resolveModel retorna o modelo a usar: param explícito > provider.Model > provider.DefaultModel.
func resolveModel(provider *ProviderConfig, requested string) string {
	if requested != "" {
		return requested
	}
	if provider.Model != "" {
		return provider.Model
	}
	return provider.DefaultModel
}

// NewChatProvider cria o ChatProvider adequado baseado no api_format do provider.
func NewChatProvider(provider *ProviderConfig, credMgr *credentials.Manager) ChatProvider {
	switch provider.GetAPIFormat() {
	case APIFormatAnthropic:
		return NewAnthropicProvider(provider, credMgr)
	case APIFormatGoogle:
		return NewGoogleProvider(provider, credMgr)
	case APIFormatOpenAIResponses:
		return NewOpenAIResponsesProvider(provider, credMgr)
	default:
		return NewOpenAIProvider(provider, credMgr)
	}
}
