package llm

import (
	"testing"

	"assistente/internal/credentials"
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
		name      string
		format    APIFormat
		wantType  string
	}{
		{"default_empty", "", "*llm.OpenAIProvider"},
		{"openai", APIFormatOpenAI, "*llm.OpenAIProvider"},
		{"anthropic_fallback", APIFormatAnthropic, "*llm.OpenAIProvider"}, // TODO: mudará na Fase 2
		{"google_fallback", APIFormatGoogle, "*llm.OpenAIProvider"},       // TODO: mudará na Fase 3
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
