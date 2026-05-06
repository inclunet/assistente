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

func TestGetConversationMessageWindow_ClampsOversizedLimit(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	for i := 0; i < database.MaxMessageWindowRows+30; i++ {
		if _, err := database.AddMessage(conv.ID, "user", "mensagem"); err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
	}

	window, err := app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID: conv.ID,
		Scope:          chat.MessageWindowScopeConversation,
		Anchor:         chat.MessageWindowAnchorEnd,
		Direction:      chat.MessageWindowDirectionBefore,
		Limit:          database.MaxMessageWindowRows + 60,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if len(window.Nodes) != database.MaxMessageWindowRows {
		t.Fatalf("expected clamped window size %d, got %d", database.MaxMessageWindowRows, len(window.Nodes))
	}
}

func TestGetConversationMessageWindow_ExpandsTurnBoundaries(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	user, err := database.AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assistant, err := database.AddAssistantToolMessage(
		conv.ID,
		user.ID,
		"vou buscar",
		`[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("create assistant tool call: %v", err)
	}
	toolResult, err := database.AddToolResultMessage(conv.ID, user.ID, "resultado", "tool-1")
	if err != nil {
		t.Fatalf("create tool result: %v", err)
	}

	window, err := app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID:  conv.ID,
		Scope:           chat.MessageWindowScopeConversation,
		AnchorMessageID: assistant.ID,
		Direction:       chat.MessageWindowDirectionAfter,
		Limit:           1,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}

	ids := make(map[string]bool)
	for _, node := range window.Nodes {
		ids[node.Message.ID] = true
	}
	if !ids[assistant.ID] {
		t.Fatalf("expected assistant message to be included, got ids=%v", ids)
	}
	if len(window.Nodes) < 2 {
		t.Fatalf("expected turn expansion to include more than raw limited row, got %d node(s)", len(window.Nodes))
	}
	for _, node := range window.Nodes {
		if node.Message.ID == assistant.ID && node.OriginalIndex != nil {
			t.Fatalf("expanded assistant outside raw window should not receive invented originalIndex")
		}
		if node.Message.ID == toolResult.ID && node.OriginalIndex == nil {
			t.Fatalf("raw window message should preserve originalIndex")
		}
	}
}

func TestGetConversationMessageWindow_BoundsTurnExpansion(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	app := &App{}

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	user, err := database.AddMessage(conv.ID, "user", "pergunta")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	assistant, err := database.AddAssistantToolMessage(
		conv.ID,
		user.ID,
		"vou buscar",
		`[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("create assistant tool call: %v", err)
	}
	for i := 0; i < database.MaxMessageWindowRows+50; i++ {
		if _, err := database.AddToolResultMessage(conv.ID, user.ID, "resultado", "tool-1"); err != nil {
			t.Fatalf("create tool result %d: %v", i, err)
		}
	}

	window, err := app.GetConversationMessageWindow(chat.MessageWindowRequest{
		ConversationID:  conv.ID,
		Scope:           chat.MessageWindowScopeConversation,
		AnchorMessageID: assistant.ID,
		Direction:       chat.MessageWindowDirectionAfter,
		Limit:           1,
	})
	if err != nil {
		t.Fatalf("get window: %v", err)
	}
	if len(window.Nodes) > database.MaxMessageWindowRows+1 {
		t.Fatalf("turn expansion should stay bounded, got %d node(s)", len(window.Nodes))
	}
}

func TestExpandWindowTurnMessages_DoesNotStarveLaterTurns(t *testing.T) {
	setupMessageWindowAppTestDB(t)

	conv, err := database.CreateConversation("Conversa", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	userOne, err := database.AddMessage(conv.ID, "user", "pergunta 1")
	if err != nil {
		t.Fatalf("create user one: %v", err)
	}
	assistantOne, err := database.AddAssistantToolMessage(
		conv.ID,
		userOne.ID,
		"vou buscar 1",
		`[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("create assistant one: %v", err)
	}
	var firstTurnTool *database.ChatMessage
	for i := 0; i < 20; i++ {
		tool, err := database.AddToolResultMessage(conv.ID, userOne.ID, "resultado 1", "tool-1")
		if err != nil {
			t.Fatalf("create first turn tool %d: %v", i, err)
		}
		if firstTurnTool == nil {
			firstTurnTool = tool
		}
	}
	userTwo, err := database.AddMessage(conv.ID, "user", "pergunta 2")
	if err != nil {
		t.Fatalf("create user two: %v", err)
	}
	assistantTwo, err := database.AddAssistantToolMessage(
		conv.ID,
		userTwo.ID,
		"vou buscar 2",
		`[{"id":"tool-2","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		"",
		"",
	)
	if err != nil {
		t.Fatalf("create assistant two: %v", err)
	}
	secondTurnTool, err := database.AddToolResultMessage(conv.ID, userTwo.ID, "resultado 2", "tool-2")
	if err != nil {
		t.Fatalf("create second turn tool: %v", err)
	}

	messages, err := expandWindowTurnMessages(conv.ID, nil, []database.ChatMessage{*firstTurnTool, *secondTurnTool}, 6)
	if err != nil {
		t.Fatalf("expand messages: %v", err)
	}
	ids := make(map[string]bool)
	for _, message := range messages {
		ids[message.ID] = true
	}
	if !ids[assistantOne.ID] {
		t.Fatalf("expected first large turn to keep a non-tool anchor")
	}
	if !ids[assistantTwo.ID] || !ids[secondTurnTool.ID] {
		t.Fatalf("expected later turn to expand despite earlier large turn, got ids=%v", ids)
	}
}
