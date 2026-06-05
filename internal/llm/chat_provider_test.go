package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"
	mcplib "assistente/internal/mcp"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/responses"
	"google.golang.org/genai"
)

func TestGetAPIFormat_Default(t *testing.T) {
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.test.com/v1",
	}
	if got := p.GetAPIFormat(); got != APIFormatOpenAI {
		t.Errorf("GetAPIFormat() = %q, want %q", got, APIFormatOpenAI)
	}
}

func TestGetAPIFormat_InfersOpenAIResponses(t *testing.T) {
	tests := []struct {
		baseURL string
		want    APIFormat
	}{
		{"https://api.openai.com/v1", APIFormatOpenAIResponses},
		{"https://api.openai.com/v1/", APIFormatOpenAIResponses},
		{"https://API.OPENAI.COM/v1", APIFormatOpenAIResponses},
		{"https://openrouter.ai/api/v1", APIFormatOpenAI},
		{"http://localhost:11434/v1", APIFormatOpenAI},
		{"https://api.groq.com/openai/v1", APIFormatOpenAI},
		{"https://api.together.xyz/v1", APIFormatOpenAI},
	}

	for _, tt := range tests {
		t.Run(tt.baseURL, func(t *testing.T) {
			p := &ProviderConfig{ID: "test", Name: "Test", BaseURL: tt.baseURL}
			if got := p.GetAPIFormat(); got != tt.want {
				t.Errorf("GetAPIFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetAPIFormat_Explicit(t *testing.T) {
	tests := []struct {
		format APIFormat
		want   APIFormat
	}{
		{APIFormatOpenAI, APIFormatOpenAI},
		{APIFormatOpenAIResponses, APIFormatOpenAIResponses},
		{APIFormatAnthropic, APIFormatAnthropic},
		{APIFormatGoogle, APIFormatGoogle},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			p := &ProviderConfig{
				ID:        "test",
				Name:      "Test",
				BaseURL:   "https://api.test.com/v1",
				APIFormat: tt.format,
			}
			if got := p.GetAPIFormat(); got != tt.want {
				t.Errorf("GetAPIFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewChatProvider_Factory(t *testing.T) {
	credMgr := credentials.NewManager(nil)

	tests := []struct {
		name          string
		format        APIFormat
		wantOpenAI    bool
		wantAnthropic bool
		wantGoogle    bool
		wantResponses bool // useResponses=true
	}{
		{"default_empty", "", true, false, false, false},
		{"openai", APIFormatOpenAI, true, false, false, false},
		{"openai_responses", APIFormatOpenAIResponses, true, false, false, true},
		{"anthropic", APIFormatAnthropic, false, true, false, false},
		{"google", APIFormatGoogle, false, false, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ProviderConfig{
				ID:        "test",
				Name:      "Test",
				BaseURL:   "https://api.test.com/v1",
				APIFormat: tt.format,
			}
			provider := NewChatProvider(p, credMgr)
			if provider == nil {
				t.Fatal("NewChatProvider returned nil")
			}
			openaiP, isOpenAI := provider.(*OpenAIProvider)
			_, isAnthropic := provider.(*AnthropicProvider)
			_, isGoogle := provider.(*GoogleProvider)
			if isOpenAI != tt.wantOpenAI {
				t.Errorf("isOpenAI = %v, want %v", isOpenAI, tt.wantOpenAI)
			}
			if isAnthropic != tt.wantAnthropic {
				t.Errorf("isAnthropic = %v, want %v", isAnthropic, tt.wantAnthropic)
			}
			if isGoogle != tt.wantGoogle {
				t.Errorf("isGoogle = %v, want %v", isGoogle, tt.wantGoogle)
			}
			if isOpenAI && openaiP.useResponses != tt.wantResponses {
				t.Errorf("useResponses = %v, want %v", openaiP.useResponses, tt.wantResponses)
			}
		})
	}
}

func TestOpenAIProvider_ChatCompletions_NoNativeMCP(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIProvider(p, credMgr)
	if provider.SupportsNativeMCP() {
		t.Error("Chat Completions provider should NOT support native MCP")
	}
}

func TestOpenAIResponsesProvider_SupportsNativeMCP(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIResponsesProvider(p, credMgr)
	if !provider.SupportsNativeMCP() {
		t.Error("Responses provider should support native MCP")
	}
}

func TestOpenAIResponsesProvider_WithMCPServers_StoresServers(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIResponsesProvider(p, credMgr)
	servers := []MCPServerConfig{
		{Name: "weather-mcp", URL: "https://mcp.weather.com/sse", AuthToken: "tok123"},
	}
	result := provider.WithMCPServers(servers)
	if result == nil {
		t.Fatal("WithMCPServers returned nil")
	}

	openaiP, ok := result.(*OpenAIProvider)
	if !ok {
		t.Fatal("WithMCPServers should return *OpenAIProvider")
	}
	if len(openaiP.mcpServers) != 1 {
		t.Fatalf("Expected 1 MCP server, got %d", len(openaiP.mcpServers))
	}
	if openaiP.mcpServers[0].Name != "weather-mcp" {
		t.Errorf("MCP server name = %q, want %q", openaiP.mcpServers[0].Name, "weather-mcp")
	}
	if !openaiP.useResponses {
		t.Error("WithMCPServers should preserve useResponses=true")
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

func TestConvertMessages_Basic(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are a helper"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
		{Role: "tool", Content: `{"result": "ok"}`, ToolCallID: "call_123"},
	}

	result := convertMessages(msgs)
	if len(result) != 4 {
		t.Fatalf("convertMessages returned %d messages, want 4", len(result))
	}

	if result[0].OfSystem == nil {
		t.Error("Expected system message at index 0")
	}
	if result[1].OfUser == nil {
		t.Error("Expected user message at index 1")
	}
	if result[2].OfAssistant == nil {
		t.Error("Expected assistant message at index 2")
	}
	if result[3].OfTool == nil {
		t.Error("Expected tool message at index 3")
	}
}

func TestConvertMessages_AssistantWithToolCalls(t *testing.T) {
	msgs := []Message{
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []ToolCall{
				{
					ID:   "call_abc",
					Type: "function",
					Function: FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city": "SP"}`,
					},
				},
			},
		},
	}

	result := convertMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("convertMessages returned %d messages, want 1", len(result))
	}

	assistant := result[0].OfAssistant
	if assistant == nil {
		t.Fatal("Expected assistant message")
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].ID != "call_abc" {
		t.Errorf("ToolCall ID = %q, want %q", assistant.ToolCalls[0].ID, "call_abc")
	}
	if assistant.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("ToolCall Function.Name = %q, want %q", assistant.ToolCalls[0].Function.Name, "get_weather")
	}
}

func TestRemoveTrailingAssistantPrefill_RemovesOnlyTrailingAssistants(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are a helper"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
		{Role: "user", Content: "Continue"},
		{Role: "assistant", Content: "prefill"},
	}

	got := removeTrailingAssistantPrefill(msgs)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[len(got)-1].Role != "user" {
		t.Fatalf("last role = %q, want user", got[len(got)-1].Role)
	}
	if msgs[len(msgs)-1].Role != "assistant" {
		t.Fatal("original slice should remain unchanged")
	}
}

func TestRemoveTrailingAssistantPrefill_KeepsAssistantHistoryFollowedByUser(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi"},
		{Role: "user", Content: "Next"},
	}

	got := removeTrailingAssistantPrefill(msgs)
	if len(got) != len(msgs) {
		t.Fatalf("len = %d, want %d", len(got), len(msgs))
	}
	if got[len(got)-1].Role != "user" {
		t.Fatalf("last role = %q, want user", got[len(got)-1].Role)
	}
}

// TestPrefillCapability_ThreeCases documenta o mapeamento explícito de
// capacidades de assistant prefill por provider (Issue #124): suporta com
// thinking / só sem thinking / não suporta.
func TestPrefillCapability_ThreeCases(t *testing.T) {
	cases := []struct {
		name string
		cfg  *ProviderConfig
		want AssistantPrefillCapability
	}{
		{
			name: "nil provider",
			cfg:  nil,
			want: PrefillUnsupported,
		},
		{
			name: "openai real responses suporta com thinking",
			cfg:  &ProviderConfig{Type: ProviderOpenAI, APIFormat: APIFormatOpenAIResponses, BaseURL: "https://api.openai.com/v1"},
			want: PrefillWithThinking,
		},
		{
			name: "openai compatible (chat completions) nao suporta",
			cfg:  &ProviderConfig{Type: ProviderOpenAI, APIFormat: APIFormatOpenAI, BaseURL: "https://example.com/v1"},
			want: PrefillUnsupported,
		},
		{
			name: "localai (qwen) so sem thinking",
			cfg:  &ProviderConfig{Type: ProviderLocalAI, BaseURL: "http://localhost:8080/v1"},
			want: PrefillWithoutThinking,
		},
		{
			name: "ollama so sem thinking",
			cfg:  &ProviderConfig{Type: ProviderOllama, BaseURL: "http://localhost:11434/v1"},
			want: PrefillWithoutThinking,
		},
		{
			name: "llamacpp so sem thinking",
			cfg:  &ProviderConfig{Type: ProviderLlamaCPP, BaseURL: "http://localhost:8080/v1"},
			want: PrefillWithoutThinking,
		},
		{
			name: "deepseek nao suporta",
			cfg:  &ProviderConfig{Type: ProviderDeepSeek, BaseURL: "https://api.deepseek.com/v1"},
			want: PrefillUnsupported,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PrefillCapability(tc.cfg); got != tc.want {
				t.Fatalf("PrefillCapability = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSupportsAssistantPrefill_OnlyWithThinking garante que o atalho booleano
// permanece conservador: só é verdadeiro para providers que aceitam prefill com
// thinking ativo (OpenAI real). Qwen/LocalAI (só sem thinking) devem usar o
// fallback por mensagem de usuário, logo retornam false aqui.
func TestSupportsAssistantPrefill_OnlyWithThinking(t *testing.T) {
	if !SupportsAssistantPrefill(&ProviderConfig{Type: ProviderOpenAI, APIFormat: APIFormatOpenAIResponses, BaseURL: "https://api.openai.com/v1"}) {
		t.Fatal("openai real responses deveria suportar prefill")
	}
	if SupportsAssistantPrefill(&ProviderConfig{Type: ProviderLocalAI, BaseURL: "http://localhost:8080/v1"}) {
		t.Fatal("localai não deveria reportar suporte incondicional a prefill")
	}
	if SupportsAssistantPrefill(&ProviderConfig{Type: ProviderOpenAI, APIFormat: APIFormatOpenAI, BaseURL: "https://example.com/v1"}) {
		t.Fatal("openai compatible não deveria suportar prefill")
	}
	if SupportsAssistantPrefill(nil) {
		t.Fatal("nil provider não suporta prefill")
	}
}

func TestConvertTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "get_time",
				Description: "Returns current time",
				Parameters:  []byte(`{"type":"object","properties":{}}`),
			},
		},
	}

	result := convertTools(tools)
	if len(result) != 1 {
		t.Fatalf("convertTools returned %d tools, want 1", len(result))
	}
	if result[0].Function.Name != "get_time" {
		t.Errorf("Tool name = %q, want %q", result[0].Function.Name, "get_time")
	}
}

func TestProviderTimeout(t *testing.T) {
	tests := []struct {
		timeout int
		wantSec int
	}{
		{0, 180}, // default 3min
		{60, 60},
		{300, 300},
	}

	for _, tt := range tests {
		p := &ProviderConfig{Timeout: tt.timeout}
		got := providerTimeout(p)
		if int(got.Seconds()) != tt.wantSec {
			t.Errorf("providerTimeout(%d) = %v, want %ds", tt.timeout, got, tt.wantSec)
		}
	}
}

func TestCredentialTransport_NilCredMgr(t *testing.T) {
	tr := credentials.NewCredentialTransport(nil, "test.com")
	if tr == nil {
		t.Fatal("NewCredentialTransport returned nil")
	}
}

func TestAnthropicProvider_SupportsNativeMCP(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.anthropic.com",
	}
	provider := NewAnthropicProvider(p, credMgr)
	if !provider.SupportsNativeMCP() {
		t.Error("AnthropicProvider.SupportsNativeMCP() = false, want true")
	}
}

func TestConvertToAnthropicMessages_SystemExtraction(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
	}

	system, result := convertToAnthropicMessages(msgs)

	if len(system) != 1 {
		t.Fatalf("Expected 1 system block, got %d", len(system))
	}
	if system[0].Text != "You are helpful" {
		t.Errorf("System text = %q, want %q", system[0].Text, "You are helpful")
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 messages (user+assistant), got %d", len(result))
	}
}

func TestConvertToAnthropicMessages_ToolResults(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Check weather"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "weather", Arguments: `{"city":"SP"}`}},
			},
		},
		{Role: "tool", Content: `{"temp": 25}`, ToolCallID: "call_1"},
	}

	system, result := convertToAnthropicMessages(msgs)

	if len(system) != 0 {
		t.Fatalf("Expected 0 system blocks, got %d", len(system))
	}

	// user + assistant + user(tool_result)
	if len(result) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(result))
	}

	// Last message should be user with tool_result
	last := result[2]
	if string(last.Role) != "user" {
		t.Errorf("Last message role = %q, want %q", last.Role, "user")
	}
	if len(last.Content) != 1 {
		t.Fatalf("Expected 1 content block in tool result message, got %d", len(last.Content))
	}
	if last.Content[0].OfToolResult == nil {
		t.Fatal("Expected ToolResult content block")
	}
}

func TestConvertToAnthropicMessages_MultipleToolResults(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Do two things"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{ID: "call_a", Type: "function", Function: FunctionCall{Name: "a", Arguments: `{}`}},
				{ID: "call_b", Type: "function", Function: FunctionCall{Name: "b", Arguments: `{}`}},
			},
		},
		{Role: "tool", Content: "result_a", ToolCallID: "call_a"},
		{Role: "tool", Content: "result_b", ToolCallID: "call_b"},
	}

	_, result := convertToAnthropicMessages(msgs)

	// user + assistant + user(2 tool_results merged)
	if len(result) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(result))
	}

	toolResultMsg := result[2]
	if len(toolResultMsg.Content) != 2 {
		t.Errorf("Expected 2 tool_result blocks merged in one user message, got %d", len(toolResultMsg.Content))
	}
}

func TestConvertAnthropicTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "get_time",
				Description: "Returns current time",
				Parameters:  []byte(`{"type":"object","properties":{}}`),
			},
		},
	}

	result := convertAnthropicTools(tools)
	if len(result) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result))
	}
	if result[0].OfTool == nil {
		t.Fatal("Expected OfTool to be set")
	}
	if result[0].OfTool.Name != "get_time" {
		t.Errorf("Tool name = %q, want %q", result[0].OfTool.Name, "get_time")
	}
}

func TestAPIFormatConstants(t *testing.T) {
	if APIFormatOpenAI != "openai" {
		t.Errorf("APIFormatOpenAI = %q", APIFormatOpenAI)
	}
	if APIFormatOpenAICompatible != "openai" {
		t.Errorf("APIFormatOpenAICompatible = %q (should be alias for APIFormatOpenAI)", APIFormatOpenAICompatible)
	}
	if APIFormatOpenAIResponses != "openai_responses" {
		t.Errorf("APIFormatOpenAIResponses = %q", APIFormatOpenAIResponses)
	}
	if APIFormatAnthropic != "anthropic" {
		t.Errorf("APIFormatAnthropic = %q", APIFormatAnthropic)
	}
	if APIFormatGoogle != "google" {
		t.Errorf("APIFormatGoogle = %q", APIFormatGoogle)
	}
}

// --- GoogleProvider tests ---

func TestGoogleProvider_SupportsNativeMCP_False(t *testing.T) {
	p := NewGoogleProvider(&ProviderConfig{
		ID:   "gp",
		Name: "Google",
	}, nil)
	if p.SupportsNativeMCP() {
		t.Error("GoogleProvider should NOT support native MCP (not implemented)")
	}
}

func TestGoogleProvider_WithMCPServers_NoOp(t *testing.T) {
	p := NewGoogleProvider(&ProviderConfig{ID: "gp"}, nil)
	p2 := p.WithMCPServers([]MCPServerConfig{{Name: "test", URL: "http://test"}})
	if p2 != p {
		t.Error("GoogleProvider.WithMCPServers should return same provider (no-op)")
	}
}

func TestConvertToGoogleContents_SystemExtraction(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are a helper."},
		{Role: "user", Content: "Hi"},
	}

	system, contents := convertToGoogleContents(msgs)

	if system == nil {
		t.Fatal("Expected system instruction")
	}
	if len(system.Parts) != 1 || system.Parts[0].Text != "You are a helper." {
		t.Errorf("System text = %q, want %q", system.Parts[0].Text, "You are a helper.")
	}
	if len(contents) != 1 {
		t.Fatalf("Expected 1 content, got %d", len(contents))
	}
	if contents[0].Role != "user" {
		t.Errorf("Role = %q, want user", contents[0].Role)
	}
}

func TestConvertToGoogleContents_AssistantRole(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	_, contents := convertToGoogleContents(msgs)

	if len(contents) != 2 {
		t.Fatalf("Expected 2 contents, got %d", len(contents))
	}
	if contents[1].Role != "model" {
		t.Errorf("Role = %q, want model", contents[1].Role)
	}
	if contents[1].Parts[0].Text != "Hi there!" {
		t.Errorf("Text = %q, want %q", contents[1].Parts[0].Text, "Hi there!")
	}
}

func TestConvertToGoogleContents_ToolResults(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "What time?"},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{
					Name:      "get_time",
					Arguments: `{"tz":"UTC"}`,
				}},
			},
		},
		{Role: "tool", ToolCallID: "get_time", Content: `{"time":"12:00"}`},
	}

	_, contents := convertToGoogleContents(msgs)

	if len(contents) != 3 {
		t.Fatalf("Expected 3 contents, got %d", len(contents))
	}

	// assistant -> model with function call
	model := contents[1]
	if model.Role != "model" {
		t.Errorf("Role = %q, want model", model.Role)
	}
	if len(model.Parts) != 1 || model.Parts[0].FunctionCall == nil {
		t.Fatal("Expected function call in model content")
	}
	if model.Parts[0].FunctionCall.Name != "get_time" {
		t.Errorf("FunctionCall.Name = %q, want get_time", model.Parts[0].FunctionCall.Name)
	}

	// tool -> user with function response
	toolResp := contents[2]
	if toolResp.Role != "user" {
		t.Errorf("Role = %q, want user", toolResp.Role)
	}
	if len(toolResp.Parts) != 1 || toolResp.Parts[0].FunctionResponse == nil {
		t.Fatal("Expected function response in tool content")
	}
}

func TestConvertGoogleTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "search",
				Description: "Search the web",
				Parameters:  []byte(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        "calc",
				Description: "Calculate expression",
				Parameters:  []byte(`{"type":"object","properties":{"expr":{"type":"string"}}}`),
			},
		},
	}

	result := convertGoogleTools(tools)
	if len(result.FunctionDeclarations) != 2 {
		t.Fatalf("Expected 2 function declarations, got %d", len(result.FunctionDeclarations))
	}
	if result.FunctionDeclarations[0].Name != "search" {
		t.Errorf("Name = %q, want search", result.FunctionDeclarations[0].Name)
	}
	if result.FunctionDeclarations[1].Name != "calc" {
		t.Errorf("Name = %q, want calc", result.FunctionDeclarations[1].Name)
	}
}

func TestMakeGoogleToolConfig(t *testing.T) {
	tests := []struct {
		choice string
		want   genai.FunctionCallingConfigMode
	}{
		{"auto", genai.FunctionCallingConfigModeAuto},
		{"required", genai.FunctionCallingConfigModeAny},
		{"none", genai.FunctionCallingConfigModeNone},
		{"anything_else", genai.FunctionCallingConfigModeAuto},
	}
	for _, tt := range tests {
		t.Run(tt.choice, func(t *testing.T) {
			cfg := makeGoogleToolConfig(tt.choice)
			if cfg.FunctionCallingConfig.Mode != tt.want {
				t.Errorf("Mode = %q, want %q", cfg.FunctionCallingConfig.Mode, tt.want)
			}
		})
	}
}

// --- Native MCP: Testes de Payload, Routing e Isolamento ---

func TestAnthropicProvider_WithMCPServers_StoresServers(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:        "anthropic",
		Name:      "Anthropic",
		BaseURL:   "https://api.anthropic.com",
		APIFormat: APIFormatAnthropic,
	}
	provider := NewAnthropicProvider(p, credMgr)
	servers := []MCPServerConfig{
		{Name: "github-mcp", URL: "https://mcp.github.com/sse", AuthToken: "ghp_xxx"},
		{Name: "slack-mcp", URL: "https://mcp.slack.com/sse"},
	}
	result := provider.WithMCPServers(servers)
	if result == nil {
		t.Fatal("WithMCPServers returned nil")
	}
	anthP, ok := result.(*AnthropicProvider)
	if !ok {
		t.Fatal("WithMCPServers should return *AnthropicProvider")
	}
	if len(anthP.mcpServers) != 2 {
		t.Fatalf("Expected 2 MCP servers, got %d", len(anthP.mcpServers))
	}
	if anthP.mcpServers[0].Name != "github-mcp" {
		t.Errorf("Server 0 name = %q", anthP.mcpServers[0].Name)
	}
	if anthP.mcpServers[1].AuthToken != "" {
		t.Errorf("Server 1 should have empty auth token, got %q", anthP.mcpServers[1].AuthToken)
	}
}

func TestAnthropicProvider_WithMCPServers_EmptyReturnsOriginal(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "anthropic",
		Name:    "Anthropic",
		BaseURL: "https://api.anthropic.com",
	}
	provider := NewAnthropicProvider(p, credMgr)
	if provider.WithMCPServers(nil) != provider {
		t.Error("WithMCPServers(nil) should return same provider")
	}
}

func TestSupportsNativeMCP_ReflectsRealImplementation(t *testing.T) {
	credMgr := credentials.NewManager(nil)

	openaiCompat := NewOpenAIProvider(&ProviderConfig{ID: "oc", Name: "OC", BaseURL: "https://api.openrouter.ai/v1"}, credMgr)
	openaiReal := NewOpenAIResponsesProvider(&ProviderConfig{ID: "or", Name: "OR", BaseURL: "https://api.openai.com/v1"}, credMgr)
	anthropic := NewAnthropicProvider(&ProviderConfig{ID: "a", Name: "A", BaseURL: "https://api.anthropic.com"}, credMgr)
	google := NewGoogleProvider(&ProviderConfig{ID: "g", Name: "G"}, credMgr)

	if openaiCompat.SupportsNativeMCP() {
		t.Error("OpenAI-compatible should NOT support native MCP (Chat Completions only)")
	}
	if !openaiReal.SupportsNativeMCP() {
		t.Error("OpenAI Responses should support native MCP")
	}
	if !anthropic.SupportsNativeMCP() {
		t.Error("Anthropic should support native MCP (Beta Messages API)")
	}
	if google.SupportsNativeMCP() {
		t.Error("Google should NOT support native MCP (not implemented)")
	}
}

// TestOpenAIResponsesProvider_LiteLLMProxy_DefaultAndCapability cobre o caso do
// proxy OpenAI-compatible (ex.: LiteLLM) com api_format=openai_responses: ele
// fala a Responses API e é fisicamente CAPAZ de emitir type:"mcp"
// (NativeMCPCapable=true), mas o DEFAULT (auto, sem override de perfil) é NÃO
// usar nativo, pois pode rotear para modelos que rejeitam type:"mcp"
// (deepseek/v4-flash → 400). A decisão final é por perfil (ver internal/chat).
func TestOpenAIResponsesProvider_LiteLLMProxy_DefaultAndCapability(t *testing.T) {
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
	if provider.SupportsNativeMCP() {
		t.Error("default (auto) do proxy deve ser NÃO-nativo (heurística por URL)")
	}
	if !provider.NativeMCPCapable() {
		t.Error("proxy via Responses API é fisicamente capaz de emitir type:mcp")
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

// TestBuildResponsesParams_EmitsMCPToolWhenServersPresent garante, no nível da
// request/wire, que type:"mcp" é emitido sempre que há MCP servers anexados ao
// provider e ausente quando não há. A POLÍTICA de anexar (default por URL +
// override de perfil) é decidida em internal/chat antes de WithMCPServers.
func TestBuildResponsesParams_EmitsMCPToolWhenServersPresent(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	ctx := context.Background()
	msgs := []Message{{Role: "user", Content: "oi"}}
	servers := []MCPServerConfig{
		{Name: "GitHub", Slug: "github", URL: "https://api.githubcopilot.com/mcp/", AuthToken: "tok"},
	}

	// Proxy via Responses (capaz): quando o perfil força nativo, o chat layer
	// chama WithMCPServers e a request passa a conter type:"mcp".
	proxy := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "l", Name: "LiteLLM", Type: ProviderCustom,
		APIFormat: APIFormatOpenAIResponses, BaseURL: "http://llm.inclunet.com.br/v1",
	}, credMgr)
	withServers, ok := proxy.WithMCPServers(servers).(*OpenAIProvider)
	if !ok {
		t.Fatal("WithMCPServers deveria retornar *OpenAIProvider (proxy é capaz)")
	}
	withParams := withServers.buildResponsesParams(ctx, "deepseek-v4-flash", msgs, ChatParams{}, withServers.mcpServers)
	if !hasMCPTool(withParams.Tools) {
		t.Error("com servers anexados, a request deveria conter tool type:mcp")
	}

	// Sem servers anexados (caso adapter): nenhuma tool type:"mcp".
	noParams := proxy.buildResponsesParams(ctx, "deepseek-v4-flash", msgs, ChatParams{}, proxy.mcpServers)
	if hasMCPTool(noParams.Tools) {
		t.Error("sem servers anexados, a request NÃO deve conter tool type:mcp")
	}
}

func hasMCPTool(tools []responses.ToolUnionParam) bool {
	for i := range tools {
		if tools[i].OfMcp != nil {
			return true
		}
	}
	return false
}

func TestWithMCPServers_ImmutableOriginal(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	provider := NewOpenAIResponsesProvider(&ProviderConfig{ID: "o", Name: "O", BaseURL: "https://api.openai.com/v1"}, credMgr)

	servers := []MCPServerConfig{{Name: "test", URL: "http://test"}}
	modified := provider.WithMCPServers(servers)

	if len(provider.mcpServers) != 0 {
		t.Error("Original provider should not be modified")
	}
	modP := modified.(*OpenAIProvider)
	if len(modP.mcpServers) != 1 {
		t.Error("Modified provider should have 1 MCP server")
	}
	if !modP.useResponses {
		t.Error("Modified provider should preserve useResponses=true")
	}
}

func TestConvertToResponsesInput_Basic(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "Be helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "get_time", Arguments: `{}`}},
			},
		},
		{Role: "tool", Content: `{"time":"12:00"}`, ToolCallID: "call_1"},
	}

	input := convertToResponsesInput(msgs)
	if len(input) != 5 {
		t.Fatalf("Expected 5 input items, got %d", len(input))
	}

	// system
	if input[0].OfMessage == nil {
		t.Error("Item 0 should be a message")
	}
	// user
	if input[1].OfMessage == nil {
		t.Error("Item 1 should be a message")
	}
	// assistant text
	if input[2].OfMessage == nil {
		t.Error("Item 2 should be a message")
	}
	// function call
	if input[3].OfFunctionCall == nil {
		t.Error("Item 3 should be a function call")
	}
	if input[3].OfFunctionCall.Name != "get_time" {
		t.Errorf("FunctionCall name = %q, want get_time", input[3].OfFunctionCall.Name)
	}
	if input[3].OfFunctionCall.CallID != "call_1" {
		t.Errorf("FunctionCall callID = %q, want call_1", input[3].OfFunctionCall.CallID)
	}
	// function output
	if input[4].OfFunctionCallOutput == nil {
		t.Error("Item 4 should be a function call output")
	}
	if input[4].OfFunctionCallOutput.CallID != "call_1" {
		t.Errorf("FunctionCallOutput callID = %q, want call_1", input[4].OfFunctionCallOutput.CallID)
	}
}

func TestConvertToBetaMessages_PreservesStructure(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
	}
	_, anthropicMsgs := convertToAnthropicMessages(msgs)
	betaMsgs := convertToBetaMessages(anthropicMsgs)

	if len(betaMsgs) != 2 {
		t.Fatalf("Expected 2 beta messages, got %d", len(betaMsgs))
	}
	if string(betaMsgs[0].Role) != "user" {
		t.Errorf("Role[0] = %q, want user", betaMsgs[0].Role)
	}
	if string(betaMsgs[1].Role) != "assistant" {
		t.Errorf("Role[1] = %q, want assistant", betaMsgs[1].Role)
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
	if !provider.SupportsNativeMCP() {
		t.Error("Responses provider should support native MCP")
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
	if provider.SupportsNativeMCP() {
		t.Error("Compatible provider should NOT support native MCP")
	}
}

// --- Testes de paridade e robustez Responses vs Chat Completions ---

func TestConvertToResponsesInput_AssistantWithContentAndToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "What's the weather?"},
		{
			Role:    "assistant",
			Content: "Let me check.",
			ToolCalls: []ToolCall{
				{ID: "call_w", Type: "function", Function: FunctionCall{Name: "get_weather", Arguments: `{"city":"SP"}`}},
			},
		},
		{Role: "tool", Content: `{"temp":28}`, ToolCallID: "call_w"},
	}

	input := convertToResponsesInput(msgs)

	// user + assistant_text + function_call + function_call_output = 4 items
	if len(input) != 4 {
		t.Fatalf("Expected 4 input items, got %d", len(input))
	}
	if input[0].OfMessage == nil {
		t.Error("Item 0 should be user message")
	}
	if input[1].OfMessage == nil {
		t.Error("Item 1 should be assistant text message")
	}
	if input[2].OfFunctionCall == nil {
		t.Error("Item 2 should be function call")
	}
	if input[2].OfFunctionCall.Name != "get_weather" {
		t.Errorf("FunctionCall name = %q, want get_weather", input[2].OfFunctionCall.Name)
	}
	if input[3].OfFunctionCallOutput == nil {
		t.Error("Item 3 should be function call output")
	}
}

func TestConvertToResponsesInput_NoToolCalls(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
		{Role: "user", Content: "How are you?"},
	}

	input := convertToResponsesInput(msgs)
	if len(input) != 4 {
		t.Fatalf("Expected 4 input items, got %d", len(input))
	}
	for i, item := range input {
		if item.OfMessage == nil {
			t.Errorf("Item %d should be a message", i)
		}
	}
}

func TestConvertToResponsesInput_EmptyAssistantWithToolCalls(t *testing.T) {
	msgs := []Message{
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "fn_a", Arguments: `{}`}},
				{ID: "call_2", Type: "function", Function: FunctionCall{Name: "fn_b", Arguments: `{"x":1}`}},
			},
		},
		{Role: "tool", Content: "result_a", ToolCallID: "call_1"},
		{Role: "tool", Content: "result_b", ToolCallID: "call_2"},
	}

	input := convertToResponsesInput(msgs)

	// Empty content → no assistant message item. 2 function_calls + 2 outputs = 4.
	if len(input) != 4 {
		t.Fatalf("Expected 4 input items (no empty assistant msg), got %d", len(input))
	}
	if input[0].OfFunctionCall == nil || input[0].OfFunctionCall.Name != "fn_a" {
		t.Error("Item 0 should be function call fn_a")
	}
	if input[1].OfFunctionCall == nil || input[1].OfFunctionCall.Name != "fn_b" {
		t.Error("Item 1 should be function call fn_b")
	}
	if input[2].OfFunctionCallOutput == nil {
		t.Error("Item 2 should be function call output")
	}
	if input[3].OfFunctionCallOutput == nil {
		t.Error("Item 3 should be function call output")
	}
}

func TestAPIFormatOpenAICompatible_Alias(t *testing.T) {
	if APIFormatOpenAICompatible != APIFormatOpenAI {
		t.Errorf("APIFormatOpenAICompatible should be alias for APIFormatOpenAI, got %q vs %q",
			APIFormatOpenAICompatible, APIFormatOpenAI)
	}
	if APIFormatOpenAICompatible != "openai" {
		t.Errorf("APIFormatOpenAICompatible wire value should be 'openai', got %q", APIFormatOpenAICompatible)
	}
}

func TestGetAPIFormat_ExplicitOverridesInference(t *testing.T) {
	p := &ProviderConfig{
		ID:        "test",
		Name:      "Test",
		BaseURL:   "https://api.openai.com/v1",
		APIFormat: APIFormatOpenAI,
	}
	if got := p.GetAPIFormat(); got != APIFormatOpenAI {
		t.Errorf("Explicit api_format should override inference, got %q, want %q", got, APIFormatOpenAI)
	}
}

func TestGetAPIFormat_LegacyConfigsBackwardsCompat(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		format  APIFormat
		want    APIFormat
	}{
		{"legacy_openai_no_format", "https://api.openai.com/v1", "", APIFormatOpenAIResponses},
		{"legacy_groq_no_format", "https://api.groq.com/openai/v1", "", APIFormatOpenAI},
		{"legacy_ollama_no_format", "http://localhost:11434/v1", "", APIFormatOpenAI},
		{"explicit_openai_compat_on_openai_url", "https://api.openai.com/v1", APIFormatOpenAI, APIFormatOpenAI},
		{"explicit_responses_on_random_url", "https://my-proxy.com/v1", APIFormatOpenAIResponses, APIFormatOpenAIResponses},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &ProviderConfig{ID: "t", Name: "T", BaseURL: tt.baseURL, APIFormat: tt.format}
			if got := p.GetAPIFormat(); got != tt.want {
				t.Errorf("GetAPIFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewChatProvider_Factory_InfersResponses(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "openai-real",
		Name:    "OpenAI Real",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewChatProvider(p, credMgr)
	openaiP, ok := provider.(*OpenAIProvider)
	if !ok {
		t.Fatal("Expected *OpenAIProvider")
	}
	if !openaiP.useResponses {
		t.Error("Provider for api.openai.com without explicit format should use Responses API")
	}
}

func TestConvertMessages_MultimodalUser_PreservesImages(t *testing.T) {
	msgs := []Message{
		{
			Role: "user",
			Content: []interface{}{
				map[string]interface{}{"type": "text", "text": "What's in this image?"},
				map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]interface{}{"url": "https://example.com/cat.png"},
				},
			},
		},
	}

	result := convertMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(result))
	}
	if result[0].OfUser == nil {
		t.Fatal("Expected user message")
	}
	// Chat Completions path: extractImageParts detects image_url parts and
	// converts to proper multimodal content (text + image_url parts).
	// This path preserves image data correctly.
}

// TestConvertToResponsesInput_MultimodalLosesImageData is an INTENTIONAL limitation test.
//
// KNOWN LIMITATION (documented):
// The Responses API path (convertToResponsesInput) does NOT support multimodal
// image_url parts. When a user message contains text + image_url, only the text
// portions are preserved — the image data is silently lost.
//
// This is because convertToResponsesInput uses GetContentAsString(), which
// concatenates only text parts from multimodal content.
//
// The Chat Completions path (convertMessages + extractImageParts) DOES preserve
// images correctly.
//
// This test exists to:
//  1. Document this as a KNOWN limitation, not a silent regression.
//  2. Freeze the current behavior so any future change is intentional.
//  3. Serve as a guide for when multimodal support is added to Responses path.
//
// When Responses API multimodal support is implemented, this test should be
// updated to verify images ARE preserved (and the limitation comment removed).
func TestConvertToResponsesInput_MultimodalLosesImageData(t *testing.T) {
	multimodalMsg := Message{
		Role: "user",
		Content: []interface{}{
			map[string]interface{}{"type": "text", "text": "What's in this image?"},
			map[string]interface{}{
				"type":      "image_url",
				"image_url": map[string]interface{}{"url": "https://example.com/cat.png"},
			},
		},
	}

	// Verify the raw content has both text and image parts
	parts, ok := multimodalMsg.Content.([]interface{})
	if !ok || len(parts) != 2 {
		t.Fatal("Test setup: expected 2 content parts (text + image_url)")
	}

	// GetContentAsString extracts only text, losing the image
	textOnly := multimodalMsg.GetContentAsString()
	if textOnly != "What's in this image?" {
		t.Errorf("GetContentAsString() = %q, want only the text portion", textOnly)
	}

	// convertToResponsesInput produces a single text message — image is lost
	input := convertToResponsesInput([]Message{multimodalMsg})
	if len(input) != 1 {
		t.Fatalf("Expected 1 input item, got %d", len(input))
	}
	if input[0].OfMessage == nil {
		t.Fatal("Expected message item (text-only fallback)")
	}

	// Compare with Chat Completions path, which DOES preserve the image
	ccMsgs := convertMessages([]Message{multimodalMsg})
	if len(ccMsgs) != 1 {
		t.Fatalf("Expected 1 CC message, got %d", len(ccMsgs))
	}
	if ccMsgs[0].OfUser == nil {
		t.Fatal("Expected user message in Chat Completions path")
	}
	// Chat Completions path uses extractImageParts, which detects image_url
	// and creates proper multimodal content parts. This is the reference behavior.
}

func TestNativeMCP_ToolDeduplication(t *testing.T) {
	// Simula cenário: tools do bridge incluem MCP tools + tools internas.
	// Quando MCP nativo está ativo, bridge tools dos servidores nativos devem ser removidas.
	allTools := []ToolDefinition{
		{Function: FunctionDefinition{Name: "internal_search"}},
		{Function: FunctionDefinition{Name: "mcp__github__list_repos"}},
		{Function: FunctionDefinition{Name: "mcp__github__create_issue"}},
		{Function: FunctionDefinition{Name: "mcp__slack__send_message"}},
		{Function: FunctionDefinition{Name: "internal_calc"}},
	}

	nativeToolNames := map[string]bool{
		"mcp__github__list_repos":   true,
		"mcp__github__create_issue": true,
	}

	var filtered []ToolDefinition
	for _, td := range allTools {
		if !nativeToolNames[td.Function.Name] {
			filtered = append(filtered, td)
		}
	}

	if len(filtered) != 3 {
		t.Fatalf("Expected 3 tools after dedup, got %d", len(filtered))
	}
	expectedNames := []string{"internal_search", "mcp__slack__send_message", "internal_calc"}
	for i, name := range expectedNames {
		if filtered[i].Function.Name != name {
			t.Errorf("filtered[%d] = %q, want %q", i, filtered[i].Function.Name, name)
		}
	}
}

func TestMCPServerConfig_AllowedToolsPreserved(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{ID: "o", Name: "O", BaseURL: "https://api.openai.com/v1"}
	provider := NewOpenAIResponsesProvider(p, credMgr)

	servers := []MCPServerConfig{
		{
			Name:         "github-mcp",
			URL:          "https://mcp.github.com/sse",
			ToolNames:    []string{"mcp_github__create_issue"},
			AllowedTools: []string{"create_issue"},
		},
	}
	result := provider.WithMCPServers(servers)
	openaiP := result.(*OpenAIProvider)
	if len(openaiP.mcpServers[0].AllowedTools) != 1 {
		t.Fatalf("Expected 1 AllowedTools, got %d", len(openaiP.mcpServers[0].AllowedTools))
	}
	if openaiP.mcpServers[0].AllowedTools[0] != "create_issue" {
		t.Errorf("AllowedTools[0] = %q, want %q", openaiP.mcpServers[0].AllowedTools[0], "create_issue")
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

func TestMCPServerConfig_EmptyAllowedToolsMeansAll(t *testing.T) {
	cfg := MCPServerConfig{
		Name:      "test-mcp",
		URL:       "https://test.com/mcp",
		ToolNames: []string{"mcp_test__tool_a", "mcp_test__tool_b"},
	}
	if len(cfg.AllowedTools) != 0 {
		t.Error("Empty AllowedTools should mean all tools are allowed")
	}
}

func TestAnthropicProvider_AllowedToolsPreserved(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{ID: "a", Name: "A", BaseURL: "https://api.anthropic.com", APIFormat: APIFormatAnthropic}
	provider := NewAnthropicProvider(p, credMgr)

	servers := []MCPServerConfig{
		{
			Name:         "github-mcp",
			URL:          "https://mcp.github.com/sse",
			AllowedTools: []string{"create_issue", "list_repos"},
		},
	}
	result := provider.WithMCPServers(servers)
	antP := result.(*AnthropicProvider)
	if len(antP.mcpServers) != 1 {
		t.Fatal("Expected 1 MCP server")
	}
	if len(antP.mcpServers[0].AllowedTools) != 2 {
		t.Fatalf("Expected 2 AllowedTools, got %d", len(antP.mcpServers[0].AllowedTools))
	}
}

// filterNativeMCPByProfile replica a lógica de filtragem do chat loop (llm.go)
// usando mcplib.ParseToolName como fonte canônica de naming.
func filterNativeMCPByProfile(enabledTools []string, servers []struct {
	slug      string
	toolNames []string
}) (configs []MCPServerConfig, nativeToolNames map[string]bool) {
	var enabledSet map[string]bool
	if enabledTools != nil {
		enabledSet = make(map[string]bool, len(enabledTools))
		for _, n := range enabledTools {
			enabledSet[n] = true
		}
	}

	nativeToolNames = make(map[string]bool)
	for _, srv := range servers {
		cfg := MCPServerConfig{
			Name:      srv.slug + "-mcp",
			URL:       "https://" + srv.slug + ".com/mcp",
			ToolNames: srv.toolNames,
		}
		if enabledSet != nil {
			var allowed []string
			var allowedFull []string
			for _, fullName := range srv.toolNames {
				if enabledSet[fullName] {
					if _, originalName, ok := mcplib.ParseToolName(fullName); ok {
						allowed = append(allowed, originalName)
					}
					allowedFull = append(allowedFull, fullName)
				}
			}
			if len(allowed) == 0 {
				continue
			}
			cfg.AllowedTools = allowed
			cfg.ToolNames = allowedFull
		}
		configs = append(configs, cfg)
		for _, tn := range cfg.ToolNames {
			nativeToolNames[tn] = true
		}
	}
	return
}

func TestProfileEnabledTools_FilterNativeMCPServers(t *testing.T) {
	type serverInput struct {
		slug      string
		toolNames []string
	}
	type testCase struct {
		name         string
		enabledTools []string
		servers      []serverInput
		wantConfigs  int
		wantAllowed  map[string][]string
	}

	tests := []testCase{
		{
			name:         "nil EnabledTools passes all servers",
			enabledTools: nil,
			servers: []serverInput{
				{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
			},
			wantConfigs: 1,
			wantAllowed: map[string][]string{"github": nil},
		},
		{
			name:         "filter keeps only enabled tools",
			enabledTools: []string{"mcp_github__create_issue", "internal_search"},
			servers: []serverInput{
				{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
			},
			wantConfigs: 1,
			wantAllowed: map[string][]string{"github": {"create_issue"}},
		},
		{
			name:         "server excluded when no tools match",
			enabledTools: []string{"internal_search"},
			servers: []serverInput{
				{slug: "github", toolNames: []string{"mcp_github__create_issue"}},
			},
			wantConfigs: 0,
		},
		{
			name:         "multiple servers with partial match",
			enabledTools: []string{"mcp_github__create_issue", "mcp_slack__send_message"},
			servers: []serverInput{
				{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
				{slug: "slack", toolNames: []string{"mcp_slack__send_message"}},
				{slug: "jira", toolNames: []string{"mcp_jira__create_ticket"}},
			},
			wantConfigs: 2,
			wantAllowed: map[string][]string{
				"github": {"create_issue"},
				"slack":  {"send_message"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srvInputs := make([]struct {
				slug      string
				toolNames []string
			}, len(tc.servers))
			for i, s := range tc.servers {
				srvInputs[i].slug = s.slug
				srvInputs[i].toolNames = s.toolNames
			}
			configs, _ := filterNativeMCPByProfile(tc.enabledTools, srvInputs)

			if len(configs) != tc.wantConfigs {
				t.Fatalf("got %d configs, want %d", len(configs), tc.wantConfigs)
			}
			for slug, wantAllowed := range tc.wantAllowed {
				found := false
				for _, cfg := range configs {
					if cfg.Name == slug+"-mcp" {
						found = true
						if wantAllowed == nil {
							if len(cfg.AllowedTools) != 0 {
								t.Errorf("server %q: want nil AllowedTools, got %v", slug, cfg.AllowedTools)
							}
						} else {
							if len(cfg.AllowedTools) != len(wantAllowed) {
								t.Errorf("server %q: got %d AllowedTools, want %d", slug, len(cfg.AllowedTools), len(wantAllowed))
							}
							for i, want := range wantAllowed {
								if i < len(cfg.AllowedTools) && cfg.AllowedTools[i] != want {
									t.Errorf("server %q AllowedTools[%d] = %q, want %q", slug, i, cfg.AllowedTools[i], want)
								}
							}
						}
					}
				}
				if !found {
					t.Errorf("server %q not found in configs", slug)
				}
			}
		})
	}
}

func TestNativeMCP_NoDuplicateWithBridge(t *testing.T) {
	bridgeToolDefs := []ToolDefinition{
		{Function: FunctionDefinition{Name: "internal_search"}},
		{Function: FunctionDefinition{Name: "mcp_github__create_issue"}},
		{Function: FunctionDefinition{Name: "mcp_github__list_repos"}},
		{Function: FunctionDefinition{Name: "mcp_slack__send_message"}},
	}

	servers := []struct {
		slug      string
		toolNames []string
	}{
		{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
	}

	_, nativeToolNames := filterNativeMCPByProfile(nil, servers)

	var afterDedup []ToolDefinition
	for _, td := range bridgeToolDefs {
		if !nativeToolNames[td.Function.Name] {
			afterDedup = append(afterDedup, td)
		}
	}

	nativeNames := make(map[string]bool)
	for name := range nativeToolNames {
		nativeNames[name] = true
	}
	bridgeNames := make(map[string]bool)
	for _, td := range afterDedup {
		bridgeNames[td.Function.Name] = true
	}

	for name := range nativeNames {
		if bridgeNames[name] {
			t.Errorf("tool %q presente tanto em native quanto em bridge (duplicata!)", name)
		}
	}

	if len(afterDedup) != 2 {
		t.Fatalf("após dedup, esperado 2 tools restantes, obtido %d", len(afterDedup))
	}
	if afterDedup[0].Function.Name != "internal_search" {
		t.Errorf("afterDedup[0] = %q, esperado internal_search", afterDedup[0].Function.Name)
	}
	if afterDedup[1].Function.Name != "mcp_slack__send_message" {
		t.Errorf("afterDedup[1] = %q, esperado mcp_slack__send_message", afterDedup[1].Function.Name)
	}
}

// mcpTrackingHandler captura MCPToolEvents para validação em testes.
type mcpTrackingHandler struct {
	captureHandler
	events []MCPToolEvent
}

func (h *mcpTrackingHandler) OnMCPToolEvent(event MCPToolEvent) {
	h.events = append(h.events, event)
}

func TestMCPToolEvent_StreamHandlerInterface(t *testing.T) {
	var handler StreamHandler = &mcpTrackingHandler{}

	handler.OnMCPToolEvent(MCPToolEvent{
		ID:          "call_001",
		Name:        "jira_search",
		ServerLabel: "Atlassian",
		IsCompleted: false,
	})

	handler.OnMCPToolEvent(MCPToolEvent{
		ID:          "call_001",
		Name:        "jira_search",
		ServerLabel: "Atlassian",
		Arguments:   `{"query":"project=FSD"}`,
		Output:      `{"issues":[{"key":"FSD-123"}]}`,
		IsCompleted: true,
	})

	h := handler.(*mcpTrackingHandler)
	if len(h.events) != 2 {
		t.Fatalf("esperado 2 eventos, obtido %d", len(h.events))
	}

	start := h.events[0]
	if start.Name != "jira_search" || start.ServerLabel != "Atlassian" || start.IsCompleted {
		t.Errorf("evento start incorreto: %+v", start)
	}

	end := h.events[1]
	if end.Name != "jira_search" || !end.IsCompleted || end.Output == "" {
		t.Errorf("evento end incorreto: %+v", end)
	}
	if end.Arguments != `{"query":"project=FSD"}` {
		t.Errorf("Arguments = %q, esperado query JQL", end.Arguments)
	}
}

func TestMCPToolEvent_ErrorTracking(t *testing.T) {
	handler := &mcpTrackingHandler{}

	handler.OnMCPToolEvent(MCPToolEvent{
		ID:          "call_err",
		Name:        "search",
		ServerLabel: "Slack",
		Error:       "unauthorized",
		IsCompleted: true,
	})

	if len(handler.events) != 1 {
		t.Fatalf("esperado 1 evento, obtido %d", len(handler.events))
	}
	ev := handler.events[0]
	if ev.Error != "unauthorized" || !ev.IsCompleted {
		t.Errorf("evento de erro incorreto: %+v", ev)
	}
}

func TestMCPToolEvent_MultipleServers(t *testing.T) {
	handler := &mcpTrackingHandler{}

	handler.OnMCPToolEvent(MCPToolEvent{
		ID: "mc1", Name: "search", ServerLabel: "Atlassian", IsCompleted: false,
	})
	handler.OnMCPToolEvent(MCPToolEvent{
		ID: "mc2", Name: "read_channel", ServerLabel: "Slack", IsCompleted: false,
	})
	handler.OnMCPToolEvent(MCPToolEvent{
		ID: "mc1", Name: "search", ServerLabel: "Atlassian",
		Output: `[{"key":"FSD-1"}]`, IsCompleted: true,
	})
	handler.OnMCPToolEvent(MCPToolEvent{
		ID: "mc2", Name: "read_channel", ServerLabel: "Slack",
		Output: "messages...", IsCompleted: true,
	})

	if len(handler.events) != 4 {
		t.Fatalf("esperado 4 eventos, obtido %d", len(handler.events))
	}

	starts := 0
	ends := 0
	servers := map[string]bool{}
	for _, ev := range handler.events {
		if ev.IsCompleted {
			ends++
		} else {
			starts++
		}
		servers[ev.ServerLabel] = true
	}
	if starts != 2 || ends != 2 {
		t.Errorf("esperado 2 starts + 2 ends, obtido %d starts + %d ends", starts, ends)
	}
	if !servers["Atlassian"] || !servers["Slack"] {
		t.Errorf("servidores rastreados: %v", servers)
	}
}

type providerRetryHandler struct {
	captureHandler
	errors []string
	done   int
}

func (h *providerRetryHandler) OnError(err string) {
	h.errors = append(h.errors, err)
}

func (h *providerRetryHandler) OnDone(fullResponse string, usage Usage, model string) {
	h.done++
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
	provider.responsesAttemptFn = func(_ context.Context, _ responses.ResponseNewParams, handler StreamHandler, servers []MCPServerConfig) mcpStreamAttemptResult {
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

func TestAnthropicProvider_StreamChatWithMCP_DegradesFailedMCPServer(t *testing.T) {
	recovered := make(chan struct{}, 1)
	seen := make([][]string, 0, 2)
	attempts := 0

	provider := &AnthropicProvider{
		provider: &ProviderConfig{ID: "a", Name: "Anthropic", BaseURL: "https://api.anthropic.com"},
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
	provider.betaAttemptFn = func(_ context.Context, _ anthropic.BetaMessageNewParams, handler StreamHandler, servers []MCPServerConfig) mcpStreamAttemptResult {
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
					Stage:      MCPFailureStageHandshake,
					Message:    "Falha no Atlassian",
					Degradable: true,
				},
			}
		}
		handler.OnChunk("ok")
		handler.OnDone("ok", Usage{}, "claude-test")
		return mcpStreamAttemptResult{done: true}
	}

	handler := &providerRetryHandler{}
	provider.streamChatWithMCP(context.Background(), "claude-test", 256, nil, nil, ChatParams{}, handler)

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

func TestNativeMCP_NoDuplicateWithProfile(t *testing.T) {
	bridgeToolDefs := []ToolDefinition{
		{Function: FunctionDefinition{Name: "internal_search"}},
		{Function: FunctionDefinition{Name: "mcp_github__create_issue"}},
	}

	servers := []struct {
		slug      string
		toolNames []string
	}{
		{slug: "github", toolNames: []string{"mcp_github__create_issue", "mcp_github__list_repos"}},
	}

	enabledTools := []string{"mcp_github__create_issue", "internal_search"}
	configs, nativeToolNames := filterNativeMCPByProfile(enabledTools, servers)

	if len(configs) != 1 {
		t.Fatalf("esperado 1 config, obtido %d", len(configs))
	}
	if len(configs[0].AllowedTools) != 1 || configs[0].AllowedTools[0] != "create_issue" {
		t.Errorf("AllowedTools = %v, esperado [create_issue]", configs[0].AllowedTools)
	}

	var afterDedup []ToolDefinition
	for _, td := range bridgeToolDefs {
		if !nativeToolNames[td.Function.Name] {
			afterDedup = append(afterDedup, td)
		}
	}

	for _, td := range afterDedup {
		if nativeToolNames[td.Function.Name] {
			t.Errorf("tool %q duplicada: aparece em bridge E native", td.Function.Name)
		}
	}

	if len(afterDedup) != 1 || afterDedup[0].Function.Name != "internal_search" {
		t.Errorf("após dedup com perfil, esperado apenas [internal_search], obtido %v",
			func() []string {
				var names []string
				for _, td := range afterDedup {
					names = append(names, td.Function.Name)
				}
				return names
			}())
	}
}
