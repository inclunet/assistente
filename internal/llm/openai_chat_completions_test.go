package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		ID: "chat-compatibility", Name: "Chat Compatibility", Type: ProviderOpenAI,
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
