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
	}
	enabled := true

	applyPromptCacheProfileState(stats, &enabled)

	if stats.PromptCacheEnabled == nil || !*stats.PromptCacheEnabled {
		t.Fatal("expected prompt cache to be marked as enabled")
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
