package llm

import (
	"context"
	"strings"
)

// rateLimitedProvider é um decorator de ChatProvider que aplica rate limiting
// por usuário antes de cada chamada de geração ao provedor (Issue #27).
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
	inner   ChatProvider
	limiter *RateLimiter
	keyFn   func(context.Context) string
}

// NewRateLimitedProvider embrulha `inner` com rate limiting por usuário.
// Quando limiter é nil (rate limiting desabilitado) ou inner é nil, retorna
// inner inalterado — mantém o comportamento legado e os testes existentes.
//
// keyFn extrai a chave de limite (tipicamente o userID) do contexto. Quando
// nil ou quando retorna vazio, uma chave global é usada para que nenhuma
// chamada escape do controle.
func NewRateLimitedProvider(inner ChatProvider, limiter *RateLimiter, keyFn func(context.Context) string) ChatProvider {
	if inner == nil || limiter == nil {
		return inner
	}
	if keyFn == nil {
		keyFn = func(context.Context) string { return globalRateLimitKey }
	}
	return &rateLimitedProvider{inner: inner, limiter: limiter, keyFn: keyFn}
}

func (p *rateLimitedProvider) key(ctx context.Context) string {
	key := strings.TrimSpace(p.keyFn(ctx))
	if key == "" {
		return globalRateLimitKey
	}
	return key
}

// StreamChat aplica o limite antes de delegar. Quando barrado, sinaliza o erro
// pelo handler (que finaliza o streaming com chat:done de erro no frontend),
// sem travar a UI nem chamar o upstream.
func (p *rateLimitedProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	if err := p.limiter.Allow(p.key(ctx)); err != nil {
		if handler != nil {
			handler.OnError(err.Error())
		}
		return
	}
	p.inner.StreamChat(ctx, messages, params, handler, tools...)
}

func (p *rateLimitedProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	if err := p.limiter.Allow(p.key(ctx)); err != nil {
		return "", err
	}
	return p.inner.SendChat(ctx, messages, params)
}

func (p *rateLimitedProvider) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	if err := p.limiter.Allow(p.key(ctx)); err != nil {
		return "", err
	}
	return p.inner.SimpleChat(ctx, model, systemPrompt, userMessage)
}

// GetModels não é limitada (metadata leve da UI de configurações).
func (p *rateLimitedProvider) GetModels(ctx context.Context) ([]string, error) {
	return p.inner.GetModels(ctx)
}

func (p *rateLimitedProvider) SupportsNativeMCP() bool {
	return p.inner.SupportsNativeMCP()
}

// WithMCPServers preserva o decorator: o provider configurado com MCP servers
// continua sujeito ao mesmo limite e chave.
func (p *rateLimitedProvider) WithMCPServers(servers []MCPServerConfig) ChatProvider {
	return &rateLimitedProvider{
		inner:   p.inner.WithMCPServers(servers),
		limiter: p.limiter,
		keyFn:   p.keyFn,
	}
}
