package llm

import (
	"context"
	"fmt"
	"strings"

	"assistente/internal/credentials"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
)

// OpenAIProvider implementa ChatProvider usando a SDK openai-go.
//
// A implementação é dividida por modo de API em arquivos dedicados do mesmo
// pacote (mantendo a struct única OpenAIProvider):
//
//   - openai_chat_completions.go: Chat Completions API (/v1/chat/completions)
//   - openai_responses.go:        Responses API (/v1/responses)
//   - openai_models.go:           listagem de modelos (SDK + fallback HTTP)
//   - openai_mcp.go:              MCP nativo (tools type:"mcp" e streaming)
//   - openai_convert.go:          conversões compartilhadas (mensagens/tools/etc.)
//
// Dois modos de operação, determinados pelo APIFormat do ProviderConfig:
//
//   - useResponses=false (APIFormatOpenAI / APIFormatOpenAICompatible):
//     Chat Completions API only (/v1/chat/completions).
//     Para provedores OpenAI-compatible: OpenRouter, Ollama, Groq, Together, etc.
//     Não é fisicamente capaz de MCP nativo: NativeMCPCapable() retorna false.
//     WithMCPServers() é no-op (retorna o provider inalterado).
//
//   - useResponses=true (APIFormatOpenAIResponses):
//     Responses API first (/v1/responses).
//     Para OpenAI real (api.openai.com) e proxies que falam Responses (ex.: LiteLLM).
//     Habilita reasoning summaries (via Reasoning param), tool_choice, e features modernas.
//     É FISICAMENTE CAPAZ de emitir tools type:"mcp" — NativeMCPCapable() retorna true
//     (inclusive em proxies). Não há heurística por URL: o default (auto, override nil)
//     tenta MCP nativo sempre que NativeMCPCapable()==true, degradando para adapter
//     (e persistindo no perfil) apenas quando o modelo rejeita type:"mcp". A POLÍTICA
//     final (usar nativo vs adapter) NÃO é decidida aqui: é resolvida na camada de
//     chat por ResolveNativeMCPEnabled, que
//     combina NativeMCPCapable() + override por perfil (Profile.Chat.NativeMCP).
//     WithMCPServers() apenas incorpora os MCP servers na request e gateia por
//     CAPACIDADE FÍSICA (NativeMCPCapable()).
//
// Limitações conhecidas do path Responses vs Chat Completions:
//   - Multimodalidade: imagens em user messages são convertidas como texto.
//     A Responses API suporta imagens mas com formato diferente (input_image).
type OpenAIProvider struct {
	client             *openai.Client
	provider           *ProviderConfig
	credMgr            *credentials.Manager
	useResponses       bool              // true = Responses API first; false = Chat Completions only
	mcpServers         []MCPServerConfig // MCP servers HTTP (só efetivo quando useResponses=true)
	responsesAttemptFn func(context.Context, responses.ResponseNewParams, StreamHandler, []MCPServerConfig, ChatParams, *DebugDumpHandle) mcpStreamAttemptResult
}

// NewOpenAIProvider cria um provider Chat Completions-only (OpenAI-compatible).
// Usado para OpenRouter, Ollama, Groq, Together, e qualquer endpoint /v1/chat/completions.
func NewOpenAIProvider(provider *ProviderConfig, credMgr *credentials.Manager) *OpenAIProvider {
	return newOpenAIProviderBase(provider, credMgr, false)
}

// NewOpenAIResponsesProvider cria um provider Responses API-first (OpenAI real).
// Usa /v1/responses como caminho padrão para streaming/chat.
// Suporta MCP nativo, reasoning summaries, e features modernas da OpenAI.
func NewOpenAIResponsesProvider(provider *ProviderConfig, credMgr *credentials.Manager) *OpenAIProvider {
	return newOpenAIProviderBase(provider, credMgr, true)
}

func newOpenAIProviderBase(provider *ProviderConfig, credMgr *credentials.Manager, useResponses bool) *OpenAIProvider {
	httpClient := newHTTPClientForProvider(provider, credMgr)

	opts := []option.RequestOption{
		option.WithHTTPClient(httpClient),
	}
	if providerUsesPlaceholderAPIKey(provider) {
		opts = append(opts, option.WithAPIKey("managed-by-credential-transport"))
	} else {
		// Provedores AuthModeNone (Ollama, llama.cpp): o SDK
		// openai-go exige uma APIKey; passamos string vazia explicita
		// para que ele não inclua o header Authorization. O transport
		// também remove qualquer placeholder residual como defesa.
		opts = append(opts, option.WithAPIKey(""))
	}

	baseURL := strings.TrimSuffix(provider.BaseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1beta") {
		baseURL += "/"
	} else {
		baseURL += "/"
	}
	opts = append(opts, option.WithBaseURL(baseURL))

	client := openai.NewClient(opts...)

	return &OpenAIProvider{
		client:       &client,
		provider:     provider,
		credMgr:      credMgr,
		useResponses: useResponses,
	}
}

// NativeMCPCapable: a OpenAI só emite tools type:"mcp" pelo caminho da Responses
// API (useResponses=true), independentemente da URL/endpoint. Chat Completions não
// carrega MCP nativo no wire. Esta é a única dimensão de provider que influencia
// MCP nativo; a decisão de USAR nativo é por perfil (ResolveNativeMCPEnabled).
func (p *OpenAIProvider) NativeMCPCapable() bool {
	return p.useResponses
}

// ReplaysReasoningContent informa se o histórico enviado a este provider carrega
// reasoning_content. Só quem replica ocupa janela de contexto com esse campo, e
// o pre-check precisa saber disso para não encolher o budget à toa.
func (p *OpenAIProvider) ReplaysReasoningContent() bool {
	return p.reasoningContentMode() == ReasoningContentReplayWithTools
}

// ReplaysReasoningContent consulta a capacidade em quem quer que seja passado.
// Decorators embrulham o provider sem herdar métodos, então quem pergunta usa
// esta função e cada embrulho a repassa para o inner.
func ReplaysReasoningContent(provider any) bool {
	replayer, ok := provider.(interface{ ReplaysReasoningContent() bool })
	return ok && replayer.ReplaysReasoningContent()
}

func (p *OpenAIProvider) reasoningContentMode() ReasoningContentMode {
	if p == nil || p.provider == nil {
		return ReasoningContentDisabled
	}
	return p.provider.EffectiveReasoningContentMode()
}

func (p *OpenAIProvider) WithMCPServers(servers []MCPServerConfig) ChatProvider {
	// Gate físico (não a política): armazena os servers sempre que o transporte for
	// capaz de emitir type:"mcp". A decisão de POLÍTICA (override do perfil; default
	// auto = adapter) é feita por internal/chat antes de chamar aqui.
	if !p.NativeMCPCapable() || len(servers) == 0 {
		return p
	}
	return &OpenAIProvider{
		client:             p.client,
		provider:           p.provider,
		credMgr:            p.credMgr,
		useResponses:       p.useResponses,
		mcpServers:         servers,
		responsesAttemptFn: p.responsesAttemptFn,
	}
}

// SendChat envia uma mensagem (não-streaming), despachando para o modo correto.
func (p *OpenAIProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		return "", fmt.Errorf("nenhum modelo especificado e nenhum modelo padrão configurado")
	}

	if p.useResponses {
		return p.sendChatResponses(ctx, model, messages, params)
	}
	return p.sendChatCompletions(ctx, model, messages, params)
}

func (p *OpenAIProvider) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	msgs := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}
	return p.SendChat(ctx, msgs, ChatParams{Model: model})
}

// StreamChat faz streaming, despachando para o modo correto (Responses-first
// quando useResponses=true; senão Chat Completions).
func (p *OpenAIProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	model := resolveModel(p.provider, params.Model)
	if model == "" {
		handler.OnError("Nenhum modelo especificado e nenhum modelo padrão configurado")
		return
	}

	// Responses-first: SEMPRE usa Responses API (com ou sem MCP servers)
	if p.useResponses {
		p.streamChatResponses(ctx, model, messages, params, handler, tools...)
		return
	}

	// Chat Completions path (OpenAI-compatible legado)
	p.streamChatCompletions(ctx, model, messages, params, handler, tools...)
}

var _ ChatProvider = (*OpenAIProvider)(nil)
