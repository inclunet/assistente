package database

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupOrderingTestDB creates an in-memory SQLite database for ordering tests
func setupOrderingTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

// createConvWithOrderedMessages creates a conversation with messages that have
// UUIDv7 IDs in reverse lexicographic order but ascending created_at.
// This simulates the real scenario where UUIDs generated in the same millisecond
// can be lexicographically "out of order" relative to insertion order.
func createConvWithOrderedMessages(t *testing.T) (convID string, msgIDs []string) {
	t.Helper()

	conv := &Conversation{Title: "ordering-test"}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	convID = conv.ID

	// UUIDs chosen so that lexicographic order (id-A < id-B < id-C) differs from
	// insertion order (id-C first, id-B second, id-A third) — i.e. created_at
	// order is the reverse of lexicographic order. This catches any code that
	// accidentally compares UUIDs lexicographically instead of using created_at.
	ids := []string{
		"01970fff-0001-7000-8000-000000000001", // created first  — lex "last" (fff)
		"01970aaa-0002-7000-8000-000000000002", // created second — lex "middle" (aaa)
		"019700bb-0003-7000-8000-000000000003", // created third  — lex "first" (0bb)
	}

	baseTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range ids {
		msg := ChatMessage{
			UUIDModel: UUIDModel{
				ID:        id,
				CreatedAt: baseTime.Add(time.Duration(i) * time.Minute),
			},
			ConversationID: convID,
			Role:           "user",
			Content:        "message " + id,
			TotalTokens:    (i + 1) * 10,
		}
		if err := db.Create(&msg).Error; err != nil {
			t.Fatalf("failed to create message %d: %v", i, err)
		}
	}

	return convID, ids
}

func TestGetMessagesAfterID_UsesCreatedAtNotLex(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithOrderedMessages(t)

	// Ask for messages after the first one (by created_at order).
	msgs, err := GetMessagesAfterID(convID, ids[0])
	if err != nil {
		t.Fatalf("GetMessagesAfterID: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after first, got %d", len(msgs))
	}
	if msgs[0].ID != ids[1] {
		t.Errorf("expected first result %s, got %s", ids[1], msgs[0].ID)
	}
	if msgs[1].ID != ids[2] {
		t.Errorf("expected second result %s, got %s", ids[2], msgs[1].ID)
	}
}

func TestGetMessagesAfterID_EmptyAfterID(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithOrderedMessages(t)

	msgs, err := GetMessagesAfterID(convID, "")
	if err != nil {
		t.Fatalf("GetMessagesAfterID empty: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected all 3 messages, got %d", len(msgs))
	}
	// Should be in created_at order
	for i, id := range ids {
		if msgs[i].ID != id {
			t.Errorf("msgs[%d]: expected %s, got %s", i, id, msgs[i].ID)
		}
	}
}

func TestGetMessagesBetweenIDs_UsesCreatedAtNotLex(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithOrderedMessages(t)

	msgs, err := GetMessagesBetweenIDs(convID, ids[0], ids[2])
	if err != nil {
		t.Fatalf("GetMessagesBetweenIDs: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages between first and last, got %d", len(msgs))
	}
	if msgs[0].ID != ids[1] {
		t.Errorf("expected first %s, got %s", ids[1], msgs[0].ID)
	}
	if msgs[1].ID != ids[2] {
		t.Errorf("expected last %s, got %s", ids[2], msgs[1].ID)
	}
}

func TestGetDetailedTokenStats_CutoffByCreatedAt(t *testing.T) {
	setupOrderingTestDB(t)
	convID, ids := createConvWithOrderedMessages(t)

	// ids[0] has 10 tokens, ids[1] has 20 tokens, ids[2] has 30 tokens
	// summaryUpToMessageID = ids[1] → out-of-context: ids[0]+ids[1] = 30 tokens,
	// in-context: ids[2] = 30 tokens
	stats, err := GetDetailedTokenStats(convID, ids[1])
	if err != nil {
		t.Fatalf("GetDetailedTokenStats: %v", err)
	}
	if stats.MessagesOutOfContextCount != 2 {
		t.Errorf("out-of-context count: expected 2, got %d", stats.MessagesOutOfContextCount)
	}
	if stats.MessagesOutOfContextTokens != 30 {
		t.Errorf("out-of-context tokens: expected 30, got %d", stats.MessagesOutOfContextTokens)
	}
	if stats.MessagesInContextCount != 1 {
		t.Errorf("in-context count: expected 1, got %d", stats.MessagesInContextCount)
	}
	if stats.MessagesInContextTokens != 30 {
		t.Errorf("in-context tokens: expected 30, got %d", stats.MessagesInContextTokens)
	}
}

func TestGetDetailedTokenStats_NoSummary(t *testing.T) {
	setupOrderingTestDB(t)
	convID, _ := createConvWithOrderedMessages(t)

	// No summary → all messages are in-context
	stats, err := GetDetailedTokenStats(convID, "")
	if err != nil {
		t.Fatalf("GetDetailedTokenStats: %v", err)
	}
	if stats.MessagesInContextCount != 3 {
		t.Errorf("in-context count: expected 3, got %d", stats.MessagesInContextCount)
	}
	if stats.MessagesInContextTokens != 60 {
		t.Errorf("in-context tokens: expected 60, got %d", stats.MessagesInContextTokens)
	}
	if stats.MessagesOutOfContextCount != 0 {
		t.Errorf("out-of-context count: expected 0, got %d", stats.MessagesOutOfContextCount)
	}
}
