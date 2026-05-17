package summarization

import (
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/profiles"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"empty string", "", 0},
		{"single char", "a", 1},
		{"exactly 4 chars", "abcd", 1},
		{"5 chars rounds up", "abcde", 2},
		{"8 chars = 2 tokens", "abcdefgh", 2},
		{"12 chars = 3 tokens", "123456789012", 3},
		{"typical sentence", "Hello, this is a test message for token estimation.", 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EstimateTokens(tt.text)
			if result != tt.expected {
				t.Errorf("EstimateTokens(%q) = %d, expected %d", tt.text, result, tt.expected)
			}
		})
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		result := EstimateMessagesTokens(nil)
		if result != 0 {
			t.Errorf("expected 0, got %d", result)
		}
	})

	t.Run("messages with content only", func(t *testing.T) {
		msgs := []database.ChatMessage{
			{Content: "abcd"},
			{Content: "abcdefgh"},
		}
		result := EstimateMessagesTokens(msgs)
		if result != 3 {
			t.Errorf("expected 3, got %d", result)
		}
	})

	t.Run("messages with content and tool_calls", func(t *testing.T) {
		msgs := []database.ChatMessage{
			{Content: "abcd", ToolCalls: "abcdefgh"},
		}
		result := EstimateMessagesTokens(msgs)
		if result != 3 {
			t.Errorf("expected 3, got %d", result)
		}
	})

	t.Run("message with empty tool_calls ignored", func(t *testing.T) {
		msgs := []database.ChatMessage{
			{Content: "abcd", ToolCalls: ""},
		}
		result := EstimateMessagesTokens(msgs)
		if result != 1 {
			t.Errorf("expected 1, got %d", result)
		}
	})
}

func TestShouldTriggerSummarization(t *testing.T) {
	makeProfile := func(contextWindow, maxTokens int) *profiles.Profile {
		return &profiles.Profile{
			Chat: profiles.ChatConfig{
				ContextWindow: contextWindow,
				MaxTokens:     maxTokens,
			},
		}
	}

	makeMsgs := func(contentLen int, count int) []database.ChatMessage {
		content := strings.Repeat("x", contentLen)
		msgs := make([]database.ChatMessage, count)
		for i := range msgs {
			msgs[i] = database.ChatMessage{Content: content}
		}
		return msgs
	}

	t.Run("nil profile returns false", func(t *testing.T) {
		if ShouldTriggerSummarization(nil, nil, "") {
			t.Error("expected false for nil profile")
		}
	})

	t.Run("zero context window returns false", func(t *testing.T) {
		p := makeProfile(0, 4096)
		if ShouldTriggerSummarization(p, nil, "") {
			t.Error("expected false for zero context window")
		}
	})

	t.Run("negative context window returns false", func(t *testing.T) {
		p := makeProfile(-1, 4096)
		if ShouldTriggerSummarization(p, nil, "") {
			t.Error("expected false for negative context window")
		}
	})

	t.Run("budget becomes zero or negative returns false", func(t *testing.T) {
		p := makeProfile(1000, 800)
		msgs := makeMsgs(100, 1)
		if ShouldTriggerSummarization(p, msgs, "") {
			t.Error("expected false when budget <= 0")
		}
	})

	t.Run("under budget returns false", func(t *testing.T) {
		p := makeProfile(100000, 4096)
		msgs := makeMsgs(100, 10)
		if ShouldTriggerSummarization(p, msgs, "") {
			t.Error("expected false when estimated tokens < budget")
		}
	})

	t.Run("over budget returns true", func(t *testing.T) {
		p := makeProfile(8000, 2000)
		msgs := makeMsgs(400, 50)
		if !ShouldTriggerSummarization(p, msgs, "") {
			t.Error("expected true when estimated tokens > budget")
		}
	})

	t.Run("existing summary tokens count toward estimate", func(t *testing.T) {
		p := makeProfile(8000, 2000)
		msgs := makeMsgs(400, 30)
		longSummary := strings.Repeat("s", 4400)
		if !ShouldTriggerSummarization(p, msgs, longSummary) {
			t.Error("expected true when msgs + summary exceed budget")
		}
	})

	t.Run("default maxTokens when zero", func(t *testing.T) {
		p := &profiles.Profile{
			Chat: profiles.ChatConfig{
				ContextWindow: 20000,
				MaxTokens:     0,
			},
		}
		msgs := makeMsgs(40, 5)
		if ShouldTriggerSummarization(p, msgs, "") {
			t.Error("expected false, small messages under budget")
		}
	})
}

func TestBuildSummarizationUserPrompt(t *testing.T) {
	t.Run("without existing summary", func(t *testing.T) {
		msgs := []database.ChatMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there!", ToolCalls: `[{"id":"call_1","type":"function","function":{"name":"grep_search","arguments":"{}"},"result":"found it"}]`},
		}

		result := BuildSummarizationUserPrompt("", msgs)

		if !strings.Contains(result, "## Conversation to Summarize") {
			t.Error("expected '## Conversation to Summarize' header")
		}
		if strings.Contains(result, "## Previous Summary") {
			t.Error("should NOT contain '## Previous Summary' when no existing summary")
		}
		if !strings.Contains(result, "**[user]**: Hello") {
			t.Error("expected user message in output")
		}
		if !strings.Contains(result, "**[assistant]**: Hi there!") {
			t.Error("expected assistant message in output")
		}
		if !strings.Contains(result, "Tool result") || !strings.Contains(result, "found it") {
			t.Error("expected tool result in output")
		}
		if !strings.Contains(result, "Please produce a concise summary of the conversation above.") {
			t.Error("expected closing instruction for new summary")
		}
	})

	t.Run("tool_calls single-object is supported", func(t *testing.T) {
		msgs := []database.ChatMessage{
			{Role: "assistant", Content: "ok", ToolCalls: `{"id":"call_1","type":"function","function":{"name":"grep_search","arguments":"{}"},"result":"achou"}`},
		}
		result := BuildSummarizationUserPrompt("", msgs)
		if !strings.Contains(result, "Tool result") || !strings.Contains(result, "achou") {
			t.Error("expected tool result from single-object tool_calls in output")
		}
	})

	t.Run("with existing summary", func(t *testing.T) {
		msgs := []database.ChatMessage{
			{Role: "user", Content: "What about feature X?"},
		}

		result := BuildSummarizationUserPrompt("Previous context about the project.", msgs)

		if !strings.Contains(result, "## Previous Summary") {
			t.Error("expected '## Previous Summary' header")
		}
		if !strings.Contains(result, "Previous context about the project.") {
			t.Error("expected existing summary content")
		}
		if !strings.Contains(result, "## New Messages to Incorporate") {
			t.Error("expected '## New Messages to Incorporate' header")
		}
		if !strings.Contains(result, "Please produce an updated summary that integrates") {
			t.Error("expected closing instruction for incremental summary")
		}
	})

	t.Run("long content is truncated at 2000 chars", func(t *testing.T) {
		longContent := strings.Repeat("x", 3000)
		msgs := []database.ChatMessage{
			{Role: "user", Content: longContent},
		}

		result := BuildSummarizationUserPrompt("", msgs)

		if !strings.Contains(result, "... [truncated]") {
			t.Error("expected truncation marker for content > 2000 chars")
		}
		if strings.Contains(result, strings.Repeat("x", 2500)) {
			t.Error("content should be truncated, not include 2500+ chars")
		}
	})

	t.Run("empty messages produces minimal prompt", func(t *testing.T) {
		result := BuildSummarizationUserPrompt("", nil)

		if !strings.Contains(result, "## Conversation to Summarize") {
			t.Error("expected header even with no messages")
		}
		if !strings.Contains(result, "Please produce a concise summary") {
			t.Error("expected closing instruction")
		}
	})
}
