package llm

import (
	"context"
	"strings"
)

type rateLimitContextKey struct{}

type rateLimitCallConfig struct {
	config      RateLimitConfig
	profileSlug string
}

// ResolvedRateLimitPolicy contém a política atual e o slug efetivamente
// resolvido. O slug pode diferir do solicitado quando um perfil foi removido e
// o fluxo recai no perfil ativo.
type ResolvedRateLimitPolicy struct {
	Config      RateLimitConfig
	ProfileSlug string
}

type RateLimitPolicyResolver func(context.Context, string) ResolvedRateLimitPolicy

// WithRateLimitProfile associa a política e o perfil a chamadas auxiliares
// que não recebem ChatParams, como a sumarização automática.
func WithRateLimitProfile(ctx context.Context, cfg RateLimitConfig, profileSlug string) context.Context {
	return context.WithValue(ctx, rateLimitContextKey{}, rateLimitCallConfig{
		config:      normalizeRateLimitConfig(cfg),
		profileSlug: strings.TrimSpace(profileSlug),
	})
}

// rateLimitedProvider é um decorator de ChatProvider que aplica rate limiting
// por usuário e perfil antes de cada chamada de geração ao provedor (Issue #27).
//
// Apenas os métodos que disparam geração no upstream são contabilizados
// (StreamChat, SendChat, SimpleChat). GetModels é metadata leve usada pela UI
// de configurações e NÃO é limitada — não é vetor de custo/abuso relevante e
// limitá-la atrapalharia a experiência de configuração.
//
// É o ponto único e central de aplicação do limite: todos os consumidores que
// resolvem o provider via providers.Service.GetChatProvider passam por aqui,
// cobrindo o loop agêntico e o streaming simples sem espalhar lógica.
type rateLimitedProvider struct {
	inner          ChatProvider
	limiter        *RateLimiter
	keyFn          func(context.Context) string
	policyResolver RateLimitPolicyResolver
}

// NewRateLimitedProvider embrulha `inner` com rate limiting por usuário e perfil.
// Quando limiter é nil (rate limiting desabilitado) ou inner é nil, retorna
// inner inalterado — mantém o comportamento legado e os testes existentes.
//
// keyFn extrai a chave de limite (tipicamente o userID) do contexto. Quando
// nil ou quando retorna vazio, uma chave global é usada para que nenhuma
// chamada escape do controle.
func NewRateLimitedProvider(inner ChatProvider, limiter *RateLimiter, keyFn func(context.Context) string) ChatProvider {
	return NewRateLimitedProviderWithResolver(inner, limiter, keyFn, nil)
}

// NewRateLimitedProviderWithResolver resolve a política atual antes de cada
// chamada. Assim, um turno antigo não consegue restaurar no bucket uma
// configuração que já foi alterada no perfil.
func NewRateLimitedProviderWithResolver(
	inner ChatProvider,
	limiter *RateLimiter,
	keyFn func(context.Context) string,
	policyResolver RateLimitPolicyResolver,
) ChatProvider {
	if inner == nil || limiter == nil {
		return inner
	}
	if keyFn == nil {
		keyFn = func(context.Context) string { return globalRateLimitKey }
	}
	return &rateLimitedProvider{
		inner:          inner,
		limiter:        limiter,
		keyFn:          keyFn,
		policyResolver: policyResolver,
	}
}

func (p *rateLimitedProvider) key(ctx context.Context, profileSlug string) string {
	key := strings.TrimSpace(p.keyFn(ctx))
	if key == "" {
		key = globalRateLimitKey
	}
	profileSlug = strings.TrimSpace(profileSlug)
	if profileSlug == "" {
		profileSlug = "__active__"
	}
	return key + "::profile::" + profileSlug
}

func rateLimitConfigForParams(params ChatParams, defaults RateLimitConfig) RateLimitConfig {
	cfg := defaults
	if params.RateLimitEnabled != nil {
		cfg.Enabled = *params.RateLimitEnabled
	}
	if params.RateLimitRPM > 0 {
		cfg.RequestsPerMinute = params.RateLimitRPM
	}
	if params.RateLimitBurst > 0 {
		cfg.Burst = params.RateLimitBurst
	}
	return cfg
}

func rateLimitConfigFromContext(ctx context.Context, defaults RateLimitConfig) rateLimitCallConfig {
	if call, ok := ctx.Value(rateLimitContextKey{}).(rateLimitCallConfig); ok {
		return call
	}
	return rateLimitCallConfig{config: defaults}
}

func (p *rateLimitedProvider) resolvePolicy(ctx context.Context, params ChatParams) rateLimitCallConfig {
	if p.policyResolver != nil {
		resolved := p.policyResolver(ctx, params.ProfileSlug)
		return rateLimitCallConfig{
			config:      normalizeRateLimitConfig(resolved.Config),
			profileSlug: strings.TrimSpace(resolved.ProfileSlug),
		}
	}
	return rateLimitCallConfig{
		config:      rateLimitConfigForParams(params, p.limiter.defaults),
		profileSlug: params.ProfileSlug,
	}
}

// StreamChat aplica o limite antes de delegar. Quando barrado, sinaliza o erro
// pelo handler (que finaliza o streaming com chat:done de erro no frontend),
// sem travar a UI nem chamar o upstream.
func (p *rateLimitedProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	policy := p.resolvePolicy(ctx, params)
	if err := p.limiter.AllowWithConfig(p.key(ctx, policy.profileSlug), policy.config); err != nil {
		if handler != nil {
			handler.OnError(err.Error())
		}
		return
	}
	p.inner.StreamChat(ctx, messages, params, handler, tools...)
}

func (p *rateLimitedProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	policy := p.resolvePolicy(ctx, params)
	if err := p.limiter.AllowWithConfig(p.key(ctx, policy.profileSlug), policy.config); err != nil {
		return "", err
	}
	return p.inner.SendChat(ctx, messages, params)
}

func (p *rateLimitedProvider) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	call := rateLimitConfigFromContext(ctx, p.limiter.defaults)
	if p.policyResolver != nil {
		resolved := p.policyResolver(ctx, call.profileSlug)
		call = rateLimitCallConfig{
			config:      normalizeRateLimitConfig(resolved.Config),
			profileSlug: strings.TrimSpace(resolved.ProfileSlug),
		}
	}
	if err := p.limiter.AllowWithConfig(p.key(ctx, call.profileSlug), call.config); err != nil {
		return "", err
	}
	return p.inner.SimpleChat(ctx, model, systemPrompt, userMessage)
}

// GetModels não é limitada (metadata leve da UI de configurações).
func (p *rateLimitedProvider) GetModels(ctx context.Context) ([]string, error) {
	return p.inner.GetModels(ctx)
}

// RefreshModels atravessa o decorator pelo mesmo motivo que GetModels não é
// limitada. Sem este repasse, o embrulho esconderia a capacidade de quem guarda
// a lista, e o recarregar da tela devolveria a lista guardada para sempre.
func (p *rateLimitedProvider) RefreshModels(ctx context.Context) ([]string, error) {
	return RefreshModels(ctx, p.inner)
}

// ModelOptions e RefreshModelOptions atravessam pelo mesmo motivo: sem o
// repasse, o embrulho esconderia quem sabe rotular os modelos e a tela voltaria
// a exibir o identificador cru do agente (AEP-0084, Fase 8).
func (p *rateLimitedProvider) ModelOptions(ctx context.Context) ([]ModelOption, error) {
	return ModelOptions(ctx, p.inner)
}

func (p *rateLimitedProvider) RefreshModelOptions(ctx context.Context) ([]ModelOption, error) {
	return RefreshModelOptions(ctx, p.inner)
}

func (p *rateLimitedProvider) NativeMCPCapable() bool {
	return p.inner.NativeMCPCapable()
}

// WithMCPServers preserva o decorator: o provider configurado com MCP servers
// continua sujeito ao mesmo limite e chave.
func (p *rateLimitedProvider) WithMCPServers(servers []MCPServerConfig) ChatProvider {
	return &rateLimitedProvider{
		inner:          p.inner.WithMCPServers(servers),
		limiter:        p.limiter,
		keyFn:          p.keyFn,
		policyResolver: p.policyResolver,
	}
}
