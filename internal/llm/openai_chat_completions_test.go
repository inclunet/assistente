package llm

import (
	"encoding/json"
	"testing"

	"assistente/internal/credentials"

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
}
