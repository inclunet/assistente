package llm

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestLimiter(rpm, burst int) *RateLimiter {
	return NewRateLimiter(RateLimitConfig{
		Enabled:           true,
		RequestsPerMinute: rpm,
		Burst:             burst,
	})
}

// TestRateLimiter_WithinLimitPasses: chamadas dentro da rajada são permitidas.
func TestRateLimiter_WithinLimitPasses(t *testing.T) {
	l := newTestLimiter(60, 5)
	now := time.Now()
	for i := 0; i < 5; i++ {
		if err := l.allowAt("user-1", now); err != nil {
			t.Fatalf("chamada %d dentro do limite deveria passar, got: %v", i+1, err)
		}
	}
}

// TestRateLimiter_OverLimitBlocked: estourando a rajada, a chamada é barrada
// com um RateLimitError carregando RetryAfter > 0.
func TestRateLimiter_OverLimitBlocked(t *testing.T) {
	l := newTestLimiter(60, 3)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.allowAt("user-1", now); err != nil {
			t.Fatalf("chamada %d deveria passar dentro da rajada, got: %v", i+1, err)
		}
	}

	err := l.allowAt("user-1", now)
	if err == nil {
		t.Fatal("4ª chamada deveria ser barrada pelo rate limit")
	}
	if !IsRateLimitError(err) {
		t.Fatalf("erro deveria ser RateLimitError, got: %T", err)
	}
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("errors.As deveria extrair RateLimitError, got: %v", err)
	}
	if rl.RetryAfter <= 0 {
		t.Errorf("RetryAfter deveria ser > 0, got: %v", rl.RetryAfter)
	}
}

// TestRateLimiter_BlockedDoesNotConsumeToken: uma chamada barrada NÃO consome
// cota futura (a reserva é cancelada). Após o reabastecimento de exatamente 1
// token, exatamente 1 chamada passa. Valida o refator de allowAt (ReserveN
// único + Cancel apenas quando há atraso).
func TestRateLimiter_BlockedDoesNotConsumeToken(t *testing.T) {
	l := newTestLimiter(60, 1) // 1 token/s, burst 1
	now := time.Now()
	if err := l.allowAt("u", now); err != nil {
		t.Fatalf("primeira chamada deveria passar, got: %v", err)
	}
	// Várias tentativas barradas não devem "endividar" o bucket.
	for i := 0; i < 5; i++ {
		if err := l.allowAt("u", now); err == nil {
			t.Fatalf("tentativa barrada %d deveria falhar", i+1)
		}
	}
	// Após 1s, exatamente 1 token disponível → 1 chamada passa, a próxima barra.
	later := now.Add(1 * time.Second)
	if err := l.allowAt("u", later); err != nil {
		t.Fatalf("após reabastecimento deveria passar (barrado não consumiu cota futura), got: %v", err)
	}
	if err := l.allowAt("u", later); err == nil {
		t.Fatal("segunda chamada imediata após 1 token deveria barrar")
	}
}

// TestRateLimiter_ResetAfterInterval: após o intervalo de reabastecimento, a
// cota volta a permitir chamadas.
func TestRateLimiter_ResetAfterInterval(t *testing.T) {
	// 60 rpm => 1 token/segundo de reabastecimento.
	l := newTestLimiter(60, 1)
	now := time.Now()

	if err := l.allowAt("user-1", now); err != nil {
		t.Fatalf("primeira chamada deveria passar, got: %v", err)
	}
	if err := l.allowAt("user-1", now); err == nil {
		t.Fatal("segunda chamada imediata deveria ser barrada")
	}

	// Avança 1 segundo: 1 token reabastecido.
	later := now.Add(1100 * time.Millisecond)
	if err := l.allowAt("user-1", later); err != nil {
		t.Fatalf("chamada após reabastecimento deveria passar, got: %v", err)
	}
}

// TestRateLimiter_PerUserIsolation: o limite de um usuário não afeta o outro.
func TestRateLimiter_PerUserIsolation(t *testing.T) {
	l := newTestLimiter(60, 1)
	now := time.Now()

	if err := l.allowAt("user-1", now); err != nil {
		t.Fatalf("user-1 primeira chamada deveria passar, got: %v", err)
	}
	if err := l.allowAt("user-1", now); err == nil {
		t.Fatal("user-1 segunda chamada deveria ser barrada")
	}
	// user-2 tem bucket próprio e ainda cheio.
	if err := l.allowAt("user-2", now); err != nil {
		t.Fatalf("user-2 não deveria ser afetado por user-1, got: %v", err)
	}
}

// TestRateLimiter_NilIsNoop: limitador nil (desabilitado) permite tudo.
func TestRateLimiter_NilIsNoop(t *testing.T) {
	var l *RateLimiter
	for i := 0; i < 100; i++ {
		if err := l.Allow("any"); err != nil {
			t.Fatalf("limitador nil deveria permitir tudo, got: %v", err)
		}
	}
}

// TestRateLimiter_EmptyKeyUsesGlobal: chave vazia cai no bucket global.
func TestRateLimiter_EmptyKeyUsesGlobal(t *testing.T) {
	l := newTestLimiter(60, 1)
	now := time.Now()
	if err := l.allowAt("", now); err != nil {
		t.Fatalf("chave vazia deveria passar na primeira chamada, got: %v", err)
	}
	if err := l.allowAt(globalRateLimitKey, now); err == nil {
		t.Fatal("chave vazia e global devem compartilhar o mesmo bucket")
	}
}

// TestRateLimiter_NearLimitAlert_EdgeTriggered: o callback dispara UMA vez ao
// cruzar o limiar (acima→abaixo) e NÃO realerta enquanto a chave continua
// abaixo do limiar.
func TestRateLimiter_NearLimitAlert_EdgeTriggered(t *testing.T) {
	l := NewRateLimiter(RateLimitConfig{
		Enabled:            true,
		RequestsPerMinute:  60,
		Burst:              10,
		NearLimitThreshold: 0.3, // alerta quando restarem <= 3 tokens
	})
	var alertedKey string
	var alertCount int
	l.SetNearLimitHandler(func(key string, remaining float64) {
		alertedKey = key
		alertCount++
	})

	now := time.Now()
	// 7 chamadas: tokens restantes 9,8,...,3 — só a 7ª cruza o limiar (<=3).
	for i := 0; i < 7; i++ {
		if err := l.allowAt("user-1", now); err != nil {
			t.Fatalf("chamada %d deveria passar, got: %v", i+1, err)
		}
	}
	if alertCount != 1 {
		t.Fatalf("esperava 1 alerta ao cruzar o limiar, got %d", alertCount)
	}
	if alertedKey != "user-1" {
		t.Errorf("chave do alerta = %q, want user-1", alertedKey)
	}

	// Mais chamadas permitidas, mas ainda abaixo do limiar (tokens 2,1,0):
	// NÃO devem realertar.
	for i := 0; i < 3; i++ {
		if err := l.allowAt("user-1", now); err != nil {
			t.Fatalf("chamada extra %d deveria passar dentro da rajada, got: %v", i+1, err)
		}
	}
	if alertCount != 1 {
		t.Fatalf("não deveria realertar enquanto continua abaixo do limiar, got %d", alertCount)
	}
}

// TestRateLimiter_NearLimitAlert_ResetsAboveThreshold: após a chave voltar
// acima do limiar (reabastecimento), um novo cruzamento dispara o alerta de
// novo.
func TestRateLimiter_NearLimitAlert_ResetsAboveThreshold(t *testing.T) {
	l := NewRateLimiter(RateLimitConfig{
		Enabled:            true,
		RequestsPerMinute:  60, // 1 token/segundo de reabastecimento
		Burst:              10,
		NearLimitThreshold: 0.3, // <= 3 tokens
	})
	var alertCount int
	l.SetNearLimitHandler(func(string, float64) { alertCount++ })

	now := time.Now()
	// Cruza o limiar pela primeira vez (7ª chamada → restam 3).
	for i := 0; i < 7; i++ {
		if err := l.allowAt("user-1", now); err != nil {
			t.Fatalf("chamada %d deveria passar, got: %v", i+1, err)
		}
	}
	if alertCount != 1 {
		t.Fatalf("esperava 1 alerta inicial, got %d", alertCount)
	}

	// Reabastece acima do limiar: +7s → bucket volta cheio (cap 10).
	// last era `now` com 3 tokens; após 7s: min(10, 3+7) = 10 tokens.
	later := now.Add(7 * time.Second)
	// Uma chamada permitida acima do limiar reseta o estado de alerta sem
	// disparar o callback (restam ~9 > 3).
	if err := l.allowAt("user-1", later); err != nil {
		t.Fatalf("chamada após reabastecimento deveria passar, got: %v", err)
	}
	if alertCount != 1 {
		t.Fatalf("chamada acima do limiar não deveria realertar, got %d", alertCount)
	}

	// Drena exatamente até cruzar o limiar de novo (tokens 9→8→7→6→5→4→3),
	// sem gerar chamadas barradas. O cruzamento ocorre na 6ª chamada → 2º alerta.
	for i := 0; i < 6; i++ {
		if err := l.allowAt("user-1", later); err != nil {
			t.Fatalf("drain %d deveria passar dentro da rajada, got: %v", i+1, err)
		}
	}
	if alertCount != 2 {
		t.Fatalf("esperava 2 alertas após novo cruzamento, got %d", alertCount)
	}
}

func TestRateLimiter_NearLimitIgnoresStalePolicyGeneration(t *testing.T) {
	l := newTestLimiter(60, 10)
	now := time.Now()
	oldCfg := RateLimitConfig{
		Enabled:            true,
		RequestsPerMinute:  60,
		Burst:              10,
		NearLimitThreshold: 0.5,
	}
	entry, oldGeneration := l.limiterFor("user-1", oldCfg, now)
	if !entry.limiter.AllowN(now, 6) {
		t.Fatal("setup deveria consumir seis tokens")
	}

	newCfg := RateLimitConfig{
		Enabled:            true,
		RequestsPerMinute:  60,
		Burst:              20,
		NearLimitThreshold: 0.1,
	}
	_, newGeneration := l.limiterFor("user-1", newCfg, now)
	if newGeneration == oldGeneration {
		t.Fatal("reconfiguração deveria avançar a geração")
	}

	var alerts int
	l.SetNearLimitHandler(func(string, float64) { alerts++ })
	// Sob a política antiga, os quatro tokens restantes estariam abaixo de 50%
	// e marcariam a chave como alertada. A geração obsoleta deve abortar antes
	// de reintroduzir esse estado depois do reset da política nova.
	l.maybeNearLimit("user-1", entry, oldGeneration, oldCfg, now)

	l.mu.Lock()
	alerted := l.nearAlerted["user-1"]
	l.mu.Unlock()
	if alerted || alerts != 0 {
		t.Fatalf("avaliação obsoleta alterou o alerta: alerted=%v callbacks=%d", alerted, alerts)
	}
}

// TestRateLimiter_ConcurrentSetHandlerAndAllow exercita SetNearLimitHandler
// concorrentemente com Allow/maybeNearLimit para que o `-race` detector
// flagre regressões de data race no campo onNearLimit.
func TestRateLimiter_ConcurrentSetHandlerAndAllow(t *testing.T) {
	l := newTestLimiter(6000, 100) // alto o suficiente para não barrar sob carga

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Escritores: trocam o handler repetidamente (inclui nil).
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				l.SetNearLimitHandler(func(string, float64) {})
				l.SetNearLimitHandler(nil)
			}
		}(w)
	}

	// Leitores: chamam Allow concorrentemente, exercitando maybeNearLimit.
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "user-" + string(rune('a'+id))
			for i := 0; i < 2000; i++ {
				_ = l.Allow(key)
			}
		}(r)
	}

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestNewRateLimiter_DisabledReturnsNil(t *testing.T) {
	if l := NewRateLimiter(RateLimitConfig{Enabled: false}); l != nil {
		t.Fatal("rate limiting desabilitado deveria retornar nil")
	}
}

func TestRateLimiter_AllowWithConfigCanDisableProfile(t *testing.T) {
	l := newTestLimiter(60, 1)
	disabled := RateLimitConfig{Enabled: false}
	for i := 0; i < 3; i++ {
		if err := l.AllowWithConfig("user-1::profile::sem-limite", disabled); err != nil {
			t.Fatalf("perfil desabilitado deveria passar: %v", err)
		}
	}
}

func TestRateLimiter_AllowWithConfigReconfiguresWithoutRefillingBucket(t *testing.T) {
	l := newTestLimiter(60, 1)
	key := "user-1::profile::pesquisa"
	now := time.Now()
	one := RateLimitConfig{Enabled: true, RequestsPerMinute: 60, Burst: 1}
	if err := l.allowAtWithConfig(key, now, one); err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	if err := l.allowAtWithConfig(key, now, one); err == nil {
		t.Fatal("segunda chamada deveria ser bloqueada com burst 1")
	}

	two := RateLimitConfig{Enabled: true, RequestsPerMinute: 120, Burst: 2}
	if err := l.allowAtWithConfig(key, now, two); err == nil {
		t.Fatal("aumentar o limite não deve devolver tokens já consumidos")
	}
	if err := l.allowAtWithConfig(key, now.Add(500*time.Millisecond), two); err != nil {
		t.Fatalf("a nova taxa deve reabastecer o bucket sem recriá-lo: %v", err)
	}
}

func TestRateLimitError_Message(t *testing.T) {
	err := &RateLimitError{Key: "user-1", RetryAfter: 2 * time.Second}
	msg := err.Error()
	if msg == "" {
		t.Fatal("mensagem de erro não deveria ser vazia")
	}
	// A mensagem é exibível ao usuário; deve ser clara e em pt-BR.
	if !strings.Contains(msg, "Limite de requisições") {
		t.Errorf("mensagem inesperada: %q", msg)
	}
}

// ---- Decorator (rateLimitedProvider) ----

// fakeChatProvider conta chamadas e satisfaz ChatProvider para testar o decorator.
type fakeChatProvider struct {
	streamCalls int
	sendCalls   int
	modelsCalls int
	simpleCalls int
}

func (f *fakeChatProvider) StreamChat(ctx context.Context, messages []Message, params ChatParams, handler StreamHandler, tools ...ToolDefinition) {
	f.streamCalls++
	if handler != nil {
		handler.OnDone("ok", Usage{}, "fake")
	}
}

func (f *fakeChatProvider) SendChat(ctx context.Context, messages []Message, params ChatParams) (string, error) {
	f.sendCalls++
	return "ok", nil
}

func (f *fakeChatProvider) GetModels(ctx context.Context) ([]string, error) {
	f.modelsCalls++
	return []string{"m1"}, nil
}

func (f *fakeChatProvider) SimpleChat(ctx context.Context, model, systemPrompt, userMessage string) (string, error) {
	f.simpleCalls++
	return "ok", nil
}

func (f *fakeChatProvider) NativeMCPCapable() bool { return false }

func (f *fakeChatProvider) WithMCPServers(servers []MCPServerConfig) ChatProvider { return f }

// recordingHandler captura OnError do StreamHandler.
type recordingHandler struct {
	lastError string
	doneCount int
}

func (h *recordingHandler) OnChunk(string)                                {}
func (h *recordingHandler) OnThinking(string)                             {}
func (h *recordingHandler) OnThinkingDone(string)                         {}
func (h *recordingHandler) OnToolCalls([]ToolCall, string, Usage, string) {}
func (h *recordingHandler) OnError(err string)                            { h.lastError = err }
func (h *recordingHandler) OnDone(string, Usage, string)                  { h.doneCount++ }
func (h *recordingHandler) OnMCPToolEvent(MCPToolEvent)                   {}

func TestRateLimitedProvider_NilLimiterPassthrough(t *testing.T) {
	inner := &fakeChatProvider{}
	got := NewRateLimitedProvider(inner, nil, nil)
	if got != ChatProvider(inner) {
		t.Fatal("limiter nil deveria retornar o provider inalterado")
	}
}

func TestRateLimitedProvider_StreamChatBlockedSignalsHandler(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 1)
	keyFn := func(context.Context) string { return "user-1" }
	p := NewRateLimitedProvider(inner, limiter, keyFn)

	h1 := &recordingHandler{}
	p.StreamChat(context.Background(), nil, ChatParams{}, h1)
	if inner.streamCalls != 1 {
		t.Fatalf("primeira chamada deveria delegar ao inner, streamCalls=%d", inner.streamCalls)
	}
	if h1.lastError != "" {
		t.Fatalf("primeira chamada não deveria ter erro, got: %q", h1.lastError)
	}

	h2 := &recordingHandler{}
	p.StreamChat(context.Background(), nil, ChatParams{}, h2)
	if inner.streamCalls != 1 {
		t.Fatalf("segunda chamada (barrada) não deveria delegar, streamCalls=%d", inner.streamCalls)
	}
	if h2.lastError == "" {
		t.Fatal("segunda chamada deveria sinalizar OnError com mensagem de rate limit")
	}
}

func TestRateLimitedProvider_ProfilesUseIndependentBuckets(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 1)
	p := NewRateLimitedProvider(inner, limiter, func(context.Context) string { return "user-1" })

	for _, slug := range []string{"conversa", "pesquisa"} {
		h := &recordingHandler{}
		p.StreamChat(context.Background(), nil, ChatParams{ProfileSlug: slug}, h)
		if h.lastError != "" {
			t.Fatalf("primeira chamada do perfil %q não deveria ser bloqueada: %s", slug, h.lastError)
		}
	}
	if inner.streamCalls != 2 {
		t.Fatalf("cada perfil deveria ter bucket próprio, chamadas delegadas=%d", inner.streamCalls)
	}
}

func TestRateLimitedProvider_ProfileCanDisableLimit(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 1)
	p := NewRateLimitedProvider(inner, limiter, func(context.Context) string { return "user-1" })
	disabled := false
	params := ChatParams{ProfileSlug: "longo", RateLimitEnabled: &disabled}

	for i := 0; i < 3; i++ {
		p.StreamChat(context.Background(), nil, params, &recordingHandler{})
	}
	if inner.streamCalls != 3 {
		t.Fatalf("perfil sem limite deveria delegar todas as chamadas, got %d", inner.streamCalls)
	}
}

func TestRateLimitedProvider_ResolverWinsOverStaleTurnSnapshot(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 30)
	current := RateLimitConfig{Enabled: true, RequestsPerMinute: 60, Burst: 1}
	resolver := func(_ context.Context, slug string) ResolvedRateLimitPolicy {
		return ResolvedRateLimitPolicy{Config: current, ProfileSlug: slug}
	}
	p := NewRateLimitedProviderWithResolver(
		inner,
		limiter,
		func(context.Context) string { return "user-1" },
		resolver,
	)
	enabled := true
	stale := ChatParams{
		ProfileSlug:      "pesquisa",
		RateLimitEnabled: &enabled,
		RateLimitRPM:     600,
		RateLimitBurst:   30,
	}

	p.StreamChat(context.Background(), nil, stale, &recordingHandler{})
	blocked := &recordingHandler{}
	p.StreamChat(context.Background(), nil, stale, blocked)
	if blocked.lastError == "" || inner.streamCalls != 1 {
		t.Fatal("snapshot antigo não deve alargar a política atual resolvida do perfil")
	}
}

func TestRateLimitedProvider_UsesEffectiveSlugFromResolver(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 1)
	resolver := func(context.Context, string) ResolvedRateLimitPolicy {
		return ResolvedRateLimitPolicy{
			Config:      RateLimitConfig{Enabled: true, RequestsPerMinute: 60, Burst: 1},
			ProfileSlug: "ativo",
		}
	}
	p := NewRateLimitedProviderWithResolver(
		inner,
		limiter,
		func(context.Context) string { return "user-1" },
		resolver,
	)

	p.StreamChat(context.Background(), nil, ChatParams{ProfileSlug: "removido"}, &recordingHandler{})
	blocked := &recordingHandler{}
	p.StreamChat(context.Background(), nil, ChatParams{ProfileSlug: "ativo"}, blocked)
	if blocked.lastError == "" {
		t.Fatal("slug removido e perfil ativo devem compartilhar o bucket efetivo")
	}
}

func TestRateLimitedProvider_SendChatBlockedReturnsError(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 1)
	keyFn := func(context.Context) string { return "user-1" }
	p := NewRateLimitedProvider(inner, limiter, keyFn)

	if _, err := p.SendChat(context.Background(), nil, ChatParams{}); err != nil {
		t.Fatalf("primeira SendChat deveria passar, got: %v", err)
	}
	_, err := p.SendChat(context.Background(), nil, ChatParams{})
	if err == nil {
		t.Fatal("segunda SendChat deveria ser barrada")
	}
	if !IsRateLimitError(err) {
		t.Fatalf("erro deveria ser RateLimitError, got: %T", err)
	}
}

func TestRateLimitedProvider_SimpleChatLimited(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 1)
	keyFn := func(context.Context) string { return "user-1" }
	p := NewRateLimitedProvider(inner, limiter, keyFn)

	if _, err := p.SimpleChat(context.Background(), "m", "sys", "hi"); err != nil {
		t.Fatalf("primeira SimpleChat deveria passar, got: %v", err)
	}
	if inner.simpleCalls != 1 {
		t.Fatalf("primeira SimpleChat deveria delegar, simpleCalls=%d", inner.simpleCalls)
	}
	_, err := p.SimpleChat(context.Background(), "m", "sys", "hi")
	if err == nil {
		t.Fatal("segunda SimpleChat deveria ser barrada")
	}
	if !IsRateLimitError(err) {
		t.Fatalf("erro deveria ser RateLimitError, got: %T", err)
	}
	if inner.simpleCalls != 1 {
		t.Fatalf("SimpleChat barrada não deveria delegar, simpleCalls=%d", inner.simpleCalls)
	}
}

func TestRateLimitedProvider_SimpleChatUsesProfileFromContext(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 1)
	p := NewRateLimitedProvider(inner, limiter, func(context.Context) string { return "user-1" })
	ctx := WithRateLimitProfile(context.Background(), RateLimitConfig{Enabled: false}, "longo")

	for i := 0; i < 3; i++ {
		if _, err := p.SimpleChat(ctx, "m", "sys", "hi"); err != nil {
			t.Fatalf("sumarização do perfil sem limite deveria passar: %v", err)
		}
	}
}

func TestRateLimitedProvider_SimpleChatResolverOverridesStaleContext(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 30)
	resolver := func(context.Context, string) ResolvedRateLimitPolicy {
		return ResolvedRateLimitPolicy{
			Config:      RateLimitConfig{Enabled: true, RequestsPerMinute: 60, Burst: 1},
			ProfileSlug: "ativo",
		}
	}
	p := NewRateLimitedProviderWithResolver(
		inner,
		limiter,
		func(context.Context) string { return "user-1" },
		resolver,
	)
	staleCtx := WithRateLimitProfile(context.Background(), RateLimitConfig{Enabled: false}, "ativo")

	if _, err := p.SimpleChat(staleCtx, "m", "sys", "hi"); err != nil {
		t.Fatalf("primeira chamada: %v", err)
	}
	if _, err := p.SimpleChat(staleCtx, "m", "sys", "hi"); err == nil {
		t.Fatal("política atual deve prevalecer sobre o snapshot antigo da sumarização")
	}
}

func TestRateLimitedProvider_GetModelsNotLimited(t *testing.T) {
	inner := &fakeChatProvider{}
	limiter := newTestLimiter(60, 1)
	keyFn := func(context.Context) string { return "user-1" }
	p := NewRateLimitedProvider(inner, limiter, keyFn)

	// Mesmo após esgotar a cota de geração, GetModels continua liberado.
	_, _ = p.SendChat(context.Background(), nil, ChatParams{})
	for i := 0; i < 5; i++ {
		if _, err := p.GetModels(context.Background()); err != nil {
			t.Fatalf("GetModels não deveria ser limitada, got: %v", err)
		}
	}
	if inner.modelsCalls != 5 {
		t.Fatalf("GetModels deveria delegar todas as vezes, got %d", inner.modelsCalls)
	}
}
