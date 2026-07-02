package database

import (
	"context"
	"testing"
	"time"

	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDeleteMessageToolInvocationsTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Conversation{}, &ChatMessage{}, &ToolCatalog{}, &ToolInvocation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed usuário e tool (ToolInvocation tem FK para ambos)
	if err := db.Create(&User{UUIDModel: UUIDModel{ID: "user-a"}, Username: "a", DisplayName: "A", PasswordHash: "x", Role: UserRoleUser, IsActive: true}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&ToolCatalog{Name: "echo", DisplayName: "echo", Origin: tools.ToolOriginBuiltin, AvailabilityStatus: tools.ToolAvailabilityAvailable}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

func TestDeleteMessageWithContext_DeletesChatToolInvocationsByTurnIDAndMessageID(t *testing.T) {
	setupDeleteMessageToolInvocationsTestDB(t)

	ctx := WithUserID(context.Background(), "user-a")
	conv, err := CreateConversationWithContext(ctx, "Conv", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	userMsg, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conv.ID, Role: "user", Content: "u1"})
	if err != nil {
		t.Fatalf("create user message: %v", err)
	}
	turnID := userMsg.ID
	assistantMsg, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conv.ID, Role: "assistant", Content: "a1", TurnID: &turnID, ToolCalls: `[{"id":"call-1","type":"function","function":{"name":"echo","arguments":"{}"}}]`})
	if err != nil {
		t.Fatalf("create assistant message: %v", err)
	}

	var tool ToolCatalog
	if err := db.WithContext(ctx).First(&tool, "name = ?", "echo").Error; err != nil {
		t.Fatalf("load tool catalog: %v", err)
	}

	queuedAt := time.Now()
	invByTurnOther := ToolInvocation{UUIDModel: UUIDModel{ID: "inv-turn-other"}, UserID: "user-a", ToolCatalogID: tool.ID, OriginType: "chat", OriginID: userMsg.ID, ToolCallID: "call-2", Status: "succeeded", QueuedAt: queuedAt}
	invByTurnCall := ToolInvocation{UUIDModel: UUIDModel{ID: "inv-turn-call"}, UserID: "user-a", ToolCatalogID: tool.ID, OriginType: "chat", OriginID: userMsg.ID, ToolCallID: "call-1", Status: "succeeded", QueuedAt: queuedAt}
	invByMessage := ToolInvocation{UUIDModel: UUIDModel{ID: "inv-msg"}, UserID: "user-a", ToolCatalogID: tool.ID, OriginType: "chat", OriginID: assistantMsg.ID, Status: "succeeded", QueuedAt: queuedAt}
	if err := db.WithContext(ctx).Create(&invByTurnOther).Error; err != nil {
		t.Fatalf("seed invocation by turn (other): %v", err)
	}
	if err := db.WithContext(ctx).Create(&invByTurnCall).Error; err != nil {
		t.Fatalf("seed invocation by turn (call): %v", err)
	}
	if err := db.WithContext(ctx).Create(&invByMessage).Error; err != nil {
		t.Fatalf("seed invocation by message: %v", err)
	}

	if err := DeleteMessageWithContext(ctx, assistantMsg.ID); err != nil {
		t.Fatalf("delete assistant message: %v", err)
	}

	var count int64
	if err := db.WithContext(ctx).Model(&ToolInvocation{}).Where("user_id = ? AND origin_type = ?", "user-a", "chat").Count(&count).Error; err != nil {
		t.Fatalf("count tool invocations: %v", err)
	}
	// Deletar uma mensagem assistant com TurnID não deve apagar invocações do turno inteiro.
	// Deve remover: inv-msg (origin_id=assistant) e inv-turn-call (turn origin + tool_call_id=call-1)
	// Deve manter: inv-turn-other (turn origin + tool_call_id=call-2)
	if count != 1 {
		t.Fatalf("expected only unrelated turn invocations to remain when deleting assistant, got %d", count)
	}
	// Agora deletando a user message raiz, o turno inteiro deve ser limpo.
	if err := DeleteMessageWithContext(ctx, userMsg.ID); err != nil {
		t.Fatalf("delete user message: %v", err)
	}
	if err := db.WithContext(ctx).Model(&ToolInvocation{}).Where("user_id = ? AND origin_type = ?", "user-a", "chat").Count(&count).Error; err != nil {
		t.Fatalf("count tool invocations after root delete: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected all turn invocations removed when deleting user root, got %d", count)
	}
}

func TestDeleteMessageWithContext_DeletesTurnInvocationsForAssistantWithoutToolCalls(t *testing.T) {
	setupDeleteMessageToolInvocationsTestDB(t)

	ctx := WithUserID(context.Background(), "user-a")
	conv, err := CreateConversationWithContext(ctx, "Conv", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	userMsg, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conv.ID, Role: "user", Content: "u1"})
	if err != nil {
		t.Fatalf("create user message: %v", err)
	}
	turnID := userMsg.ID
	assistantMsg, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conv.ID, Role: "assistant", Content: "vou buscar", TurnID: &turnID})
	if err != nil {
		t.Fatalf("create assistant message: %v", err)
	}
	var tool ToolCatalog
	if err := db.WithContext(ctx).First(&tool, "name = ?", "echo").Error; err != nil {
		t.Fatalf("load tool catalog: %v", err)
	}
	if err := db.WithContext(ctx).Create(&ToolInvocation{
		UUIDModel:     UUIDModel{ID: "inv-turn-l3-free"},
		UserID:        "user-a",
		ToolCatalogID: tool.ID,
		OriginType:    "chat",
		OriginID:      turnID,
		ToolCallID:    "call-1",
		Status:        "succeeded",
		Metadata:      `{"display":{"assistant_message_id":"` + assistantMsg.ID + `"}}`,
		QueuedAt:      time.Now(),
	}).Error; err != nil {
		t.Fatalf("seed invocation: %v", err)
	}

	if err := DeleteMessageWithContext(ctx, assistantMsg.ID); err != nil {
		t.Fatalf("delete assistant message: %v", err)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&ToolInvocation{}).Where("user_id = ? AND origin_type = ?", "user-a", "chat").Count(&count).Error; err != nil {
		t.Fatalf("count tool invocations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected l3-free assistant deletion to remove turn invocation, got %d", count)
	}
}

func TestDeleteMessageWithContext_DeletesUntaggedInvocationForL3FreeAssistant(t *testing.T) {
	setupDeleteMessageToolInvocationsTestDB(t)

	ctx := WithUserID(context.Background(), "user-a")
	conv, err := CreateConversationWithContext(ctx, "Conv", "")
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	userMsg, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conv.ID, Role: "user", Content: "u1"})
	if err != nil {
		t.Fatalf("create user message: %v", err)
	}
	turnID := userMsg.ID
	assistantMsg, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conv.ID, Role: "assistant", Content: "vou buscar", TurnID: &turnID})
	if err != nil {
		t.Fatalf("create assistant message: %v", err)
	}
	otherAssistant, err := CreateMessageWithContext(ctx, MessageOptions{ConversationID: conv.ID, Role: "assistant", Content: "outra iteracao", TurnID: &turnID})
	if err != nil {
		t.Fatalf("create other assistant message: %v", err)
	}
	queuedAt := assistantMsg.CreatedAt.Add(-time.Second)
	futureQueuedAt := otherAssistant.CreatedAt.Add(time.Second)
	var tool ToolCatalog
	if err := db.WithContext(ctx).First(&tool, "name = ?", "echo").Error; err != nil {
		t.Fatalf("load tool catalog: %v", err)
	}
	rows := []ToolInvocation{
		{
			UUIDModel:     UUIDModel{ID: "inv-untagged"},
			UserID:        "user-a",
			ToolCatalogID: tool.ID,
			OriginType:    "chat",
			OriginID:      turnID,
			ToolCallID:    "call-untagged",
			Status:        "succeeded",
			QueuedAt:      queuedAt,
		},
		{
			UUIDModel:     UUIDModel{ID: "inv-future-untagged"},
			UserID:        "user-a",
			ToolCatalogID: tool.ID,
			OriginType:    "chat",
			OriginID:      turnID,
			ToolCallID:    "call-future",
			Status:        "succeeded",
			QueuedAt:      futureQueuedAt,
		},
		{
			UUIDModel:     UUIDModel{ID: "inv-other-assistant"},
			UserID:        "user-a",
			ToolCatalogID: tool.ID,
			OriginType:    "chat",
			OriginID:      turnID,
			ToolCallID:    "call-other",
			Status:        "succeeded",
			Metadata:      `{"display":{"assistant_message_id":"` + otherAssistant.ID + `"}}`,
			QueuedAt:      futureQueuedAt,
		},
	}
	if err := db.WithContext(ctx).Create(&rows).Error; err != nil {
		t.Fatalf("seed invocations: %v", err)
	}

	if err := DeleteMessageWithContext(ctx, assistantMsg.ID); err != nil {
		t.Fatalf("delete assistant message: %v", err)
	}
	var remaining []ToolInvocation
	if err := db.WithContext(ctx).Where("user_id = ? AND origin_type = ?", "user-a", "chat").Find(&remaining).Error; err != nil {
		t.Fatalf("load remaining tool invocations: %v", err)
	}
	got := map[string]bool{}
	for _, inv := range remaining {
		got[inv.ToolCallID] = true
	}
	if len(got) != 2 || !got["call-other"] || !got["call-future"] {
		t.Fatalf("expected other assistant and future untagged invocations to remain, got %+v", remaining)
	}
}
