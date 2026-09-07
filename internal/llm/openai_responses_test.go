package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/openai/openai-go/responses"
)

func TestOpenAIResponsesProvider_NativeMCPCapable(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIResponsesProvider(p, credMgr)
	if !provider.NativeMCPCapable() {
		t.Error("Responses provider should be native MCP capable")
	}
}

func TestOpenAIResponsesProvider_WithMCPServers_EmptyReturnsOriginal(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIResponsesProvider(p, credMgr)
	result := provider.WithMCPServers(nil)
	if result != provider {
		t.Error("WithMCPServers(nil) should return same provider")
	}
	result = provider.WithMCPServers([]MCPServerConfig{})
	if result != provider {
		t.Error("WithMCPServers([]) should return same provider")
	}
}

// TestOpenAIResponsesProvider_Proxy_CapableRegardlessOfURL cobre o caso do proxy
// OpenAI-compatible (ex.: LiteLLM) com api_format=openai_responses: ele fala a
// Responses API e é fisicamente CAPAZ de emitir type:"mcp" (NativeMCPCapable=true)
// independentemente da URL. Não há mais heurística por endpoint; o DEFAULT (auto)
// é adapter e MCP nativo é opt-in por perfil (ver internal/chat.ResolveNativeMCPEnabled).
func TestOpenAIResponsesProvider_Proxy_CapableRegardlessOfURL(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:        "litellm",
		Name:      "LiteLLM",
		Type:      ProviderCustom,
		APIFormat: APIFormatOpenAIResponses,
		BaseURL:   "http://llm.inclunet.com.br/v1",
	}
	provider := NewOpenAIResponsesProvider(p, credMgr)

	if !provider.useResponses {
		t.Fatal("proxy deve continuar usando a Responses API (useResponses=true)")
	}
	if !provider.NativeMCPCapable() {
		t.Error("proxy via Responses API é fisicamente capaz de emitir type:mcp")
	}
}

func TestBuildResponsesParams_EmitsPromptCacheKeyWhenProvided(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	ctx := context.Background()
	msgs := []Message{{Role: "user", Content: "oi"}}
	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "o", Name: "OpenAI", Type: ProviderOpenAI,
		APIFormat: APIFormatOpenAIResponses, BaseURL: "https://api.openai.com/v1",
	}, credMgr)

	got := provider.buildResponsesParams(ctx, "gpt-4o-mini", msgs, ChatParams{PromptCacheKey: "asst-abc"}, nil)
	if !got.PromptCacheKey.Valid() || got.PromptCacheKey.Value != "asst-abc" {
		t.Fatalf("PromptCacheKey = %#v, want asst-abc", got.PromptCacheKey)
	}
}

func TestOpenAIResponsesProvider_UsesResponsesWithoutMCP(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "openai-real",
		Name:    "OpenAI",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIResponsesProvider(p, credMgr)

	if !provider.useResponses {
		t.Error("Responses provider should have useResponses=true")
	}
	if len(provider.mcpServers) != 0 {
		t.Error("Fresh provider should have no MCP servers")
	}
	if !provider.NativeMCPCapable() {
		t.Error("Responses provider should be native MCP capable")
	}
}

type noopStreamHandler struct {
	err string
}

func (h *noopStreamHandler) OnChunk(string) {}

func (h *noopStreamHandler) OnThinking(string) {}

func (h *noopStreamHandler) OnThinkingDone(string) {}

func (h *noopStreamHandler) OnToolCalls([]ToolCall, string, Usage, string) {}

func (h *noopStreamHandler) OnError(err string) { h.err = err }

func (h *noopStreamHandler) OnDone(string, Usage, string) {}

func (h *noopStreamHandler) OnMCPToolEvent(MCPToolEvent) {}

func TestOpenAIResponsesStreamInjectsScopedCredential(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		http.Error(w, "stop after auth capture", http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx := database.WithUserID(context.Background(), "user-1")
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	if err := credMgr.RegisterPatternWithContext(ctx, "llm.inclunet.com.br", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "sk-litellm-user-1",
	}); err != nil {
		t.Fatalf("RegisterPatternWithContext() error = %v", err)
	}

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID:                "litellm-test",
		Name:              "LiteLLM Test",
		BaseURL:           server.URL + "/v1",
		APIFormat:         APIFormatOpenAIResponses,
		CredentialPattern: "llm.inclunet.com.br",
		DefaultModel:      "test-model",
	}, credMgr)

	handler := &noopStreamHandler{}
	provider.StreamChat(ctx, []Message{{Role: "user", Content: "hello"}}, ChatParams{Model: "test-model"}, handler)

	if gotAuth != "Bearer sk-litellm-user-1" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer sk-litellm-user-1")
	}
}

func TestOpenAIProvider_StreamChatResponses_DegradesFailedMCPServer(t *testing.T) {
	recovered := make(chan struct{}, 1)
	seen := make([][]string, 0, 2)
	attempts := 0

	provider := &OpenAIProvider{
		provider:     &ProviderConfig{ID: "o", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
		useResponses: true,
		mcpServers: []MCPServerConfig{
			{
				Name: "Atlassian",
				Slug: "atlassian",
				URL:  "https://mcp.atlassian.com/v1/sse",
				Recover: func(context.Context) error {
					recovered <- struct{}{}
					return nil
				},
			},
			{Name: "Slack", Slug: "slack", URL: "https://mcp.slack.com/mcp"},
		},
	}
	provider.responsesAttemptFn = func(_ context.Context, _ responses.ResponseNewParams, handler StreamHandler, servers []MCPServerConfig, _ ChatParams, _ *DebugDumpHandle) mcpStreamAttemptResult {
		attempts++
		slugs := make([]string, 0, len(servers))
		for _, srv := range servers {
			slugs = append(slugs, srv.Slug)
		}
		seen = append(seen, slugs)
		if attempts == 1 {
			return mcpStreamAttemptResult{
				mcpFailure: &MCPAttemptFailure{
					ServerName: "Atlassian",
					ServerSlug: "atlassian",
					Stage:      MCPFailureStageListTools,
					Message:    "Falha no Atlassian",
					Degradable: true,
				},
			}
		}
		handler.OnChunk("ok")
		handler.OnDone("ok", Usage{}, "gpt-test")
		return mcpStreamAttemptResult{done: true}
	}

	handler := &providerRetryHandler{}
	provider.streamChatResponses(context.Background(), "gpt-test", []Message{{Role: "user", Content: "oi"}}, ChatParams{}, handler)

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(seen) != 2 || len(seen[0]) != 2 || len(seen[1]) != 1 || seen[1][0] != "slack" {
		t.Fatalf("servers por tentativa = %#v", seen)
	}
	select {
	case <-recovered:
	case <-time.After(1 * time.Second):
		t.Fatal("esperava callback de recovery assíncrono")
	}
	if got := handler.chunks.String(); got != "ok" {
		t.Fatalf("chunk final = %q, want %q", got, "ok")
	}
	if len(handler.errors) != 0 {
		t.Fatalf("erros inesperados: %v", handler.errors)
	}
	if handler.done != 1 {
		t.Fatalf("OnDone = %d, want 1", handler.done)
	}
}

// TestOpenAIProvider_StreamChatResponses_NativeMCPUnsupportedFallsBackToAdapter
// cobre o auto-fallback nativo→adapter no MESMO turno: a 1ª tentativa (com MCP
// nativo) retorna nativeMCPUnsupported; o loop dropa os servers, dispara o hook
// OnNativeMCPUnsupported e re-tenta sem servers, concluindo sem erro.
func TestOpenAIProvider_StreamChatResponses_NativeMCPUnsupportedFallsBackToAdapter(t *testing.T) {
	seen := make([][]string, 0, 2)
	attempts := 0
	hookCalls := 0

	provider := &OpenAIProvider{
		provider:     &ProviderConfig{ID: "o", Name: "Proxy", BaseURL: "http://proxy.local/v1"},
		useResponses: true,
		mcpServers: []MCPServerConfig{
			{Name: "Atlassian", Slug: "atlassian", URL: "https://mcp.atlassian.com/v1/sse"},
		},
	}
	provider.responsesAttemptFn = func(_ context.Context, _ responses.ResponseNewParams, handler StreamHandler, servers []MCPServerConfig, _ ChatParams, _ *DebugDumpHandle) mcpStreamAttemptResult {
		attempts++
		slugs := make([]string, 0, len(servers))
		for _, srv := range servers {
			slugs = append(slugs, srv.Slug)
		}
		seen = append(seen, slugs)
		if attempts == 1 {
			return mcpStreamAttemptResult{nativeMCPUnsupported: true}
		}
		handler.OnChunk("ok")
		handler.OnDone("ok", Usage{}, "deepseek-v4-flash")
		return mcpStreamAttemptResult{done: true}
	}

	handler := &providerRetryHandler{}
	params := ChatParams{OnNativeMCPUnsupported: func() { hookCalls++ }}
	provider.streamChatResponses(context.Background(), "deepseek-v4-flash", []Message{{Role: "user", Content: "oi"}}, params, handler)

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(seen) != 2 || len(seen[0]) != 1 || len(seen[1]) != 0 {
		t.Fatalf("servers por tentativa = %#v (esperava [1 server] depois [0 servers])", seen)
	}
	if hookCalls != 1 {
		t.Fatalf("OnNativeMCPUnsupported chamado %d vezes, want 1", hookCalls)
	}
	if len(handler.errors) != 0 {
		t.Fatalf("não deveria emitir erro ao usuário (fallback transparente): %v", handler.errors)
	}
	if handler.done != 1 {
		t.Fatalf("OnDone = %d, want 1", handler.done)
	}
}

func TestOpenAIProvider_StreamChatResponses_PromptCacheHintUnsupportedRetriesWithoutHint(t *testing.T) {
	attempts := 0
	hookCalls := 0
	seenKeys := make([]string, 0, 2)

	provider := &OpenAIProvider{
		provider:     &ProviderConfig{ID: "o", Name: "Proxy", BaseURL: "http://proxy.local/v1"},
		useResponses: true,
	}
	provider.responsesAttemptFn = func(_ context.Context, params responses.ResponseNewParams, handler StreamHandler, _ []MCPServerConfig, _ ChatParams, _ *DebugDumpHandle) mcpStreamAttemptResult {
		attempts++
		if params.PromptCacheKey.Valid() {
			seenKeys = append(seenKeys, params.PromptCacheKey.Value)
		} else {
			seenKeys = append(seenKeys, "")
		}
		if attempts == 1 {
			return mcpStreamAttemptResult{promptCacheHintUnsupported: true}
		}
		handler.OnChunk("ok")
		handler.OnDone("ok", Usage{}, "gpt-test")
		return mcpStreamAttemptResult{done: true}
	}

	handler := &providerRetryHandler{}
	params := ChatParams{
		PromptCacheKey:          "asst-key",
		PromptCacheHintFallback: &PromptCacheHintFallback{},
		OnPromptCacheHintUnsupported: func() {
			hookCalls++
		},
	}
	provider.streamChatResponses(context.Background(), "gpt-test", []Message{{Role: "user", Content: "oi"}}, params, handler)

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(seenKeys) != 2 || seenKeys[0] != "asst-key" || seenKeys[1] != "" {
		t.Fatalf("seenKeys = %#v, want [asst-key, empty]", seenKeys)
	}
	if hookCalls != 1 {
		t.Fatalf("OnPromptCacheHintUnsupported chamado %d vezes, want 1", hookCalls)
	}
	if !params.PromptCacheHintFallback.Disabled() {
		t.Fatal("PromptCacheHintFallback should be disabled after explicit rejection")
	}
	if len(handler.errors) != 0 {
		t.Fatalf("não deveria emitir erro ao usuário (fallback transparente): %v", handler.errors)
	}
	if handler.done != 1 {
		t.Fatalf("OnDone = %d, want 1", handler.done)
	}
}

func TestOpenAIProvider_StreamChatResponses_PromptCacheHintUnsupportedWithoutKeyEmitsError(t *testing.T) {
	provider := &OpenAIProvider{
		provider:     &ProviderConfig{ID: "o", Name: "Proxy", BaseURL: "http://proxy.local/v1"},
		useResponses: true,
	}
	provider.responsesAttemptFn = func(_ context.Context, _ responses.ResponseNewParams, _ StreamHandler, _ []MCPServerConfig, _ ChatParams, _ *DebugDumpHandle) mcpStreamAttemptResult {
		return mcpStreamAttemptResult{promptCacheHintUnsupported: true}
	}

	handler := &providerRetryHandler{}
	provider.streamChatResponses(context.Background(), "gpt-test", []Message{{Role: "user", Content: "oi"}}, ChatParams{}, handler)

	if len(handler.errors) != 1 {
		t.Fatalf("errors = %v, want one explicit error", handler.errors)
	}
	if !strings.Contains(handler.errors[0], "provider_hints") || !strings.Contains(handler.errors[0], "gateway/proxy") {
		t.Fatalf("erro = %q, want actionable provider_hints + gateway/proxy guidance", handler.errors[0])
	}
	if handler.done != 0 {
		t.Fatalf("OnDone = %d, want 0", handler.done)
	}
}

// TestOpenAIProvider_StreamChatResponses_NativeMCPUnsupportedAbortsWhenFallbackSet
// cobre o caminho preferencial: quando há NativeMCPFallback configurado (o loop
// agêntico re-tenta em adapter com bridges), o provider apenas dispara o
// Trigger/hook e ABORTA — sem fazer o retry "pelado" interno (sem 2ª tentativa).
func TestOpenAIProvider_StreamChatResponses_NativeMCPUnsupportedAbortsWhenFallbackSet(t *testing.T) {
	attempts := 0
	hookCalls := 0

	provider := &OpenAIProvider{
		provider:     &ProviderConfig{ID: "o", Name: "Proxy", BaseURL: "http://proxy.local/v1"},
		useResponses: true,
		mcpServers: []MCPServerConfig{
			{Name: "Atlassian", Slug: "atlassian", URL: "https://mcp.atlassian.com/v1/sse"},
		},
	}
	provider.responsesAttemptFn = func(_ context.Context, _ responses.ResponseNewParams, _ StreamHandler, _ []MCPServerConfig, _ ChatParams, _ *DebugDumpHandle) mcpStreamAttemptResult {
		attempts++
		return mcpStreamAttemptResult{nativeMCPUnsupported: true}
	}

	fb := &NativeMCPAdapterFallback{}
	handler := &providerRetryHandler{}
	params := ChatParams{
		OnNativeMCPUnsupported: func() { hookCalls++ },
		NativeMCPFallback:      fb,
	}
	provider.streamChatResponses(context.Background(), "deepseek-v4-flash", []Message{{Role: "user", Content: "oi"}}, params, handler)

	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 (abort sem retry pelado)", attempts)
	}
	if hookCalls != 1 {
		t.Fatalf("OnNativeMCPUnsupported chamado %d vezes, want 1", hookCalls)
	}
	if !fb.Consume() {
		t.Fatal("NativeMCPFallback deveria ter sido disparado (Trigger)")
	}
	if len(handler.errors) != 0 {
		t.Fatalf("não deveria emitir erro (caller re-tenta): %v", handler.errors)
	}
	if handler.done != 0 {
		t.Fatalf("não deveria emitir done (abortou para retry upstream): %d", handler.done)
	}
}
