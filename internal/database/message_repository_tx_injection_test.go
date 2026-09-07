package database

import (
	"context"
	"testing"
	"time"

	"assistente/internal/tools"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupTxInjectionTestDB prepara um banco em memória com o schema mínimo para o
// teste de contrato de injeção de transação (mensagens + tool invocations).
func setupTxInjectionTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Conversation{}, &ChatMessage{}, &ToolCatalog{}, &ToolInvocation{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
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

// TestMessageRepository_InjectedTxRollback_NothingPersists é um teste de
// CONTRATO da injeção de *gorm.DB (AEP-0040): constrói NewMessageRepository(tx)
// com uma transação separada, executa uma escrita destrutiva
// (DeleteMessageWithContext numa mensagem com tool invocations associadas),
// faz rollback e verifica que NADA foi persistido — incluindo tool_invocations.
//
// Se algum helper do caminho de deleção escapar do tx para a global `db`
// (regressão do slice por repositório), essa deleção seria auto-commitada fora
// da transação e sobreviveria ao Rollback, fazendo este teste FALHAR.
func TestMessageRepository_InjectedTxRollback_NothingPersists(t *testing.T) {
	setupTxInjectionTestDB(t)

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
	assistantMsg, err := CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "a1",
		TurnID:         &turnID,
		ToolCalls:      `[{"id":"call-1","type":"function","function":{"name":"echo","arguments":"{}"}}]`,
	})
	if err != nil {
		t.Fatalf("create assistant message: %v", err)
	}

	var tool ToolCatalog
	if err := db.WithContext(ctx).First(&tool, "name = ?", "echo").Error; err != nil {
		t.Fatalf("load tool catalog: %v", err)
	}
	queuedAt := time.Now()
	invTurn := ToolInvocation{UUIDModel: UUIDModel{ID: "inv-turn"}, UserID: "user-a", ToolCatalogID: tool.ID, OriginType: "chat", OriginID: userMsg.ID, ToolCallID: "call-1", Status: "succeeded", QueuedAt: queuedAt}
	invMsg := ToolInvocation{UUIDModel: UUIDModel{ID: "inv-msg"}, UserID: "user-a", ToolCatalogID: tool.ID, OriginType: "chat", OriginID: assistantMsg.ID, Status: "succeeded", QueuedAt: queuedAt}
	if err := db.WithContext(ctx).Create(&invTurn).Error; err != nil {
		t.Fatalf("seed invocation by turn: %v", err)
	}
	if err := db.WithContext(ctx).Create(&invMsg).Error; err != nil {
		t.Fatalf("seed invocation by message: %v", err)
	}

	// Estado esperado antes da operação: 2 mensagens, 2 tool invocations.
	const wantMessages = int64(2)
	const wantInvocations = int64(2)

	// Abre uma transação separada e injeta no repositório.
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx: %v", tx.Error)
	}
	repo := NewMessageRepository(tx)
	// Deletar a mensagem raiz (user) aciona o caminho completo de cleanup:
	// limpeza de tool_invocations do turno (deleteChatToolInvocationsForOriginIDs)
	// + deleção das mensagens do turno. Tudo DEVE rodar no tx injetado.
	if err := repo.DeleteMessageWithContext(ctx, userMsg.ID); err != nil {
		tx.Rollback()
		t.Fatalf("delete within tx: %v", err)
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// Após o rollback, NADA pode ter sido persistido. Se algum helper tivesse
	// usado a global `db`, suas deleções teriam sido commitadas e os counts
	// abaixo viriam menores que o esperado.
	var msgCount int64
	if err := db.WithContext(ctx).Model(&ChatMessage{}).
		Where("conversation_id = ?", conv.ID).Count(&msgCount).Error; err != nil {
		t.Fatalf("count messages after rollback: %v", err)
	}
	if msgCount != wantMessages {
		t.Fatalf("rollback não preservou mensagens: esperado %d, obtido %d (helper escapou do tx para a global)", wantMessages, msgCount)
	}

	var invCount int64
	if err := db.WithContext(ctx).Model(&ToolInvocation{}).
		Where("user_id = ? AND origin_type = ?", "user-a", "chat").Count(&invCount).Error; err != nil {
		t.Fatalf("count tool invocations after rollback: %v", err)
	}
	if invCount != wantInvocations {
		t.Fatalf("rollback não preservou tool_invocations: esperado %d, obtido %d (helper escapou do tx para a global)", wantInvocations, invCount)
	}
}
