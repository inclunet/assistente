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
}
