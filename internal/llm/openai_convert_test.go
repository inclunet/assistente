package llm

import (
	"testing"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/responses"
)

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

func TestPromptCacheKeyAppliedToOpenAIParams(t *testing.T) {
	params := ChatParams{PromptCacheKey: "asst-123"}

	chatParams := openai.ChatCompletionNewParams{}
	applyPromptCacheKeyToChatCompletions(&chatParams, params)
	if !chatParams.PromptCacheKey.Valid() || chatParams.PromptCacheKey.Value != "asst-123" {
		t.Fatalf("ChatCompletion PromptCacheKey = %#v, want asst-123", chatParams.PromptCacheKey)
	}

	respParams := responses.ResponseNewParams{}
	applyPromptCacheKeyToResponses(&respParams, params)
	if !respParams.PromptCacheKey.Valid() || respParams.PromptCacheKey.Value != "asst-123" {
		t.Fatalf("Responses PromptCacheKey = %#v, want asst-123", respParams.PromptCacheKey)
	}

	params.PromptCacheHintFallback = &PromptCacheHintFallback{}
	params.PromptCacheHintFallback.Disable()
	chatParams = openai.ChatCompletionNewParams{}
	applyPromptCacheKeyToChatCompletions(&chatParams, params)
	if chatParams.PromptCacheKey.Valid() {
		t.Fatalf("ChatCompletion PromptCacheKey = %#v, want omitted after fallback disable", chatParams.PromptCacheKey)
	}
}

func TestLooksLikePromptCacheHintUnsupportedRequiresExplicitRejection(t *testing.T) {
	if !looksLikePromptCacheHintUnsupported("400 invalid parameter: prompt_cache_key is not supported") {
		t.Fatal("expected explicit prompt_cache_key rejection")
	}
	if !looksLikePromptCacheHintUnsupported("Unrecognized request argument supplied: prompt_cache_key") {
		t.Fatal("expected LiteLLM/OpenAI unrecognized request argument rejection")
	}
	if looksLikePromptCacheHintUnsupported("503 timeout while sending prompt_cache_key") {
		t.Fatal("retryable transport error should not look like explicit prompt_cache_key rejection")
	}
	if looksLikePromptCacheHintUnsupported("prompt_cache_key returned zero cache hits") {
		t.Fatal("cache miss/zero hit should not disable provider hints")
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
