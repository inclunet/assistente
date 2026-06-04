package database

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

var errForcedUpdateFail = errors.New("forced update fail (test)")

// TestClearConversationContentWithContext cobre o caminho feliz: o clear apaga
// mensagens E zera o resumo de uma só vez.
func TestClearConversationContentWithContext(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()
	convID := createTestConversation(t, "sub")
	createTestMessage(t, convID, "user", "m1")
	createTestMessage(t, convID, "assistant", "m2")
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
	summary, upTo, err := GetConversationSummaryWithContext(ctx, convID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary != "" || upTo != "" {
		t.Fatalf("esperava summary/upTo vazios após clear, veio %q/%q", summary, upTo)
	}
}

// TestClearConversationContentAtomicRollback garante a atomicidade: se a 2ª
// escrita destrutiva (limpeza do summary) falhar, a 1ª (delete de mensagens) é
// REVERTIDA — sem estado parcialmente limpo (summary antigo apontando para
// mensagens já apagadas).
func TestClearConversationContentAtomicRollback(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()
	convID := createTestConversation(t, "sub")
	createTestMessage(t, convID, "user", "m1")
	createTestMessage(t, convID, "assistant", "m2")
	if err := UpdateConversationSummaryWithContext(ctx, convID, "resumo", "1"); err != nil {
		t.Fatalf("set summary: %v", err)
	}

	// Injeta falha na atualização de `conversations` (limpeza do summary) DENTRO da
	// transação do clear. O delete de mensagens roda antes, no mesmo tx → o erro
	// deve reverter ambos.
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
	summary, _, err := GetConversationSummaryWithContext(ctx, convID)
	if err != nil {
		t.Fatalf("get summary: %v", err)
	}
	if summary != "resumo" {
		t.Fatalf("rollback deveria preservar o summary; veio %q", summary)
	}
}
