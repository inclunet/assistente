package database

import "testing"

func TestCacheTokenMetricsPersistAndAggregate(t *testing.T) {
	setupOrderingTestDB(t)

	conv := &Conversation{Title: "cache-stats", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	turnID := "01972002-0000-7000-8000-000000000001"

	msg, err := CreateMessageWithContext(testCtx(), MessageOptions{
		ConversationID:   conv.ID,
		TurnID:           &turnID,
		Role:             "assistant",
		Content:          "ok",
		PromptTokens:     1000,
		CompletionTokens: 100,
		TotalTokens:      1100,
		CacheReadTokens:  350,
		CacheWriteTokens: 120,
		CacheMissTokens:  650,
		Model:            "cache-model",
	})
	if err != nil {
		t.Fatalf("CreateMessageWithContext: %v", err)
	}

	retrieved, err := GetMessageWithContext(testCtx(), msg.ID)
	if err != nil {
		t.Fatalf("GetMessageWithContext: %v", err)
	}
	if retrieved.CacheReadTokens != 350 || retrieved.CacheWriteTokens != 120 || retrieved.CacheMissTokens != 650 {
		t.Fatalf("cache tokens not persisted: read=%d write=%d miss=%d", retrieved.CacheReadTokens, retrieved.CacheWriteTokens, retrieved.CacheMissTokens)
	}

	stats, err := GetConversationDetailedTokenStatsWithContext(testCtx(), conv.ID)
	if err != nil {
		t.Fatalf("GetConversationDetailedTokenStatsWithContext: %v", err)
	}
	if stats.CacheReadTokens != 350 || stats.CacheWriteTokens != 120 || stats.CacheMissTokens != 650 {
		t.Fatalf("cache tokens not aggregated: read=%d write=%d miss=%d", stats.CacheReadTokens, stats.CacheWriteTokens, stats.CacheMissTokens)
	}

	turnStats, err := GetTurnTokenStatsWithContext(testCtx(), conv.ID, turnID)
	if err != nil {
		t.Fatalf("GetTurnTokenStatsWithContext: %v", err)
	}
	if turnStats.CacheReadTokens != 350 || turnStats.CacheWriteTokens != 120 || turnStats.CacheMissTokens != 650 {
		t.Fatalf("turn cache tokens not aggregated: read=%d write=%d miss=%d", turnStats.CacheReadTokens, turnStats.CacheWriteTokens, turnStats.CacheMissTokens)
	}
}

func TestUpdateMessageCacheTokensWithContext(t *testing.T) {
	setupOrderingTestDB(t)

	conv := &Conversation{Title: "cache-update", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	msg, err := CreateMessageWithContext(testCtx(), MessageOptions{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "placeholder",
	})
	if err != nil {
		t.Fatalf("CreateMessageWithContext: %v", err)
	}

	if err := UpdateMessageCacheTokensWithContext(testCtx(), msg.ID, 10, 20, 30); err != nil {
		t.Fatalf("UpdateMessageCacheTokensWithContext: %v", err)
	}

	retrieved, err := GetMessageWithContext(testCtx(), msg.ID)
	if err != nil {
		t.Fatalf("GetMessageWithContext: %v", err)
	}
	if retrieved.CacheReadTokens != 10 || retrieved.CacheWriteTokens != 20 || retrieved.CacheMissTokens != 30 {
		t.Fatalf("cache tokens not updated: read=%d write=%d miss=%d", retrieved.CacheReadTokens, retrieved.CacheWriteTokens, retrieved.CacheMissTokens)
	}

	if err := UpdateMessageCacheTokensWithContext(testCtx(), msg.ID, 40, 0, 0); err != nil {
		t.Fatalf("UpdateMessageCacheTokensWithContext partial: %v", err)
	}
	retrieved, err = GetMessageWithContext(testCtx(), msg.ID)
	if err != nil {
		t.Fatalf("GetMessageWithContext after partial update: %v", err)
	}
	if retrieved.CacheReadTokens != 40 || retrieved.CacheWriteTokens != 20 || retrieved.CacheMissTokens != 30 {
		t.Fatalf("partial update should preserve unreported cache tokens: read=%d write=%d miss=%d", retrieved.CacheReadTokens, retrieved.CacheWriteTokens, retrieved.CacheMissTokens)
	}

	if err := UpdateMessageCacheTokensWithContext(testCtx(), msg.ID, 0, 0, 0); err != nil {
		t.Fatalf("UpdateMessageCacheTokensWithContext zeros: %v", err)
	}
	retrieved, err = GetMessageWithContext(testCtx(), msg.ID)
	if err != nil {
		t.Fatalf("GetMessageWithContext after zero update: %v", err)
	}
	if retrieved.CacheReadTokens != 40 || retrieved.CacheWriteTokens != 20 || retrieved.CacheMissTokens != 30 {
		t.Fatalf("zero update should preserve cache tokens: read=%d write=%d miss=%d", retrieved.CacheReadTokens, retrieved.CacheWriteTokens, retrieved.CacheMissTokens)
	}

	if err := UpdateMessageContentAndReasoningWithContext(testCtx(), msg.ID, "new response", "", 5, 6, 11, "model"); err != nil {
		t.Fatalf("UpdateMessageContentAndReasoningWithContext: %v", err)
	}
	retrieved, err = GetMessageWithContext(testCtx(), msg.ID)
	if err != nil {
		t.Fatalf("GetMessageWithContext after content update: %v", err)
	}
	if retrieved.CacheReadTokens != 40 || retrieved.CacheWriteTokens != 20 || retrieved.CacheMissTokens != 30 {
		t.Fatalf("content-only update should preserve cache tokens: read=%d write=%d miss=%d", retrieved.CacheReadTokens, retrieved.CacheWriteTokens, retrieved.CacheMissTokens)
	}

	if err := UpdateMessageContentReasoningAndUsageWithContext(testCtx(), msg.ID, "final response", "", 7, 8, 15, 0, 0, 0, "model"); err != nil {
		t.Fatalf("UpdateMessageContentReasoningAndUsageWithContext: %v", err)
	}
	retrieved, err = GetMessageWithContext(testCtx(), msg.ID)
	if err != nil {
		t.Fatalf("GetMessageWithContext after atomic usage update: %v", err)
	}
	if retrieved.CacheReadTokens != 0 || retrieved.CacheWriteTokens != 0 || retrieved.CacheMissTokens != 0 {
		t.Fatalf("atomic usage update should clear stale cache tokens: read=%d write=%d miss=%d", retrieved.CacheReadTokens, retrieved.CacheWriteTokens, retrieved.CacheMissTokens)
	}
}

func TestCacheTokenStatsTreatLegacyNullCacheTokensAsZero(t *testing.T) {
	setupOrderingTestDB(t)

	conv := &Conversation{Title: "legacy-cache-null", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	turnID := "01972002-0000-7000-8000-000000000002"
	msg, err := CreateMessageWithContext(testCtx(), MessageOptions{
		ConversationID:   conv.ID,
		TurnID:           &turnID,
		Role:             "assistant",
		Content:          "legacy",
		PromptTokens:     11,
		CompletionTokens: 7,
		TotalTokens:      18,
	})
	if err != nil {
		t.Fatalf("CreateMessageWithContext: %v", err)
	}
	if err := db.Model(&ChatMessage{}).Where("id = ?", msg.ID).Updates(map[string]any{
		"cache_read_tokens":  nil,
		"cache_write_tokens": nil,
		"cache_miss_tokens":  nil,
	}).Error; err != nil {
		t.Fatalf("set legacy null cache tokens: %v", err)
	}

	conversationStats, err := GetConversationTokenStatsWithContext(testCtx(), conv.ID)
	if err != nil {
		t.Fatalf("GetConversationTokenStatsWithContext: %v", err)
	}
	if conversationStats["cache_read_tokens"] != 0 || conversationStats["cache_write_tokens"] != 0 || conversationStats["cache_miss_tokens"] != 0 {
		t.Fatalf("conversation stats should coalesce null cache tokens: %+v", conversationStats)
	}
	allStats, err := GetAllTokenStatsWithContext(testCtx())
	if err != nil {
		t.Fatalf("GetAllTokenStatsWithContext: %v", err)
	}
	if allStats["cache_read_tokens"] != 0 || allStats["cache_write_tokens"] != 0 || allStats["cache_miss_tokens"] != 0 {
		t.Fatalf("all stats should coalesce null cache tokens: %+v", allStats)
	}
	turnStats, err := GetTurnTokenStatsWithContext(testCtx(), conv.ID, turnID)
	if err != nil {
		t.Fatalf("GetTurnTokenStatsWithContext: %v", err)
	}
	if turnStats.CacheReadTokens != 0 || turnStats.CacheWriteTokens != 0 || turnStats.CacheMissTokens != 0 {
		t.Fatalf("turn stats should coalesce null cache tokens: %+v", turnStats)
	}
	detailedStats, err := GetConversationDetailedTokenStatsWithContext(testCtx(), conv.ID)
	if err != nil {
		t.Fatalf("GetConversationDetailedTokenStatsWithContext: %v", err)
	}
	if detailedStats.CacheReadTokens != 0 || detailedStats.CacheWriteTokens != 0 || detailedStats.CacheMissTokens != 0 {
		t.Fatalf("detailed stats should coalesce null cache tokens: %+v", detailedStats)
	}
}
