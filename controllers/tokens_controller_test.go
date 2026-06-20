package controllers

import (
	"testing"

	"assistente/internal/chat"
)

func TestApplyPromptCacheProfileStateMarksProfileState(t *testing.T) {
	stats := &chat.TokenStats{
		PromptTokens: 100,
	}

	applyPromptCacheProfileState(stats, true)

	if !stats.PromptCacheEnabled {
		t.Fatal("expected prompt cache to be marked as enabled")
	}
}

func TestApplyPromptCacheProfileStateHandlesReportedMetrics(t *testing.T) {
	stats := &chat.TokenStats{
		PromptTokens:        100,
		CacheTokensReported: true,
		CacheReadTokens:     40,
	}

	applyPromptCacheProfileState(stats, true)

	if !stats.PromptCacheEnabled {
		t.Fatal("expected prompt cache to be marked as enabled")
	}
}
