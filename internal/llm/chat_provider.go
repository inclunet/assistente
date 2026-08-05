package llm

import (
	"context"

	"assistente/internal/acp"
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

	// NativeMCPCapable indica se o provider/transport é FISICAMENTE capaz de emitir
	// MCP nativo, independentemente de qualquer política. É a ÚNICA dimensão de
	// provider que influencia MCP nativo (não há heurística por URL/endpoint).
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
	// O gate aqui é apenas de capacidade de TRANSPORTE (NativeMCPCapable()). A
	// POLÍTICA de usar nativo vs adapter NÃO é decidida pelo provider: é resolvida
	// na camada de chat por ResolveNativeMCPEnabled, que combina NativeMCPCapable()
	// + o override de perfil (Profile.Chat.NativeMCP). O default (auto, override nil)
	// é OTIMISTA: tenta nativo sempre que o provider for fisicamente capaz, degradando
	// para adapter (e persistindo no perfil) apenas quando o modelo rejeita type:"mcp".
	// A camada de chat só chama WithMCPServers quando a política resolve nativo.
	WithMCPServers(servers []MCPServerConfig) ChatProvider
}

// ModelRefresher é o provedor que guarda a lista de modelos e sabe descartá-la.
// Só faz sentido para quem tem cache: um provedor HTTP pergunta ao servidor a
// cada listagem, enquanto o agente de código responde de uma sessão de
// descoberta guardada por processo (AEP-0084 D6).
type ModelRefresher interface {
	// RefreshModels esquece o que sabia e pergunta de novo.
	RefreshModels(ctx context.Context) ([]string, error)
}

// RefreshModels relista os modelos de um provedor, descartando o que ele tiver
// guardado. Quem não guarda nada é listado normalmente: para ele, "de novo" e
// "agora" são a mesma coisa.
func RefreshModels(ctx context.Context, provider ChatProvider) ([]string, error) {
	if refresher, ok := provider.(ModelRefresher); ok {
		return refresher.RefreshModels(ctx)
	}
	return provider.GetModels(ctx)
}

// ModelOption é um modelo oferecido por um provedor com o nome pelo qual ele
// quer ser chamado na tela.
type ModelOption struct {
	// Value é o que fica gravado no perfil e volta ao provedor.
	Value string `json:"value"`
	// Label é como exibi-lo. Igual ao Value quando o provedor só tem
	// identificadores a oferecer, que é o caso de todo provedor HTTP.
	Label string `json:"label"`
}

// ModelCatalog é a lista de modelos junto com o que a tela precisa saber para
// interpretá-la. Lista vazia quer dizer coisas diferentes conforme quem
// respondeu: de um provedor HTTP é falta de modelo, de um agente de código é
// ele dizendo que a escolha é dele (AEP-0084, Fase 8).
type ModelCatalog struct {
	Models []ModelOption `json:"models"`
	// Agent diz que quem respondeu é um agente de código.
	Agent bool `json:"agent"`
}

// ModelDescriber é o provedor que sabe o nome legível de cada modelo. Um
// provedor HTTP lista identificadores e nada mais; um agente de código oferece
// o rótulo junto, e é ele que a pessoa reconhece (AEP-0084, Fase 8).
type ModelDescriber interface {
	ModelOptions(ctx context.Context) ([]ModelOption, error)
	RefreshModelOptions(ctx context.Context) ([]ModelOption, error)
}

// ModelOptions lista os modelos de um provedor com rótulo. Quem não sabe
// rotular é listado do mesmo jeito, com o identificador fazendo as duas vezes.
func ModelOptions(ctx context.Context, provider ChatProvider) ([]ModelOption, error) {
	if describer, ok := provider.(ModelDescriber); ok {
		return describer.ModelOptions(ctx)
	}
	models, err := provider.GetModels(ctx)
	if err != nil {
		return nil, err
	}
	return modelOptionsOf(models), nil
}

// RefreshModelOptions é ModelOptions descartando o que o provedor tiver
// guardado, para o recarregar da tela.
func RefreshModelOptions(ctx context.Context, provider ChatProvider) ([]ModelOption, error) {
	if describer, ok := provider.(ModelDescriber); ok {
		return describer.RefreshModelOptions(ctx)
	}
	models, err := RefreshModels(ctx, provider)
	if err != nil {
		return nil, err
	}
	return modelOptionsOf(models), nil
}

func modelOptionsOf(models []string) []ModelOption {
	options := make([]ModelOption, 0, len(models))
	for _, model := range models {
		options = append(options, ModelOption{Value: model, Label: model})
	}
	return options
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
//
// agents é o serviço de longa duração dos agentes de código (AEP-0084 D3) e só
// o formato acp o usa. Ele é parâmetro, e não um registro global, porque o
// processo do agente pertence ao serviço: quem constrói um provider de agente
// sem ter o serviço em mãos precisa descobrir isso na chamada, e não num turno
// que falha depois.
func NewChatProvider(provider *ProviderConfig, credMgr *credentials.Manager, agents *acp.Manager) ChatProvider {
	switch provider.GetAPIFormat() {
	case APIFormatACP:
		return NewACPChatProvider(provider, agents)
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
