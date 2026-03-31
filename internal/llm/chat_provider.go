package llm

import (
	"context"

	"assistente/internal/credentials"
)

// Streamer é a interface mínima para streaming de chat.
// Tanto *Client (legado, HTTP raw) quanto ChatProvider (SDKs oficiais) a satisfazem.
// Usado como parâmetro de runAgenticLoop e no routing de sendMessageInternal.
type Streamer interface {
	StreamChat(ctx context.Context, messages []Message, params ChatParams,
		handler StreamHandler, tools ...ToolDefinition)
}

// MCPServerConfig descreve um MCP server HTTP remoto para resolução nativa server-side.
type MCPServerConfig struct {
	Label      string `json:"label"`
	URL        string `json:"url"`
	APIKeyName string `json:"api_key_name,omitempty"` // header name para autenticação
	APIKey     string `json:"api_key,omitempty"`
}

// ChatProvider abstrai a comunicação com LLMs via SDKs oficiais.
// Cada implementação (OpenAI, Anthropic, Google) encapsula o SDK correspondente.
// ChatProvider é um superset de Streamer com suporte adicional a MCP.
type ChatProvider interface {
	Streamer

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
		return NewGoogleProvider(provider, credMgr)
	default:
		return NewOpenAIProvider(provider, credMgr)
	}
}
