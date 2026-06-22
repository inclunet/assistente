package controllers

import (
	"testing"

	"assistente/internal/chat"
)

func TestApplyPromptCacheProfileStateMarksProfileState(t *testing.T) {
	stats := &chat.TokenStats{
		PromptTokens: 100,
	}
	enabled := true

	applyPromptCacheProfileState(stats, &enabled)

	if stats.PromptCacheEnabled == nil || !*stats.PromptCacheEnabled {
		t.Fatal("expected prompt cache to be marked as enabled")
	}
}

func TestApplyPromptCacheProfileStateHandlesReportedMetrics(t *testing.T) {
	stats := &chat.TokenStats{
		PromptTokens:        100,
		CacheTokensReported: true,
		CacheReadTokens:     40,
		CacheHitRate:        25,
	}
	enabled := true

	applyPromptCacheProfileState(stats, &enabled)

	if stats.PromptCacheEnabled == nil || !*stats.PromptCacheEnabled {
		t.Fatal("expected prompt cache to be marked as enabled")
	}
	if !stats.CacheTokensReported || stats.CacheReadTokens != 40 || stats.CacheHitRate != 25 {
		t.Fatal("expected existing cache metrics to be preserved")
	}
}

func TestApplyPromptCacheProfileStateKeepsUnknownState(t *testing.T) {
	stats := &chat.TokenStats{
		PromptTokens: 100,
	}

	applyPromptCacheProfileState(stats, nil)

	if stats.PromptCacheEnabled != nil {
		t.Fatal("expected prompt cache state to remain unknown")
	}
}
