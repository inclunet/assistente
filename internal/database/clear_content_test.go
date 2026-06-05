package database

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"
)

var errForcedUpdateFail = errors.New("forced update fail (test)")

// seedChatToolInvocation cria uma ToolInvocation de chat ligada a uma mensagem
// (origin_id = messageID) do usuário de teste, para verificar que o clear a
// remove (sucesso) ou a preserva (rollback).
func seedChatToolInvocation(t *testing.T, messageID string) string {
	t.Helper()
	if err := db.AutoMigrate(&ToolInvocation{}); err != nil {
		t.Fatalf("migrate tool invocations: %v", err)
	}
	ti := &ToolInvocation{
		UserID:        testUserID,
		ToolCatalogID: "tool-x",
		OriginType:    "chat",
		OriginID:      messageID,
		Status:        "completed",
		QueuedAt:      time.Now(),
	}
	if err := db.Create(ti).Error; err != nil {
		t.Fatalf("criar tool invocation: %v", err)
	}
	return ti.ID
}

func countToolInvocations(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&ToolInvocation{}).Count(&n).Error; err != nil {
		t.Fatalf("count tool invocations: %v", err)
	}
	return n
}

// TestClearConversationContentWithContext cobre o caminho feliz: o clear apaga
// tool invocations + mensagens E zera o resumo de uma só vez.
func TestClearConversationContentWithContext(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()
	convID := createTestConversation(t, "sub")
	msgID := createTestMessage(t, convID, "user", "m1")
	createTestMessage(t, convID, "assistant", "m2")
	seedChatToolInvocation(t, msgID)
	if err := UpdateConversationSummaryWithContext(ctx, convID, "resumo", "1"); err != nil {
		t.Fatalf("set summary: %v", err)
	}

	if err := ClearConversationContentWithContext(ctx, convID); err != nil {
		t.Fatalf("clear: %v", err)
	}

	var msgCount int64
	if err := db.Model(&ChatMessage{}).Where("conversation_id = ?", convID).Count(&msgCount).Error; err != nil {
		t.Fatalf("count msgs: %v", err)
	}
	if msgCount != 0 {
		t.Fatalf("esperava 0 mensagens após clear, veio %d", msgCount)
	}
	if n := countToolInvocations(t); n != 0 {
		t.Fatalf("esperava 0 tool invocations após clear, veio %d", n)
	}
	summary, upTo, err := GetConversationSummaryWithContext(ctx, convID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary != "" || upTo != "" {
		t.Fatalf("esperava summary/upTo vazios após clear, veio %q/%q", summary, upTo)
	}
}

// TestClearConversationContentAtomicRollback garante a atomicidade TOTAL: se a
// limpeza do summary (última escrita) falhar, TODAS as escritas anteriores —
// delete de tool invocations e de mensagens — são REVERTIDAS. Sem estado
// parcialmente limpo.
func TestClearConversationContentAtomicRollback(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()
	convID := createTestConversation(t, "sub")
	msgID := createTestMessage(t, convID, "user", "m1")
	createTestMessage(t, convID, "assistant", "m2")
	seedChatToolInvocation(t, msgID)
	if err := UpdateConversationSummaryWithContext(ctx, convID, "resumo", "1"); err != nil {
		t.Fatalf("set summary: %v", err)
	}

	// Injeta falha na atualização de `conversations` (limpeza do summary) DENTRO da
	// transação do clear. O delete de tool invocations e de mensagens roda antes,
	// no mesmo tx → o erro deve reverter TUDO.
	const cbName = "force_update_fail_test"
	if err := db.Callback().Update().Before("gorm:update").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "conversations" {
			_ = tx.AddError(errForcedUpdateFail)
		}
	}); err != nil {
		t.Fatalf("registrar callback: %v", err)
	}
	removed := false
	removeCb := func() {
		if !removed {
			_ = db.Callback().Update().Remove(cbName)
			removed = true
		}
	}
	t.Cleanup(removeCb)

	if err := ClearConversationContentWithContext(ctx, convID); err == nil {
		t.Fatal("esperava erro quando a limpeza do summary falha")
	}

	// Remove o callback para inspecionar o estado pós-rollback sem interferência.
	removeCb()

	var msgCount int64
	if err := db.Model(&ChatMessage{}).Where("conversation_id = ?", convID).Count(&msgCount).Error; err != nil {
		t.Fatalf("count msgs: %v", err)
	}
	if msgCount != 2 {
		t.Fatalf("rollback deveria preservar as 2 mensagens; veio %d", msgCount)
	}
	if n := countToolInvocations(t); n != 1 {
		t.Fatalf("rollback deveria preservar a tool invocation; veio %d", n)
	}
	summary, _, err := GetConversationSummaryWithContext(ctx, convID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary != "resumo" {
		t.Fatalf("rollback deveria preservar o summary; veio %q", summary)
	}
}
