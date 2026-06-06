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
	Slug         string                      `json:"slug,omitempty"`
	Name         string                      `json:"name"`
	URL          string                      `json:"url"`
	AuthToken    string                      `json:"auth_token,omitempty"`
	Headers      map[string]string           `json:"headers,omitempty"`
	ToolNames    []string                    `json:"tool_names,omitempty"`    // todas as tools do server (namespaced, para dedup bridge)
	AllowedTools []string                    `json:"allowed_tools,omitempty"` // tools permitidas pelo perfil (nomes originais do MCP server); vazio = todas
	Recover      func(context.Context) error `json:"-"`
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

	// SupportsNativeMCP indica o DEFAULT (auto) de MCP nativo deste provider quando
	// o perfil não força nada. É a heurística segura por endpoint (ex.: OpenAI real
	// por api.openai.com). Um perfil pode sobrescrever esse default (ver AEP-0021).
	SupportsNativeMCP() bool

	// NativeMCPCapable indica se o provider/transport é FISICAMENTE capaz de emitir
	// MCP nativo (independentemente da heurística de URL ou de política de perfil).
	// Ex.: OpenAI via Responses API e Anthropic são capazes; Chat Completions e
	// Google não. Um override de perfil "true" só habilita MCP nativo quando o
	// provider é capaz — evita remover bridge tools sem ter como enviar type:"mcp".
	NativeMCPCapable() bool

	// WithMCPServers retorna uma cópia do provider configurada com MCP servers HTTP remotos,
	// incorporando-os na request ao LLM quando o provider é FISICAMENTE capaz
	// (NativeMCPCapable() == true): Anthropic mcp_servers, OpenAI Responses type:mcp.
	// Quando o provider não é capaz, retorna o provider inalterado (no-op) e os servers
	// devem usar adapter/bridge.
	//
	// O gate aqui é apenas de capacidade de TRANSPORTE. A POLÍTICA de usar nativo vs
	// adapter NÃO é decidida pelo provider: é resolvida na camada de chat por
	// ResolveNativeMCPEnabled, que combina NativeMCPCapable() + override do perfil
	// (Profile.Chat.NativeMCP) + o default por endpoint (SupportsNativeMCP()). A camada
	// de chat só chama WithMCPServers quando essa política resolve para nativo.
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
