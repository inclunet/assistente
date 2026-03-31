package llm

import (
	"context"

	"assistente/internal/credentials"
)

// MCPServerConfig descreve um MCP server HTTP remoto para resolução nativa server-side.
type MCPServerConfig struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	APIKeyName string `json:"api_key_name,omitempty"` // header name para autenticação
	APIKey     string `json:"api_key,omitempty"`
}

// ChatProvider abstrai a comunicação com LLMs via SDKs oficiais.
// Cada implementação (OpenAI, Anthropic, Google) encapsula o SDK correspondente.
type ChatProvider interface {
	// StreamChat envia mensagens com streaming e executa handler para cada evento.
	StreamChat(ctx context.Context, messages []Message, params ChatParams,
		handler StreamHandler, tools ...ToolDefinition)

	// SupportsNativeMCP indica se este provider suporta MCP connector nativo.
	SupportsNativeMCP() bool

	// WithMCPServers retorna uma cópia do provider configurada com MCP servers
	// para resolução nativa server-side.
	WithMCPServers(servers []MCPServerConfig) ChatProvider
}

// NewChatProvider cria o ChatProvider adequado baseado no api_format do provider.
func NewChatProvider(provider *ProviderConfig, credMgr *credentials.Manager) ChatProvider {
	switch provider.GetAPIFormat() {
	case APIFormatAnthropic:
		return NewAnthropicProvider(provider, credMgr)
	case APIFormatGoogle:
		// TODO: Fase 3
		return NewOpenAIProvider(provider, credMgr)
	default:
		return NewOpenAIProvider(provider, credMgr)
	}
}
