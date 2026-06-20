package controllers

import (
	"testing"

	"assistente/internal/chat"
)

func TestApplyPromptCacheNoticeMarksProfileStateWithoutInferringProviderReporting(t *testing.T) {
	stats := &chat.TokenStats{
		PromptTokens: 100,
	}

	applyPromptCacheNotice(stats, true)

	if !stats.PromptCacheEnabled {
		t.Fatal("expected prompt cache to be marked as enabled")
	}
	if stats.PromptCacheNotice != "" {
		t.Fatalf("unexpected prompt cache notice: got %q want empty", stats.PromptCacheNotice)
	}
}

func TestApplyPromptCacheNoticeSkipsWarningWhenMetricsExist(t *testing.T) {
	stats := &chat.TokenStats{
		PromptTokens:        100,
		CacheTokensReported: true,
		CacheReadTokens:     40,
	}

	applyPromptCacheNotice(stats, true)

	if stats.PromptCacheNotice != "" {
		t.Fatalf("unexpected prompt cache notice: got %q want empty", stats.PromptCacheNotice)
	}
}
