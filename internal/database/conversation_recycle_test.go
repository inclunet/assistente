package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestDBWithTabs(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	if err := db.AutoMigrate(&Conversation{}, &ChatMessage{}, &ChatTab{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		db = nil
	})
}

func TestRecycleOrCreateConversation_CreatesWhenNoCandidates(t *testing.T) {
	setupTestDBWithTabs(t)

	conv, err := RecycleOrCreateConversation("Minha Conversa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.Title != "Minha Conversa" {
		t.Errorf("expected title 'Minha Conversa', got %q", conv.Title)
	}
	if conv.ID == 0 {
		t.Error("expected non-zero ID")
	}

	var count int64
	db.Model(&Conversation{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 conversation, got %d", count)
	}
}

func TestRecycleOrCreateConversation_RecyclesEmptyOrphan(t *testing.T) {
	setupTestDBWithTabs(t)

	orphan, err := CreateConversation("Conversa Orfã", "")
	if err != nil {
		t.Fatalf("failed to create orphan: %v", err)
	}
	orphanID := orphan.ID

	conv, err := RecycleOrCreateConversation("Reciclada")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conv.ID != orphanID {
		t.Errorf("expected recycled ID %d, got %d", orphanID, conv.ID)
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
	setupTestDBWithTabs(t)

	conv, _ := CreateConversation("Com Mensagens", "")
	db.Create(&ChatMessage{ConversationID: conv.ID, Role: "user", Content: "oi"})

	result, err := RecycleOrCreateConversation("Nova")
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

func TestRecycleOrCreateConversation_DoesNotRecycleLinkedToTab(t *testing.T) {
	setupTestDBWithTabs(t)

	conv, _ := CreateConversation("Vinculada", "")
	tab := &ChatTab{Title: "Tab", ConversationID: &conv.ID, Position: 0}
	db.Create(tab)

	result, err := RecycleOrCreateConversation("Nova")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID == conv.ID {
		t.Error("should NOT recycle conversation linked to a tab")
	}
}

func TestRecycleOrCreateConversation_DoesNotRecycleChannelConversation(t *testing.T) {
	setupTestDBWithTabs(t)

	conv := &Conversation{Title: "Signal Chat", Channel: "signal", ContactID: "+5511999"}
	db.Create(conv)

	result, err := RecycleOrCreateConversation("Nova")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID == conv.ID {
		t.Error("should NOT recycle conversation with channel")
	}
}

func TestRecycleOrCreateConversation_RecyclesOldestFirst(t *testing.T) {
	setupTestDBWithTabs(t)

	first, _ := CreateConversation("Primeira", "")
	second, _ := CreateConversation("Segunda", "")

	result, err := RecycleOrCreateConversation("Reciclada")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != first.ID {
		t.Errorf("expected to recycle oldest (ID=%d), got ID=%d", first.ID, result.ID)
	}

	// Segunda ainda existe
	var remaining Conversation
	if err := db.First(&remaining, second.ID).Error; err != nil {
		t.Error("second conversation should still exist")
	}
}

func TestRecycleOrCreateConversation_ResetsSummaryFields(t *testing.T) {
	setupTestDBWithTabs(t)

	conv, _ := CreateConversation("Com Resumo", "")
	db.Model(conv).Updates(map[string]interface{}{
		"summary":                  "resumo antigo",
		"summary_up_to_message_id": 42,
	})

	result, err := RecycleOrCreateConversation("Limpa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != conv.ID {
		t.Fatalf("expected recycled ID %d", conv.ID)
	}
	if result.Summary != "" {
		t.Errorf("expected empty summary, got %q", result.Summary)
	}
	if result.SummaryUpToMessageID != 0 {
		t.Errorf("expected SummaryUpToMessageID=0, got %d", result.SummaryUpToMessageID)
	}
}

// ==================== EnsureTabsHaveConversation ====================

func TestEnsureTabsHaveConversation_FixesOrphanTabs(t *testing.T) {
	setupTestDBWithTabs(t)

	tab := &ChatTab{Title: "Orfã", Position: 0, IsActive: true}
	if err := db.Create(tab).Error; err != nil {
		t.Fatalf("failed to create tab: %v", err)
	}

	if tab.ConversationID != nil {
		t.Fatal("tab should start without conversation")
	}

	if err := EnsureTabsHaveConversation(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated ChatTab
	db.First(&updated, tab.ID)
	if updated.ConversationID == nil {
		t.Error("tab should have conversation after EnsureTabsHaveConversation")
	}
}

func TestEnsureTabsHaveConversation_DoesNotTouchTabsWithConversation(t *testing.T) {
	setupTestDBWithTabs(t)

	conv, _ := CreateConversation("Existente", "")
	tab := &ChatTab{Title: "Com Conversa", ConversationID: &conv.ID, Position: 0}
	db.Create(tab)

	if err := EnsureTabsHaveConversation(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated ChatTab
	db.First(&updated, tab.ID)
	if updated.ConversationID == nil || *updated.ConversationID != conv.ID {
		t.Errorf("expected conversation_id=%d, got %v", conv.ID, updated.ConversationID)
	}

	var count int64
	db.Model(&Conversation{}).Count(&count)
	if count != 1 {
		t.Errorf("should not create extra conversations, got %d", count)
	}
}

func TestEnsureTabsHaveConversation_RecyclesEmptyConversation(t *testing.T) {
	setupTestDBWithTabs(t)

	orphanConv, _ := CreateConversation("Orfã", "")

	tab := &ChatTab{Title: "Sem Conversa", Position: 0}
	db.Create(tab)

	if err := EnsureTabsHaveConversation(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated ChatTab
	db.First(&updated, tab.ID)
	if updated.ConversationID == nil {
		t.Fatal("tab should have conversation")
	}

	if *updated.ConversationID != orphanConv.ID {
		t.Errorf("expected recycled conversation %d, got %d", orphanConv.ID, *updated.ConversationID)
	}

	var count int64
	db.Model(&Conversation{}).Count(&count)
	if count != 1 {
		t.Errorf("should have recycled, not created — expected 1 conversation, got %d", count)
	}
}

func TestEnsureTabsHaveConversation_NoopWhenAllTabsHaveConversation(t *testing.T) {
	setupTestDBWithTabs(t)

	conv, _ := CreateConversation("OK", "")
	tab := &ChatTab{Title: "Boa", ConversationID: &conv.ID, Position: 0}
	db.Create(tab)

	var countBefore int64
	db.Model(&Conversation{}).Count(&countBefore)

	if err := EnsureTabsHaveConversation(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var countAfter int64
	db.Model(&Conversation{}).Count(&countAfter)
	if countAfter != countBefore {
		t.Errorf("conversation count changed: %d -> %d", countBefore, countAfter)
	}
}
