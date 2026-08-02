package llm

import (
	"context"
	"testing"
	"time"

	"assistente/internal/credentials"
	"github.com/anthropics/anthropic-sdk-go"
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
			provider := NewChatProvider(p, credMgr, nil)
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

func TestAnthropicProvider_NativeMCPCapable(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.anthropic.com",
	}
	provider := NewAnthropicProvider(p, credMgr)
	if !provider.NativeMCPCapable() {
		t.Error("AnthropicProvider.NativeMCPCapable() = false, want true")
	}
}

func TestConvertToAnthropicMessages_SystemExtraction(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
	}

	system, result := convertToAnthropicMessages(msgs, false)

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

func TestConvertToAnthropicMessages_ExplicitCacheControlSplitsStableSystemPrefix(t *testing.T) {
	const stable = "stable instructions"
	const dynamic = "\n\n<conversation_summary>\ndynamic summary\n</conversation_summary>"
	msgs := []Message{
		{
			Role:                        "system",
			Content:                     stable + dynamic,
			SystemCacheControlPrefixLen: len(stable),
		},
		{Role: "user", Content: "Hello"},
	}

	system, result := convertToAnthropicMessages(msgs, true)

	if len(result) != 1 {
		t.Fatalf("Expected 1 non-system message, got %d", len(result))
	}
	if len(system) != 2 {
		t.Fatalf("Expected 2 system blocks, got %d", len(system))
	}
	if system[0].Text != stable {
		t.Fatalf("stable system block = %q, want %q", system[0].Text, stable)
	}
	if system[0].CacheControl.Type == "" {
		t.Fatal("expected cache_control on stable system block")
	}
	if system[1].Text != dynamic {
		t.Fatalf("dynamic system block = %q, want %q", system[1].Text, dynamic)
	}
	if system[1].CacheControl.Type != "" {
		t.Fatal("dynamic system block should not have cache_control")
	}
}

func TestConvertToAnthropicMessages_ExplicitCacheControlDisabledOmitsMarker(t *testing.T) {
	msgs := []Message{
		{
			Role:                        "system",
			Content:                     "stable\n\ndynamic",
			SystemCacheControlPrefixLen: len("stable"),
		},
		{Role: "user", Content: "Hello"},
	}

	system, _ := convertToAnthropicMessages(msgs, false)

	if len(system) != 1 {
		t.Fatalf("Expected 1 system block, got %d", len(system))
	}
	if system[0].CacheControl.Type != "" {
		t.Fatal("cache_control should be omitted when explicit cache control is disabled")
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

	system, result := convertToAnthropicMessages(msgs, false)

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

	_, result := convertToAnthropicMessages(msgs, false)

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

	result := convertAnthropicTools(tools, false)
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

func TestConvertAnthropicTools_ExplicitCacheControlMarksLastTool(t *testing.T) {
	tools := []ToolDefinition{
		{Function: FunctionDefinition{Name: "first", Description: "First", Parameters: []byte(`{"type":"object"}`)}},
		{Function: FunctionDefinition{Name: "last", Description: "Last", Parameters: []byte(`{"type":"object"}`)}},
	}

	result := convertAnthropicTools(tools, true)

	if len(result) != 2 {
		t.Fatalf("Expected 2 tools, got %d", len(result))
	}
	if result[0].OfTool.CacheControl.Type != "" {
		t.Fatal("first tool should not have cache_control")
	}
	if result[1].OfTool.CacheControl.Type == "" {
		t.Fatal("last tool should have cache_control")
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

func TestGoogleProvider_NativeMCPCapable_False(t *testing.T) {
	p := NewGoogleProvider(&ProviderConfig{
		ID:   "gp",
		Name: "Google",
	}, nil)
	if p.NativeMCPCapable() {
		t.Error("GoogleProvider should NOT be native MCP capable (not implemented)")
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

func TestNativeMCPCapable_ReflectsRealImplementation(t *testing.T) {
	credMgr := credentials.NewManager(nil)

	openaiCompat := NewOpenAIProvider(&ProviderConfig{ID: "oc", Name: "OC", BaseURL: "https://api.openrouter.ai/v1"}, credMgr)
	openaiReal := NewOpenAIResponsesProvider(&ProviderConfig{ID: "or", Name: "OR", BaseURL: "https://api.openai.com/v1"}, credMgr)
	anthropic := NewAnthropicProvider(&ProviderConfig{ID: "a", Name: "A", BaseURL: "https://api.anthropic.com"}, credMgr)
	google := NewGoogleProvider(&ProviderConfig{ID: "g", Name: "G"}, credMgr)

	if openaiCompat.NativeMCPCapable() {
		t.Error("OpenAI-compatible (Chat Completions) should NOT be native MCP capable")
	}
	if !openaiReal.NativeMCPCapable() {
		t.Error("OpenAI Responses should be native MCP capable")
	}
	if !anthropic.NativeMCPCapable() {
		t.Error("Anthropic should be native MCP capable (Beta Messages API)")
	}
	if google.NativeMCPCapable() {
		t.Error("Google should NOT be native MCP capable (not implemented)")
	}
}

func TestConvertToBetaMessages_PreservesStructure(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi!"},
	}
	_, anthropicMsgs := convertToAnthropicMessages(msgs, false)
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

func TestAnthropicProvider_BetaMCPParams_ExplicitCacheControlMarksLastTool(t *testing.T) {
	provider := &AnthropicProvider{}
	tools := []ToolDefinition{
		{Function: FunctionDefinition{Name: "local_tool", Description: "Local tool", Parameters: []byte(`{"type":"object"}`)}},
	}
	params := provider.buildBetaMCPParams(
		context.Background(),
		"claude-sonnet",
		1024,
		nil,
		nil,
		ChatParams{ExplicitCacheControl: true},
		[]MCPServerConfig{{Name: "remote", URL: "https://mcp.example.com"}},
		tools...,
	)

	if len(params.Tools) != 2 {
		t.Fatalf("Tools len = %d, want 2", len(params.Tools))
	}
	if params.Tools[0].GetCacheControl().Type != "" {
		t.Fatal("MCP toolset should not be marked when a later local tool exists")
	}
	if params.Tools[1].GetCacheControl().Type == "" {
		t.Fatal("last beta tool should have cache_control")
	}
}

func TestAnthropicProvider_BetaMCPParams_ExplicitCacheControlMarksMCPToolsetWhenLast(t *testing.T) {
	provider := &AnthropicProvider{}
	params := provider.buildBetaMCPParams(
		context.Background(),
		"claude-sonnet",
		1024,
		nil,
		nil,
		ChatParams{ExplicitCacheControl: true},
		[]MCPServerConfig{{Name: "remote", URL: "https://mcp.example.com"}},
	)

	if len(params.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(params.Tools))
	}
	if params.Tools[0].GetCacheControl().Type == "" {
		t.Fatal("MCP toolset should have cache_control when it is the last stable tool")
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
	provider := NewChatProvider(p, credMgr, nil)
	openaiP, ok := provider.(*OpenAIProvider)
	if !ok {
		t.Fatal("Expected *OpenAIProvider")
	}
	if !openaiP.useResponses {
		t.Error("Provider for api.openai.com without explicit format should use Responses API")
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
