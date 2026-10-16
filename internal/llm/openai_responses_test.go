package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llmcapabilities"
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
	err     string
	content string
	usage   Usage
	finish  FinishInfo
}

func (h *noopStreamHandler) OnChunk(content string) { h.content += content }

func (h *noopStreamHandler) OnThinking(string) {}

func (h *noopStreamHandler) OnThinkingDone(string) {}

func (h *noopStreamHandler) OnToolCalls([]ToolCall, string, Usage, string) {}

func (h *noopStreamHandler) OnError(err string) { h.err = err }

func (h *noopStreamHandler) OnDone(_ string, usage Usage, _ string) { h.usage = usage }

func (h *noopStreamHandler) OnMCPToolEvent(MCPToolEvent)    {}
func (h *noopStreamHandler) OnFinishReason(info FinishInfo) { h.finish = info }

type resetCountingHandler struct {
	noopStreamHandler
	resets int
}

func (h *resetCountingHandler) ResetStreamAttempt() {
	h.resets++
}

func TestOpenAIResponsesResetaHandlerAntesDeNovaTentativa(t *testing.T) {
	attempts := 0
	provider := &OpenAIProvider{
		provider:     &ProviderConfig{ID: "o", Name: "Proxy", BaseURL: "http://proxy.local/v1"},
		useResponses: true,
	}
	provider.responsesAttemptFn = func(_ context.Context, _ responses.ResponseNewParams, handler StreamHandler, _ []MCPServerConfig, _ ChatParams, _ *DebugDumpHandle) mcpStreamAttemptResult {
		attempts++
		if attempts == 1 {
			handler.OnThinking("descartado")
			return mcpStreamAttemptResult{promptCacheHintUnsupported: true}
		}
		handler.OnDone("ok", Usage{}, "gpt-test")
		return mcpStreamAttemptResult{done: true}
	}

	handler := &resetCountingHandler{}
	params := ChatParams{
		PromptCacheKey:          "asst-key",
		PromptCacheHintFallback: &PromptCacheHintFallback{},
	}
	provider.streamChatResponses(context.Background(), "gpt-test", []Message{{Role: "user", Content: "oi"}}, params, handler)

	if attempts != 2 || handler.resets != 1 {
		t.Fatalf("attempts=%d resets=%d, esperado 2/1", attempts, handler.resets)
	}
}

func TestOpenAIResponsesRetriesAfterPersistingUnsupportedField(t *testing.T) {
	var attempts int
	var temperatureSent []bool
	compatibility := NewCompatibilityState("model-1", 7, nil, func(_ context.Context, capability llmcapabilities.Capability, field llmcapabilities.FieldKey, recognizer string) error {
		if capability != llmcapabilities.CapabilityResponses || field != llmcapabilities.FieldTemperature || recognizer != "provider-error:openai:openai_responses:v1:unsupported_parameter" {
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
		w.Header().Set("Content-Type", "text/event-stream")
		if attempts == 1 {
			_, _ = w.Write([]byte(`event: response.failed
data: {"type":"response.failed","sequence_number":1,"response":{"id":"resp_1","object":"response","status":"failed","error":{"code":"unsupported_parameter","message":"temperature is not supported","param":"temperature","type":"invalid_request_error"}}}

`))
			return
		}
		if !compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature) {
			t.Errorf("retry iniciado antes da persistência da restrição")
		}
		_, _ = w.Write([]byte(`event: response.completed
data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_2","object":"response","status":"completed","model":"gpt-test","output":[]}}

`))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-compatibility", Name: "Responses Compatibility", Type: ProviderOpenAI,
		ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL:     server.URL + "/v1", APIFormat: APIFormatOpenAIResponses,
		AuthMode: AuthModeNone, DefaultModel: "gpt-test",
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

func TestOpenAIResponsesAllowsOnlyOneCompatibilityRetry(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"sequence_number\":1,\"response\":{\"id\":\"resp_failed\",\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"unsupported_parameter\",\"message\":\"temperature is not supported\",\"param\":\"temperature\",\"type\":\"invalid_request_error\"}}}\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-one-retry", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "gpt-test", Temperature: 0.4, Compatibility: compatibility}, handler)

	if attempts != 2 || persistCalls != 1 || !compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature) || len(handler.errors) != 1 {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v erros=%v; esperado uma única repetição", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature), handler.errors)
	}
}

func TestOpenAIResponsesDoesNotRetryAfterToolEffectAcrossIterations(t *testing.T) {
	attempts := 0
	persistCalls := 0
	var temperatureSent []bool
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decodificar payload: %v", err)
		}
		_, sent := body["temperature"]
		temperatureSent = append(temperatureSent, sent)
		w.Header().Set("Content-Type", "text/event-stream")
		if attempts == 1 {
			_, _ = w.Write([]byte("event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"sequence_number\":1,\"output_index\":0,\"item\":{\"id\":\"fc_1\",\"type\":\"function_call\",\"status\":\"in_progress\",\"call_id\":\"call_1\",\"name\":\"lookup\",\"arguments\":\"\"}}\n\nevent: response.function_call_arguments.done\ndata: {\"type\":\"response.function_call_arguments.done\",\"sequence_number\":2,\"item_id\":\"fc_1\",\"output_index\":0,\"arguments\":\"{}\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":3,\"response\":{\"id\":\"resp_tool\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-test\",\"output\":[]}}\n\n"))
			return
		}
		_, _ = w.Write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"sequence_number\":1,\"response\":{\"id\":\"resp_failed\",\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"unsupported_parameter\",\"message\":\"temperature is not supported\",\"param\":\"temperature\",\"type\":\"invalid_request_error\"}}}\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-tool-effect", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	params := ChatParams{Model: "gpt-test", Temperature: 0.4, Compatibility: compatibility}
	tool := ToolDefinition{Type: "function", Function: FunctionDefinition{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, params, handler, tool)
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "resultado de lookup"}}, params, handler, tool)

	if attempts != 2 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature))
	}
	if handler.tools != 1 || len(handler.errors) != 1 || len(temperatureSent) != 2 || !temperatureSent[0] || !temperatureSent[1] {
		t.Fatalf("tools=%d erros=%v temperatureSent=%v; esperado tool executada e parâmetro mantido sem retry", handler.tools, handler.errors, temperatureSent)
	}
}

func TestOpenAIResponsesDoesNotRetryAfterVisibleContentAcrossIterations(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		if attempts == 1 {
			_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"partial\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_content\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-test\",\"output\":[]}}\n\n"))
			return
		}
		_, _ = w.Write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"sequence_number\":1,\"response\":{\"id\":\"resp_failed\",\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"unsupported_parameter\",\"message\":\"temperature is not supported\",\"param\":\"temperature\",\"type\":\"invalid_request_error\"}}}\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-content-effect", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	params := ChatParams{Model: "gpt-test", Temperature: 0.4, Compatibility: compatibility}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, params, handler)
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "continuação"}}, params, handler)

	if attempts != 2 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature))
	}
	if handler.chunks.String() != "partial" || len(handler.errors) != 1 {
		t.Fatalf("conteúdo=%q erros=%v; esperado conteúdo parcial e erro sem retry", handler.chunks.String(), handler.errors)
	}
}

func TestOpenAIResponsesDoesNotRetryAfterReasoningAcrossIterations(t *testing.T) {
	attempts := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		t.Fatal("reasoning exposto não deve persistir restrição nem repetir")
		return nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		if attempts == 1 {
			_, _ = w.Write([]byte("event: response.reasoning_summary_text.delta\ndata: {\"type\":\"response.reasoning_summary_text.delta\",\"sequence_number\":1,\"item_id\":\"reasoning_1\",\"output_index\":0,\"summary_index\":0,\"delta\":\"reasoning\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_reasoning\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-test\",\"output\":[]}}\n\n"))
			return
		}
		_, _ = w.Write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"sequence_number\":1,\"response\":{\"id\":\"resp_failed\",\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"unsupported_parameter\",\"message\":\"temperature is not supported\",\"param\":\"temperature\",\"type\":\"invalid_request_error\"}}}\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-reasoning-effect", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	params := ChatParams{Model: "gpt-test", Temperature: 0.4, Compatibility: compatibility}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, params, handler)
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "continuação"}}, params, handler)

	if attempts != 2 || compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d campoSuprimido=%v", attempts, compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature))
	}
	if handler.thinking.String() != "reasoning" || len(handler.errors) != 1 {
		t.Fatalf("reasoning=%q erros=%v; esperado reasoning entregue e erro sem retry", handler.thinking.String(), handler.errors)
	}
}

func TestOpenAIResponsesDoesNotRetryWhenPersistingRestrictionFails(t *testing.T) {
	attempts := 0
	persistCalls := 0
	compatibility := NewCompatibilityState("model-1", 7, nil, func(context.Context, llmcapabilities.Capability, llmcapabilities.FieldKey, string) error {
		persistCalls++
		return context.Canceled
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"sequence_number\":1,\"response\":{\"id\":\"resp_failed\",\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"unsupported_parameter\",\"message\":\"temperature is not supported\",\"param\":\"temperature\",\"type\":\"invalid_request_error\"}}}\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-persist-failure", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "gpt-test", Temperature: 0.4, Compatibility: compatibility}, handler)

	if attempts != 1 || persistCalls != 1 || compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature))
	}
}

func TestOpenAIResponsesCancellationDoesNotRetryAfterPartialOutput(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"partial\"}\n\n"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-cancel-after-content", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &cancelOnChunkRetryHandler{cancel: cancel}
	provider.StreamChat(ctx, []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "gpt-test"}, handler)

	if attempts != 1 || len(handler.errors) != 1 || !strings.Contains(handler.errors[0], "cancelado") || handler.chunks.String() != "partial" {
		t.Fatalf("tentativas=%d erros=%v conteúdo=%q; cancelamento deve encerrar sem retry", attempts, handler.errors, handler.chunks.String())
	}
}

func TestOpenAIResponsesDoesNotRetryOrPersistUnsupportedValue(t *testing.T) {
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
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"sequence_number\":1,\"response\":{\"id\":\"resp_invalid\",\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"unsupported_value\",\"message\":\"temperature value is outside the allowed range\",\"param\":\"temperature\",\"type\":\"invalid_request_error\"}}}\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-invalid-value", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{
		Temperature: 0.4, Compatibility: compatibility,
	}, handler)

	if attempts != 1 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldTemperature))
	}
	if len(handler.errors) != 1 || !strings.Contains(handler.errors[0], "provider_error:v1:invalid_value:temperature") {
		t.Fatalf("diagnóstico de valor recusado = %v", handler.errors)
	}
}

func TestOpenAIResponsesDoesNotLearnUnknownOutputTokenAlias(t *testing.T) {
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
		if _, sent := body["max_output_tokens"]; !sent {
			t.Error("Responses deveria enviar max_output_tokens")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"sequence_number\":1,\"response\":{\"id\":\"resp_alias\",\"object\":\"response\",\"status\":\"failed\",\"error\":{\"code\":\"unsupported_parameter\",\"message\":\"max_tokens is not supported\",\"param\":\"max_tokens\",\"type\":\"invalid_request_error\"}}}\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-unknown-alias", Name: "OpenAI", Type: ProviderOpenAI, ErrorFamily: ProviderErrorFamilyOpenAI,
		BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone, DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &providerRetryHandler{}
	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{
		MaxTokens: 32, Compatibility: compatibility,
	}, handler)

	if attempts != 1 || persistCalls != 0 || compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldMaxOutputTokens) {
		t.Fatalf("tentativas=%d persistências=%d campoSuprimido=%v", attempts, persistCalls, compatibility.IsUnsupported(llmcapabilities.CapabilityResponses, llmcapabilities.FieldMaxOutputTokens))
	}
}

func TestOpenAIResponsesPropagaLimiteComDiagnostico(t *testing.T) {
	const stream = "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"ok\"}\n\n" +
		"event: response.incomplete\n" +
		"data: {\"type\":\"response.incomplete\",\"sequence_number\":2,\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1,\"status\":\"incomplete\",\"output\":[],\"incomplete_details\":{\"reason\":\"max_output_tokens\"},\"usage\":{\"input_tokens\":3,\"output_tokens\":7,\"output_tokens_details\":{\"reasoning_tokens\":5}}}}\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID:           "responses-test",
		Name:         "Responses Test",
		BaseURL:      server.URL + "/v1",
		APIFormat:    APIFormatOpenAIResponses,
		AuthMode:     AuthModeNone,
		DefaultModel: "gpt-test",
	}, credentials.NewManager(nil))
	handler := &noopStreamHandler{}

	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}},
		ChatParams{MaxTokens: 99}, handler)

	if handler.err != "" {
		t.Fatalf("stream falhou: %s", handler.err)
	}
	if got := handler.finish; got.Reason != FinishReasonMaxTokens || got.RawReason != "max_output_tokens" ||
		got.Provider != "responses-test" || got.Model != "gpt-test" || got.OutputLimit != 99 || got.ResponseBytes != 2 {
		t.Fatalf("diagnóstico de término incompleto: %#v", got)
	}
	if !handler.usage.Reported || !handler.usage.ReasoningTokensReported ||
		handler.usage.CompletionTokens != 7 || handler.usage.ReasoningTokens != 5 ||
		handler.usage.TotalTokens != 10 {
		t.Fatalf("usage de output/reasoning não preservada: %#v", handler.usage)
	}
}

func TestOpenAIResponsesCompletedNaoInventaRawReason(t *testing.T) {
	const stream = "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":1,\"item_id\":\"msg_1\",\"output_index\":0,\"content_index\":0,\"delta\":\"ok\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1,\"status\":\"completed\",\"model\":\"gpt-test\",\"output\":[]}}\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	provider := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "responses-completed", Name: "Responses Completed", BaseURL: server.URL + "/v1",
		APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone,
	}, credentials.NewManager(nil))
	handler := &noopStreamHandler{}

	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}},
		ChatParams{Model: "gpt-test"}, handler)

	if handler.err != "" {
		t.Fatalf("stream falhou: %s", handler.err)
	}
	if handler.finish.Reason != FinishReasonStop || handler.finish.RawReason != "" {
		t.Fatalf("response.completed inventou motivo bruto: %#v", handler.finish)
	}
}

func TestOpenAIResponsesStreamInjectsScopedCredential(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		http.Error(w, "stop after auth capture", http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx := database.WithUserID(context.Background(), "user-1")
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	if err := credMgr.RegisterPatternWithContext(ctx, "llm.inclunet.com.br", &credentials.AuthConfig{Source: "static",
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
	if handler.errors[0] != streamPromptCacheHintRejectedError {
		t.Fatalf("erro = %q, want %q", handler.errors[0], streamPromptCacheHintRejectedError)
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
