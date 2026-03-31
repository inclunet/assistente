package llm

import (
	"testing"

	"assistente/internal/credentials"
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

func TestGetAPIFormat_Explicit(t *testing.T) {
	tests := []struct {
		format APIFormat
		want   APIFormat
	}{
		{APIFormatOpenAI, APIFormatOpenAI},
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
		name        string
		format      APIFormat
		wantOpenAI  bool
		wantAnthropic bool
		wantGoogle  bool
	}{
		{"default_empty", "", true, false, false},
		{"openai", APIFormatOpenAI, true, false, false},
		{"anthropic", APIFormatAnthropic, false, true, false},
		{"google", APIFormatGoogle, false, false, true},
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
			_, isOpenAI := provider.(*OpenAIProvider)
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
		})
	}
}

func TestOpenAIProvider_SupportsNativeMCP(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIProvider(p, credMgr)
	if !provider.SupportsNativeMCP() {
		t.Error("OpenAIProvider.SupportsNativeMCP() = false, want true")
	}
}

func TestOpenAIProvider_WithMCPServers(t *testing.T) {
	credMgr := credentials.NewManager(nil)
	p := &ProviderConfig{
		ID:      "test",
		Name:    "Test",
		BaseURL: "https://api.openai.com/v1",
	}
	provider := NewOpenAIProvider(p, credMgr)
	result := provider.WithMCPServers([]MCPServerConfig{
		{Label: "test", URL: "https://mcp.test.com"},
	})
	if result == nil {
		t.Fatal("WithMCPServers returned nil")
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
		{0, 180},  // default 3min
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
	tr := newCredentialTransport(nil, "test.com")
	if tr == nil {
		t.Fatal("newCredentialTransport returned nil")
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
	if APIFormatAnthropic != "anthropic" {
		t.Errorf("APIFormatAnthropic = %q", APIFormatAnthropic)
	}
	if APIFormatGoogle != "google" {
		t.Errorf("APIFormatGoogle = %q", APIFormatGoogle)
	}
}

// --- GoogleProvider tests ---

func TestGoogleProvider_SupportsNativeMCP(t *testing.T) {
	p := NewGoogleProvider(&ProviderConfig{
		ID:   "gp",
		Name: "Google",
	}, nil)
	if !p.SupportsNativeMCP() {
		t.Error("GoogleProvider should support native MCP")
	}
}

func TestGoogleProvider_WithMCPServers(t *testing.T) {
	p := NewGoogleProvider(&ProviderConfig{ID: "gp"}, nil)
	p2 := p.WithMCPServers([]MCPServerConfig{{Label: "test", URL: "http://test"}})
	if p2 == nil {
		t.Fatal("WithMCPServers returned nil")
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
