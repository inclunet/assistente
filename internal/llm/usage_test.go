package llm

import "testing"

func TestUsageFromOpenAICompletion_CachedTokens(t *testing.T) {
	usage := UsageFromOpenAICompletion(1000, 120, 1120, 400, "")
	if usage.CacheReadTokens != 400 {
		t.Fatalf("CacheReadTokens=%d, want 400", usage.CacheReadTokens)
	}
	if usage.CacheMissTokens != 600 {
		t.Fatalf("CacheMissTokens=%d, want 600", usage.CacheMissTokens)
	}
	if usage.CacheWriteTokens != 0 {
		t.Fatalf("CacheWriteTokens=%d, want 0", usage.CacheWriteTokens)
	}
}

func TestUsageFromOpenAICompletion_DeepSeekFields(t *testing.T) {
	raw := `{"prompt_cache_hit_tokens":256,"prompt_cache_miss_tokens":744}`
	usage := UsageFromOpenAICompletion(1000, 80, 1080, 0, raw)
	if usage.CacheReadTokens != 256 {
		t.Fatalf("CacheReadTokens=%d, want 256", usage.CacheReadTokens)
	}
	if usage.CacheMissTokens != 744 {
		t.Fatalf("CacheMissTokens=%d, want 744", usage.CacheMissTokens)
	}
}

func TestUsageFromAnthropic_CacheTokensArePartOfInput(t *testing.T) {
	usage := UsageFromAnthropic(700, 90, 200, 300)
	if usage.PromptTokens != 1200 {
		t.Fatalf("PromptTokens=%d, want 1200", usage.PromptTokens)
	}
	if usage.CacheWriteTokens != 200 {
		t.Fatalf("CacheWriteTokens=%d, want 200", usage.CacheWriteTokens)
	}
	if usage.CacheReadTokens != 300 {
		t.Fatalf("CacheReadTokens=%d, want 300", usage.CacheReadTokens)
	}
	if usage.CacheMissTokens != 700 {
		t.Fatalf("CacheMissTokens=%d, want 700", usage.CacheMissTokens)
	}
}

func TestUsageFromGemini_CachedContentTokenCount(t *testing.T) {
	usage := UsageFromGemini(900, 100, 1000, 250)
	if usage.CacheReadTokens != 250 {
		t.Fatalf("CacheReadTokens=%d, want 250", usage.CacheReadTokens)
	}
	if usage.CacheMissTokens != 650 {
		t.Fatalf("CacheMissTokens=%d, want 650", usage.CacheMissTokens)
	}
}
