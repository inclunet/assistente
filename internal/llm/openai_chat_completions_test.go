package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/llmcapabilities"

	"github.com/openai/openai-go"
)

func TestOpenAIProvider_ChatCompletions_NoNativeMCP(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIProvider(p, credMgr)
	if provider.NativeMCPCapable() {
		t.Error("Chat Completions provider should NOT be native MCP capable")
	}
}

func TestOpenAICompatible_WithMCPServers_NoOp(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openrouter.ai/v1",
	}
	provider := NewOpenAIProvider(p, credMgr)
	servers := []MCPServerConfig{
		{Name: "test-mcp", URL: "https://mcp.test.com"},
	}
	result := provider.WithMCPServers(servers)
	if result != provider {
		t.Error("Chat Completions provider.WithMCPServers should return same provider (no-op)")
	}
}

func TestOpenAIChatCompletionsRetriesAfterPersistingUnsupportedField(t *testing.T) {
	var attempts int
	var temperatureSent []bool
	compatibility := NewCompatibilityState("model-1", 7, nil, func(_ context.Context, capability llmcapabilities.Capability, field llmcapabilities.FieldKey, recognizer string) error {
		if capability != llmcapabilities.CapabilityChatCompletions || field != llmcapabilities.FieldTemperature || recognizer != "provider-error:openai:openai:v1:unsupported_parameter" {
			t.Fatalf("persistência incompatível: capability=%s field=%s recognizer=%q", capability, field, recognizer)
		}
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decodificar request: %v", err)
		}
		_, sent := body["temperature"]
		temperatureSent = append(temperatureSent, sent)
		if attempts == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"temperature is not supported","type":"invalid_request_error","param":"temperature","code":"unsupported_parameter"}}`))
			return
		}
		if !compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature) {
			t.Errorf("retry iniciado antes da persistência da restrição")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n" + "data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-compatibility", Name: "Chat Compatibility", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{
		Temperature: 0.4, Compatibility: compatibility,
	}, handler)

	if attempts != 2 || len(temperatureSent) != 2 || !temperatureSent[0] || temperatureSent[1] {
		t.Fatalf("tentativas=%d temperatureSent=%v; esperado [true false]", attempts, temperatureSent)
	}
	if len(handler.errors) != 0 || handler.done != 1 {
		t.Fatalf("erros=%v done=%d; esperado retry concluído", handler.errors, handler.done)
	}
}

func TestOpenAIChatCompletionsAllowsOnlyOneCompatibilityRetry(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"temperature is not supported","type":"invalid_request_error","param":"temperature","code":"unsupported_parameter"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-one-retry", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Temperature: 0.4, Compatibility: compatibility}, handler)

	if attempts != 2 || persistCalls != 1 || !compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature) || len(handler.errors) != 1 {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v erros=%v; esperado uma única repetição", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature), handler.errors)
	}
}

func TestOpenAIChatCompletionsDoesNotRetryAfterToolEffectAcrossIterations(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"lookup\",\"arguments\":\"{}\"}}]}}]}\n\n" + "data: {\"id\":\"chatcmpl-tool\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n" + "data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"temperature is not supported","type":"invalid_request_error","param":"temperature","code":"unsupported_parameter"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-tool-effect", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	params := ChatParams{Temperature: 0.4, Compatibility: compatibility}
	tool := ToolDefinition{Type: "function", Function: FunctionDefinition{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, params, handler, tool)
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "resultado de lookup"}}, params, handler, tool)

	if attempts != 2 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature))
	}
	if handler.tools != 1 || len(handler.errors) != 1 {
		t.Fatalf("tools=%d erros=%v; esperado uma tool e falha sem retry", handler.tools, handler.errors)
	}
}

func TestOpenAIChatCompletionsDoesNotRetryAfterVisibleContentAcrossIterations(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-content\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n" + "data: {\"id\":\"chatcmpl-content\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" + "data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"temperature is not supported","type":"invalid_request_error","param":"temperature","code":"unsupported_parameter"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-content-effect", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	params := ChatParams{Temperature: 0.4, Compatibility: compatibility}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, params, handler)
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "continuação"}}, params, handler)

	if attempts != 2 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature))
	}
	if handler.chunks.String() != "partial" || len(handler.errors) != 1 {
		t.Fatalf("conteúdo=%q erros=%v; esperado conteúdo parcial e erro sem retry", handler.chunks.String(), handler.errors)
	}
}

func TestOpenAIChatCompletionsDoesNotRetryAfterReasoningAcrossIterations(t *testing.T) {
	attempts := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		t.Fatal("reasoning exposto não deve persistir restrição nem repetir")
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-reasoning\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"reasoning\"}}]}\n\n" + "data: {\"id\":\"chatcmpl-reasoning\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" + "data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"temperature is not supported","type":"invalid_request_error","param":"temperature","code":"unsupported_parameter"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-reasoning-effect", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
		ReasoningContentMode: ReasoningContentReplayWithTools,
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	params := ChatParams{Temperature: 0.4, Compatibility: compatibility}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, params, handler)
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "continuação"}}, params, handler)

	if attempts != 2 || compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d campoSuprimido=%v", attempts, compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature))
	}
	if handler.thinking.String() != "reasoning" || len(handler.errors) != 1 {
		t.Fatalf("reasoning=%q erros=%v; esperado reasoning entregue e erro sem retry", handler.thinking.String(), handler.errors)
	}
}

func TestOpenAIChatCompletionsDoesNotRetryAfterReasoningOnIdleTimeout(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts > 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-retried\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"duplicado\"},\"finish_reason\":\"stop\"}]}\n\n" + "data: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-reasoning-timeout\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"reasoning_content\":\"reasoning parcial\"}}]}\n\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-reasoning-timeout", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
		ReasoningContentMode: ReasoningContentReplayWithTools, StreamIdleTimeoutSeconds: 1,
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{}, handler)

	if attempts != 1 || handler.thinking.String() != "reasoning parcial" || handler.chunks.Len() != 0 {
		t.Fatalf("tentativas=%d reasoning=%q conteúdo=%q; esperado uma tentativa sem retry", attempts, handler.thinking.String(), handler.chunks.String())
	}
	if len(handler.errors) != 1 {
		t.Fatalf("erros=%v; esperado erro de timeout após reasoning", handler.errors)
	}
}

func TestOpenAIChatCompletionsDoesNotRetryWhenPersistingRestrictionFails(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return context.Canceled
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"temperature is not supported","type":"invalid_request_error","param":"temperature","code":"unsupported_parameter"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-persist-failure", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Temperature: 0.4, Compatibility: compatibility}, handler)

	if attempts != 1 || persistCalls != 1 || compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature))
	}
}

func TestOpenAIChatCompletionsCancellationDoesNotRetryAfterPartialOutput(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-cancel\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}]}\n\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-cancel-after-content", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &cancelOnChunkRetryHandler{cancel: cancel}
	provider.StreamChat(ctx, []Message{{Role: "user", Content: "oi"}}, ChatParams{}, handler)

	if attempts != 1 || len(handler.errors) != 1 || !strings.Contains(handler.errors[0], "cancelado") || handler.chunks.String() != "partial" {
		t.Fatalf("tentativas=%d erros=%v conteúdo=%q; cancelamento deve encerrar sem retry", attempts, handler.errors, handler.chunks.String())
	}
}

func TestOpenAIChatCompletionsDoesNotRetryUnsupportedValue(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decodificar payload: %v", err)
		}
		if _, sent := body["temperature"]; !sent {
			t.Error("a requisição deveria manter o campo recusado por valor")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"temperature value is outside the allowed range","type":"invalid_request_error","param":"temperature","code":"unsupported_value"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-invalid-value", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{
		Temperature: 0.4, Compatibility: compatibility,
	}, handler)

	if attempts != 1 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTemperature))
	}
	if len(handler.errors) != 1 || !strings.Contains(handler.errors[0], "provider_error:v1:invalid_value:temperature") {
		t.Fatalf("diagnóstico de valor recusado = %v", handler.errors)
	}
}

func TestOpenAIChatCompletionsDoesNotLearnUnsupportedUnsentField(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decodificar payload: %v", err)
		}
		if _, sent := body["top_p"]; sent {
			t.Error("top_p default não deveria ter sido enviado")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","param":"top_p","code":"unsupported_parameter"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-unsent-field", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Compatibility: compatibility}, handler)

	if attempts != 1 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTopP) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldTopP))
	}
}

func TestOpenAIChatCompletionsDoesNotRetryAmbiguousOutputTokenAlias(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decodificar payload: %v", err)
		}
		if _, sent := body["max_tokens"]; !sent {
			t.Error("fixture exige o alias max_tokens na primeira tentativa")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","param":"max_tokens","code":"unsupported_parameter"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID: "chat-ambiguous-alias", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAI, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{
		MaxTokens: 32, MaxTokensMode: "max_tokens", Compatibility: compatibility,
	}, handler)

	if attempts != 1 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldMaxOutputTokens) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityChatCompletions, llmcapabilities.FieldMaxOutputTokens))
	}
}

// TestOpenAICompatible_NativeMCPCapable_False garante que Chat Completions não é
// fisicamente capaz de MCP nativo (um override de perfil "true" não o habilita).
func TestOpenAICompatible_NativeMCPCapable_False(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := NewOpenAIProvider(&ProviderConfig{ID: "oc", Name: "OC", BaseURL: "https://openrouter.ai/api/v1"}, credMgr)
	if p.NativeMCPCapable() {
		t.Error("Chat Completions NÃO é capaz de emitir type:mcp")
	}
}

func TestOpenAICompatProvider_UsesChatCompletions(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "ollama",
		Name:    "Ollama",
		BaseURL: "http://localhost:11434/v1",
	}
	provider := NewOpenAIProvider(p, credMgr)

	if provider.useResponses {
		t.Error("Compatible provider should NOT use Responses API")
	}
	if provider.NativeMCPCapable() {
		t.Error("Compatible provider should NOT be native MCP capable")
	}
}

func TestOpenAIProvider_ReasoningContentReplayDependeSoDaCapability(t *testing.T) {
	tests := []struct {
		name string
		cfg  ProviderConfig
		want bool
	}{
		{
			name: "capability habilitada em endpoint customizado",
			cfg: ProviderConfig{
				Type: ProviderCustom, BaseURL: "https://proxy.example/v1",
				ReasoningContentMode: ReasoningContentReplayWithTools,
			},
			want: true,
		},
		{
			name: "marca e endpoint não habilitam implicitamente",
			cfg:  ProviderConfig{Type: ProviderDeepSeek, BaseURL: "https://api.deepseek.com/v1"},
			want: false,
		},
		{
			name: "capability desabilitada",
			cfg: ProviderConfig{
				Type: ProviderCustom, BaseURL: "https://api.deepseek.com/v1",
				ReasoningContentMode: ReasoningContentDisabled,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &OpenAIProvider{provider: &tt.cfg}
			if got := provider.ReplaysReasoningContent(); got != tt.want {
				t.Fatalf("ReplaysReasoningContent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChatCompletionReasoningContentReadsProviderExtension(t *testing.T) {
	var chunk openai.ChatCompletionChunk
	raw := `{
		"id": "chatcmpl-deepseek",
		"object": "chat.completion.chunk",
		"created": 1710000000,
		"model": "deepseek-reasoner",
		"choices": [{
			"index": 0,
			"delta": {
				"role": "assistant",
				"content": null,
				"reasoning_content": "preciso consultar a ferramenta"
			}
		}]
	}`
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		t.Fatalf("json.Unmarshal chunk: %v", err)
	}

	got := chatCompletionReasoningContent(chunk.Choices[0].Delta)
	if got != "preciso consultar a ferramenta" {
		t.Fatalf("reasoning_content = %q, want %q", got, "preciso consultar a ferramenta")
	}
}

func TestChatCompletionStreamUsageExtras_PreservesCacheMetrics(t *testing.T) {
	var chunk openai.ChatCompletionChunk
	raw := `{
		"id": "chatcmpl-test",
		"object": "chat.completion.chunk",
		"created": 1710000000,
		"model": "gpt-test",
		"choices": [],
		"usage": {
			"prompt_tokens": 1000,
			"completion_tokens": 120,
			"total_tokens": 1120,
			"prompt_tokens_details": {
				"cached_tokens": 400
			},
			"completion_tokens_details": {
				"reasoning_tokens": 70
			},
			"cache_write_tokens": 80
		}
	}`
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		t.Fatalf("json.Unmarshal chunk: %v", err)
	}

	acc := openai.ChatCompletionAccumulator{}
	if ok := acc.AddChunk(chunk); !ok {
		t.Fatal("AddChunk returned false")
	}

	var promptDetails openai.CompletionUsagePromptTokensDetails
	var usageRawJSON string
	accumulateChatCompletionStreamUsageExtras(&promptDetails, chunk, &usageRawJSON)

	cachedTokens := acc.Usage.PromptTokensDetails.CachedTokens
	if cachedTokens == 0 {
		cachedTokens = promptDetails.CachedTokens
	}
	usage := UsageFromOpenAICompletion(
		int(acc.Usage.PromptTokens),
		int(acc.Usage.CompletionTokens),
		int(acc.Usage.TotalTokens),
		int(cachedTokens),
		usageRawJSON,
	)

	if usage.CacheReadTokens != 400 {
		t.Fatalf("CacheReadTokens=%d, want 400", usage.CacheReadTokens)
	}
	if usage.CacheWriteTokens != 80 {
		t.Fatalf("CacheWriteTokens=%d, want 80", usage.CacheWriteTokens)
	}
	if usage.CacheMissTokens != 600 {
		t.Fatalf("CacheMissTokens=%d, want 600", usage.CacheMissTokens)
	}
	if !usage.Reported || !usage.ReasoningTokensReported || usage.ReasoningTokens != 70 {
		t.Fatalf("usage de reasoning não preservada: %#v", usage)
	}
}
