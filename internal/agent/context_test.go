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
		{"abcd", 1},     // 4 chars = 1 token
		{"abcde", 2},    // 5 chars = 2 tokens (ceil)
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
		{Role: "user", Content: "Hello world!"}, // 12 chars = 3 tokens + 4 overhead = 7
		{
			Role:             "assistant",
			Content:          "Hi there",
			ReasoningContent: "12345678", // 8 chars = 2 tokens
		}, // 2 content + 2 reasoning + 4 overhead = 8
	}
	got := estimateMessageTokens(msgs, true)
	// 3 + 4 + 2 + 2 + 4 = 15
	if got != 15 {
		t.Errorf("estimateMessageTokens = %d, want 15", got)
	}
}

func TestEstimateMessageTokens_IgnoraReasoningSemReplay(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "Hello world!"}, // 3 + 4 overhead
		{
			Role:             "assistant",
			Content:          "Hi there",
			ReasoningContent: "12345678", // não vai no wire sem replay
		}, // 2 + 4 overhead
	}
	got := estimateMessageTokens(msgs, false)
	// 3 + 4 + 2 + 4 = 13
	if got != 13 {
		t.Errorf("estimateMessageTokens sem replay = %d, want 13", got)
	}
}

func TestPreCheckContextWindow_NoLimit(t *testing.T) {
	results := []string{"some content", "more content"}
	check := PreCheckContextWindow(0, 4096, nil, results, false)
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
	check := PreCheckContextWindow(100000, 4096, msgs, results, false)
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

	check := PreCheckContextWindow(1000, 500, msgs, results, false)
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

	check := PreCheckContextWindow(500, 200, msgs, results, false)
	if !check.Truncated {
		t.Fatal("should truncate")
	}
	if !utf8.ValidString(results[0]) {
		t.Error("truncated result should be valid UTF-8")
	}
}

func TestPreCheckContextWindow_MinResultSize(t *testing.T) {
	// Under extreme pressure (budget zero or negative), results are completely
	// removed to avoid adding tokens when there's no budget available.
	msgs := []llm.Message{
		{Role: "user", Content: strings.Repeat("x", 3600)}, // ~900 tokens
	}

	// Tiny budget remaining: safeLimit=900, msgTokens=904, reserve=50 → available<0
	result := strings.Repeat("a", 1000)
	results := []string{result}

	check := PreCheckContextWindow(1000, 50, msgs, results, false)
	if !check.Truncated {
		t.Error("expected Truncated=true when budget is zero/negative")
	}
	// Budget is zero/negative — result should be completely emptied
	if len(results[0]) != 0 {
		t.Errorf("result should be empty when budget is zero, got %d bytes", len(results[0]))
	}
	if check.FinalTokens != 0 {
		t.Errorf("FinalTokens should be 0 when budget is zero, got %d", check.FinalTokens)
	}
}

func TestPreCheckContextWindow_AvailableLessThanResults(t *testing.T) {
	// With 50 tool results, overhead alone is 50*4=200 tokens.
	// estimateMessageTokens("x"*372) = ceil(372/4) + 4 overhead = 97 tokens.
	// contextWindow=115, safeLimit=103, reserve=5
	// available=103-97-5-200 = -199 → clamped to 0 → all results emptied.
	msgs := []llm.Message{
		{Role: "user", Content: strings.Repeat("x", 372)},
	}

	results := make([]string, 50)
	for i := range results {
		results[i] = strings.Repeat("a", 100)
	}

	check := PreCheckContextWindow(115, 5, msgs, results, false)

	totalBytes := 0
	for _, r := range results {
		totalBytes += len(r)
	}

	// Budget is zero (tool overhead alone exceeds available), all results emptied.
	if totalBytes != 0 {
		t.Errorf("total result bytes %d should be 0 (budget zero with tool overhead)", totalBytes)
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

func TestPreCheckContextWindow_ToolOverheadReducesBudget(t *testing.T) {
	// Verify that tool message overhead (4 tokens per result) is subtracted
	// from the available budget. With many tool results, this matters.
	msgs := []llm.Message{
		{Role: "user", Content: "hi"}, // ~1 token + 4 overhead = 5 tokens
	}

	// 10 small results — each 20 bytes = 5 tokens
	results := make([]string, 10)
	for i := range results {
		results[i] = strings.Repeat("x", 20)
	}

	// contextLimit=200, safeLimit=180, reserve=50
	// existingTokens=5, toolOverhead=10*4=40
	// available=180-5-50-40=85 tokens
	// resultTokens = 10 * 5 = 50 → fits in 85
	check := PreCheckContextWindow(200, 50, msgs, results, false)
	if check.Truncated {
		t.Error("should not truncate: results fit with overhead accounted")
	}

	// Tight budget where overhead tips it over:
	// contextLimit=110, safeLimit=99, reserve=10
	// existingTokens=5, toolOverhead=10*4=40
	// available=99-5-10-40=44 tokens
	// resultTokens=50 > 44 → must truncate
	results2 := make([]string, 10)
	for i := range results2 {
		results2[i] = strings.Repeat("x", 20)
	}
	check2 := PreCheckContextWindow(110, 10, msgs, results2, false)
	if !check2.Truncated {
		t.Error("should truncate when tool overhead reduces budget below result tokens")
	}
}
