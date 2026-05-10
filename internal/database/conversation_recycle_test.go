package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupRecycleTestDB(t *testing.T) {
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

func TestRecycleOrCreateConversation_CreatesWhenNoCandidates(t *testing.T) {
	setupRecycleTestDB(t)
	ctx := testCtx()

	conv, err := RecycleOrCreateConversationWithContext(ctx, "Minha Conversa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.Title != "Minha Conversa" {
		t.Errorf("expected title 'Minha Conversa', got %q", conv.Title)
	}
	if conv.ID == "" {
		t.Error("expected non-empty ID")
	}

	var count int64
	db.Model(&Conversation{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 conversation, got %d", count)
	}
}

func TestRecycleOrCreateConversation_RecyclesEmptyOrphan(t *testing.T) {
	setupRecycleTestDB(t)
	ctx := testCtx()

	orphan, err := CreateConversationWithContext(ctx, "Conversa Orfã", "")
	if err != nil {
		t.Fatalf("failed to create orphan: %v", err)
	}
	orphanID := orphan.ID

	conv, err := RecycleOrCreateConversationWithContext(ctx, "Reciclada")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conv.ID != orphanID {
		t.Errorf("expected recycled ID %s, got %s", orphanID, conv.ID)
	}
	if conv.Title != "Reciclada" {
		t.Errorf("expected title 'Reciclada', got %q", conv.Title)
	}

	var count int64
	db.Model(&Conversation{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 conversation (recycled), got %d", count)
	}
}

func TestRecycleOrCreateConversation_DoesNotRecycleWithMessages(t *testing.T) {
	setupRecycleTestDB(t)
	ctx := testCtx()

	conv, _ := CreateConversationWithContext(ctx, "Com Mensagens", "")
	db.Create(&ChatMessage{ConversationID: conv.ID, Role: "user", Content: "oi"})

	result, err := RecycleOrCreateConversationWithContext(ctx, "Nova")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID == conv.ID {
		t.Error("should NOT recycle conversation that has messages")
	}

	var count int64
	db.Model(&Conversation{}).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 conversations, got %d", count)
	}
}

func TestRecycleOrCreateConversation_DoesNotRecycleChannelConversation(t *testing.T) {
	setupRecycleTestDB(t)
	ctx := testCtx()

	conv := &Conversation{Title: "Signal Chat", Channel: "signal", ContactID: "+5511999", UserID: testUserID}
	db.Create(conv)

	result, err := RecycleOrCreateConversationWithContext(ctx, "Nova")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID == conv.ID {
		t.Error("should NOT recycle conversation with channel")
	}
}

func TestRecycleOrCreateConversation_RecyclesOldestFirst(t *testing.T) {
	setupRecycleTestDB(t)
	ctx := testCtx()

	first, _ := CreateConversationWithContext(ctx, "Primeira", "")
	second, _ := CreateConversationWithContext(ctx, "Segunda", "")

	result, err := RecycleOrCreateConversationWithContext(ctx, "Reciclada")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != first.ID {
		t.Errorf("expected to recycle oldest (ID=%s), got ID=%s", first.ID, result.ID)
	}

	var remaining Conversation
	if err := db.First(&remaining, "id = ?", second.ID).Error; err != nil {
		t.Error("second conversation should still exist")
	}
}

func TestRecycleOrCreateConversation_ResetsSummaryFields(t *testing.T) {
	setupRecycleTestDB(t)
	ctx := testCtx()

	conv, _ := CreateConversationWithContext(ctx, "Com Resumo", "")
	db.Model(conv).Updates(map[string]interface{}{
		"summary":                  "resumo antigo",
		"summary_up_to_message_id": 42,
	})

	result, err := RecycleOrCreateConversationWithContext(ctx, "Limpa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != conv.ID {
		t.Fatalf("expected recycled ID %s", conv.ID)
	}
	if result.Summary != "" {
		t.Errorf("expected empty summary, got %q", result.Summary)
	}
	if result.SummaryUpToMessageID != "" {
		t.Errorf("expected SummaryUpToMessageID=empty, got %s", result.SummaryUpToMessageID)
	}
}
