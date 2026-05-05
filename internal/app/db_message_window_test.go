package app

import (
	"strings"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupMessageWindowAppTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetDB(db)
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
}

func TestGetConversationMessageWindow_ValidatesRequestShape(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := database.AddMessage(conv.ID, "user", "mensagem"); err != nil {
		t.Fatalf("create message: %v", err)
	}

	_, err = app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeConversation,
		Direction:      "sideways",
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "direction") {
		t.Fatalf("expected direction validation error, got %v", err)
	}

	_, err = app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeConversation,
		Direction:      chat.MessageWindowDirectionAround,
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "anchorMessageId") {
		t.Fatalf("expected around anchor validation error, got %v", err)
	}
}

func TestGetConversationMessageWindow_RejectsNestedThreadParent(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	root, err := database.AddMessage(conv.ID, "assistant", "root")
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	child, err := database.AddChildMessage(conv.ID, root.ID, "assistant", "child", "")
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	_, err = app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeThread,
		ThreadParentID: child.ID,
		Anchor:         chat.MessageWindowAnchorStart,
		Direction:      chat.MessageWindowDirectionAfter,
		Limit:          10,
	})
	if err == nil || !strings.Contains(err.Error(), "mensagem raiz") {
		t.Fatalf("expected root thread parent validation error, got %v", err)
	}
}

func TestGetConversationMessageWindow_NormalizesAnchorNotFound(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if _, err := database.AddMessage(conv.ID, "user", "mensagem"); err != nil {
		t.Fatalf("create message: %v", err)
	}

	_, err = app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID:  conv.ID,
		Scope:           chat.MessageWindowScopeConversation,
		AnchorMessageID: "missing-message",
		Direction:       chat.MessageWindowDirectionAfter,
		Limit:           10,
	})
	if err == nil || !strings.Contains(err.Error(), "anchorMessageId inválido") {
		t.Fatalf("expected normalized anchor error, got %v", err)
	}
}
