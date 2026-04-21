package agent

import (
	"strings"
	"testing"
	"unicode/utf8"

	"assistente/internal/llm"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"abcd", 1},    // 4 chars = 1 token
		{"abcde", 2},   // 5 chars = 2 tokens (ceil)
		{"12345678", 2}, // 8 chars = 2 tokens
	}
	for _, tt := range tests {
		got := estimateTokens(tt.input)
		if got != tt.expected {
			t.Errorf("estimateTokens(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "Hello world!"},  // 12 chars = 3 tokens + 4 overhead = 7
		{Role: "assistant", Content: "Hi there"}, // 8 chars = 2 tokens + 4 overhead = 6
	}
	got := estimateMessageTokens(msgs)
	// 3 + 4 + 2 + 4 = 13
	if got != 13 {
		t.Errorf("estimateMessageTokens = %d, want 13", got)
	}
}

func TestPreCheckContextWindow_NoLimit(t *testing.T) {
	results := []string{"some content", "more content"}
	check := PreCheckContextWindow(0, 4096, nil, results)
	if check.Truncated {
		t.Error("should not truncate when contextLimit=0")
	}
}

func TestPreCheckContextWindow_FitsInBudget(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "hello"},
	}
	// Small results that fit easily
	results := []string{"result 1", "result 2"}
	check := PreCheckContextWindow(100000, 4096, msgs, results)
	if check.Truncated {
		t.Error("should not truncate when results fit in budget")
	}
}

func TestPreCheckContextWindow_TruncatesWhenOverBudget(t *testing.T) {
	// Create a tight context: 1000 token limit, 500 response tokens
	// Existing messages: ~100 tokens
	// Available: 1000*0.9 - 100 - 500 = 300 tokens = ~1200 bytes
	existingContent := strings.Repeat("x", 400) // ~100 tokens
	msgs := []llm.Message{
		{Role: "user", Content: existingContent},
	}

	// Each result ~2000 bytes = ~500 tokens (way over 300 budget)
	result1 := strings.Repeat("a", 2000)
	result2 := strings.Repeat("b", 2000)
	results := []string{result1, result2}

	check := PreCheckContextWindow(1000, 500, msgs, results)
	if !check.Truncated {
		t.Fatal("should truncate when results exceed budget")
	}
	if check.FinalTokens >= check.OriginalTokens {
		t.Errorf("FinalTokens (%d) should be less than OriginalTokens (%d)",
			check.FinalTokens, check.OriginalTokens)
	}
	// Verify results were actually truncated
	if len(results[0]) >= 2000 || len(results[1]) >= 2000 {
		t.Error("results should have been truncated in-place")
	}
	// Verify truncation notice
	if !strings.Contains(results[0], "[CONTEXTO TRUNCADO:") {
		t.Error("result should contain truncation notice")
	}
}

func TestPreCheckContextWindow_UTF8Safe(t *testing.T) {
	// Content with multibyte chars
	msgs := []llm.Message{
		{Role: "user", Content: strings.Repeat("x", 400)},
	}

	// Result with multibyte UTF-8 content: "日本語" = 9 bytes for 3 chars
	multibyteContent := strings.Repeat("日本語テスト", 200) // lots of multibyte content
	results := []string{multibyteContent}

	check := PreCheckContextWindow(500, 200, msgs, results)
	if !check.Truncated {
		t.Fatal("should truncate")
	}
	if !utf8.ValidString(results[0]) {
		t.Error("truncated result should be valid UTF-8")
	}
}

func TestPreCheckContextWindow_MinResultSize(t *testing.T) {
	// Under extreme pressure, results are truncated proportionally to available budget.
	// When the budget is smaller than minResultContextSize * len(results), the effective
	// minimum is reduced to fit within the budget (preventing budget overflow).
	msgs := []llm.Message{
		{Role: "user", Content: strings.Repeat("x", 3600)}, // ~900 tokens
	}

	// Tiny budget remaining
	result := strings.Repeat("a", 1000)
	results := []string{result}

	check := PreCheckContextWindow(1000, 50, msgs, results)
	if check.Truncated {
		// Verify the result is not empty — it should have at least some content
		if len(results[0]) == 0 {
			t.Error("result was truncated to empty string")
		}
	}
}

func TestPreCheckContextWindow_AvailableLessThanResults(t *testing.T) {
	// When availableBytes < len(toolResults) and scaling zeroes all quotas,
	// the fallback must not exceed availableBytes (no overflow).
	// estimateMessageTokens("x"*372) = ceil(372/4) + 4 overhead = 97 tokens.
	// contextWindow=115, safeLimit=int(115*0.9)=103, reserve=5
	// availableTokens=103-97-5=1 → availableBytes=4.
	// 50 tool results → availableBytes(4) < nResults(50).
	msgs := []llm.Message{
		{Role: "user", Content: strings.Repeat("x", 372)},
	}

	results := make([]string, 50)
	for i := range results {
		results[i] = strings.Repeat("a", 100)
	}

	check := PreCheckContextWindow(115, 5, msgs, results)

	totalBytes := 0
	for _, r := range results {
		totalBytes += len(r)
	}

	// availableBytes=4; sum of truncated results must not exceed it.
	if totalBytes > 4 {
		t.Errorf("total result bytes %d exceeds available budget 4", totalBytes)
	}
	if !check.Truncated {
		t.Error("expected Truncated=true")
	}
}

func TestTruncateUTF8Safe(t *testing.T) {
	// "café" in UTF-8: c(1) a(1) f(1) é(2) = 5 bytes
	s := "café"
	result := truncateUTF8Safe(s, 4)
	if !utf8.ValidString(result) {
		t.Fatalf("not valid UTF-8: %q", result)
	}
	if result != "caf" {
		t.Fatalf("expected 'caf', got %q", result)
	}

	// Exact fit
	result = truncateUTF8Safe(s, 5)
	if result != "café" {
		t.Fatalf("expected 'café', got %q", result)
	}

	// Over limit (no truncation needed)
	result = truncateUTF8Safe(s, 100)
	if result != "café" {
		t.Fatalf("expected 'café', got %q", result)
	}
}
