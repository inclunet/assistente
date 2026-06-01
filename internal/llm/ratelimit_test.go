package llm

import (
	"context"
	"errors"
	"strings"
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

// TestRateLimiter_NearLimitAlert: o callback de proximidade dispara quando os
// tokens restantes caem abaixo do limiar.
func TestRateLimiter_NearLimitAlert(t *testing.T) {
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
	// 7 chamadas: tokens restantes 9,8,...,3 — só a última cruza o limiar (<=3).
	for i := 0; i < 7; i++ {
		if err := l.allowAt("user-1", now); err != nil {
			t.Fatalf("chamada %d deveria passar, got: %v", i+1, err)
		}
	}
	if alertCount == 0 {
		t.Fatal("alerta de proximidade deveria ter disparado")
	}
	if alertedKey != "user-1" {
		t.Errorf("chave do alerta = %q, want user-1", alertedKey)
	}
}

func TestNewRateLimiter_DisabledReturnsNil(t *testing.T) {
	if l := NewRateLimiter(RateLimitConfig{Enabled: false}); l != nil {
		t.Fatal("rate limiting desabilitado deveria retornar nil")
	}
}

func TestRateLimitConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv(envRateLimitEnabled, "")
	// Remove para garantir defaults — Setenv com "" ainda define a var; usamos
	// Unsetenv via os: t.Setenv não tem unset, então validamos os defaults
	// diretamente em DefaultRateLimitConfig.
	cfg := DefaultRateLimitConfig()
	if !cfg.Enabled {
		t.Error("default deveria estar habilitado")
	}
	if cfg.RequestsPerMinute != DefaultRateLimitRPM {
		t.Errorf("rpm default = %d, want %d", cfg.RequestsPerMinute, DefaultRateLimitRPM)
	}
	if cfg.Burst != DefaultRateLimitBurst {
		t.Errorf("burst default = %d, want %d", cfg.Burst, DefaultRateLimitBurst)
	}
}

func TestRateLimitConfigFromEnv_Overrides(t *testing.T) {
	t.Setenv(envRateLimitEnabled, "true")
	t.Setenv(envRateLimitRPM, "120")
	t.Setenv(envRateLimitBurst, "50")

	cfg := RateLimitConfigFromEnv()
	if !cfg.Enabled {
		t.Error("enabled deveria ser true")
	}
	if cfg.RequestsPerMinute != 120 {
		t.Errorf("rpm = %d, want 120", cfg.RequestsPerMinute)
	}
	if cfg.Burst != 50 {
		t.Errorf("burst = %d, want 50", cfg.Burst)
	}
}

func TestRateLimitConfigFromEnv_InvalidKeepsDefault(t *testing.T) {
	t.Setenv(envRateLimitRPM, "not-a-number")
	t.Setenv(envRateLimitBurst, "-5")

	cfg := RateLimitConfigFromEnv()
	if cfg.RequestsPerMinute != DefaultRateLimitRPM {
		t.Errorf("rpm inválido deveria manter default %d, got %d", DefaultRateLimitRPM, cfg.RequestsPerMinute)
	}
	if cfg.Burst != DefaultRateLimitBurst {
		t.Errorf("burst inválido deveria manter default %d, got %d", DefaultRateLimitBurst, cfg.Burst)
	}
}

func TestRateLimitConfigFromEnv_Disable(t *testing.T) {
	t.Setenv(envRateLimitEnabled, "false")
	cfg := RateLimitConfigFromEnv()
	if cfg.Enabled {
		t.Error("ASSISTENTE_LLM_RATE_LIMIT_ENABLED=false deveria desabilitar")
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
	return "ok", nil
}

func (f *fakeChatProvider) SupportsNativeMCP() bool { return false }

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
func (h *recordingHandler) OnError(err string)                           { h.lastError = err }
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
