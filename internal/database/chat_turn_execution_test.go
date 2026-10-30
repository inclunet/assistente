package database

import (
	"assistente/internal/turnlease"
	"context"
	"errors"
	"testing"
	"time"
)

func TestTurnExecutionAtomicResponseAndLateWrites(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user")
	repo := NewChatTurnRepository(db)
	attempt, err := repo.Begin(ctx, "conv", "turn", "", TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	responseID, err := repo.EnsureResponse(ctx, "conv", "turn", attempt.ExecutionID)
	if err != nil || responseID != "response" {
		t.Fatalf("resposta: %q %v", responseID, err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_terminal BEFORE UPDATE ON chat_turn_attempts WHEN NEW.state='completed' BEGIN SELECT RAISE(ABORT,'terminal indisponível'); END`).Error; err != nil {
		t.Fatal(err)
	}
	response := &MessageOptions{Content: "nova resposta", TotalTokens: 11, CacheReadTokens: 3}
	if err := repo.FinishResponse(ctx, "conv", "turn", attempt.ExecutionID, TurnCompleted, "completed", responseID, response); err == nil {
		t.Fatal("aceitou terminal rejeitado")
	}
	var message ChatMessage
	if err := db.First(&message, "id = ?", responseID).Error; err != nil {
		t.Fatal(err)
	}
	if message.Content != "antes" || message.TotalTokens != 0 {
		t.Fatalf("resposta vazou do rollback: %+v", message)
	}
	if err := db.Exec("DROP TRIGGER reject_terminal").Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishResponse(ctx, "conv", "turn", attempt.ExecutionID, TurnCompleted, "completed", responseID, response); err != nil {
		t.Fatal(err)
	}
	next, err := repo.Begin(ctx, "conv", "turn", "", TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.WritePartial(ctx, attempt.ExecutionID, responseID, "atrasada", "", false); !errors.Is(err, ErrChatTurnStale) {
		t.Fatalf("write antigo: %v", err)
	}
	if err := repo.FinishResponse(ctx, "conv", "turn", attempt.ExecutionID, TurnCompleted, "completed", responseID, &MessageOptions{Content: "atrasada"}); !errors.Is(err, ErrChatTurnStale) {
		t.Fatalf("terminal antigo: %v", err)
	}
	if err := repo.WritePartial(ctx, next.ExecutionID, responseID, "parcial", "", false); err != nil {
		t.Fatal(err)
	}
	message = ChatMessage{}
	if err := db.First(&message, "id = ?", responseID).Error; err != nil {
		t.Fatal(err)
	}
	if message.Content != "parcial" || message.TotalTokens != 11 || message.CacheReadTokens != 3 {
		t.Fatalf("partial alterou usage: %+v", message)
	}
}

func TestTurnExecutionReconcilesLegacyButPreservesCanonicalResponse(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "old")
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	turnID := "turn"
	newer := ChatMessage{UUIDModel: UUIDModel{ID: "final", CreatedAt: time.Now().Add(time.Hour)}, ConversationID: "conv", TurnID: &turnID, Role: "assistant", Content: "final histórico"}
	if err := db.Create(&newer).Error; err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user")
	repo := NewChatTurnRepository(db)
	attempt, err := repo.Begin(ctx, "conv", turnID, "", TurnModeContinue)
	if err != nil {
		t.Fatal(err)
	}
	responseID, err := repo.EnsureResponse(ctx, "conv", turnID, attempt.ExecutionID)
	if err != nil || responseID != "final" {
		t.Fatalf("reconciliação: %q %v", responseID, err)
	}
	if err := repo.Finish(ctx, "conv", turnID, attempt.ExecutionID, TurnInterrupted, "cancelled"); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&ChatMessage{}).Where("id='old'").Update("updated_at", time.Now().Add(2*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	next, err := repo.Begin(ctx, "conv", turnID, "", TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	responseID, err = repo.EnsureResponse(ctx, "conv", turnID, next.ExecutionID)
	if err != nil || responseID != "final" {
		t.Fatalf("reclassificou turno canônico: %q %v", responseID, err)
	}
}

func TestTurnExecutionStartupRecoversBeforeReconciliation(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "")
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := ensureSchemaMigrationsTable(db); err != nil {
		t.Fatal(err)
	}
	if err := recordMigration(db, migration{Version: 41, Name: "chat_turns_and_attempts"}); err != nil {
		t.Fatal(err)
	}
	if err := migrateChatTurnLeases(db); err != nil {
		t.Fatal(err)
	}
	if err := recordMigration(db, migration{Version: 42, Name: "chat_turn_execution_leases"}); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user")
	repo := NewChatTurnRepository(db)
	attempt, err := repo.Begin(ctx, "conv", "turn", "", TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := turnlease.Create(ctx, t.TempDir(), attempt.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turn_attempts SET lease_path=? WHERE execution_id=?", lease.Path, attempt.ExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	seedChatTurnTestMessages(t, db, "conv", "imported-after-migration", "imported-response")
	if err := initializeChatTurnsBeforeWorkers(db); err != nil {
		t.Fatal(err)
	}
	turn, err := repo.Get(ctx, "conv", "turn")
	if err != nil || turn.State != TurnInterrupted || turn.ActiveExecutionID != nil {
		t.Fatalf("recuperação: %+v %v", turn, err)
	}
	var stored ChatTurnAttempt
	if err := db.First(&stored, "execution_id = ?", attempt.ExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TerminalReason != "restart" || stored.FinishedAt == nil {
		t.Fatalf("tentativa: %+v", stored)
	}
	imported, err := repo.Get(ctx, "conv", "imported-after-migration")
	if err != nil || imported.ResponseMessageID == nil || *imported.ResponseMessageID != "imported-response" {
		t.Fatalf("importação: %+v %v", imported, err)
	}
}

func TestTurnExecutionReconciliationDoesNotRewriteUnchangedResponse(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	if err := migrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_unchanged BEFORE UPDATE ON chat_turns WHEN NEW.response_message_id IS OLD.response_message_id BEGIN SELECT RAISE(ABORT,'update redundante'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := reconcileLegacyChatTurns(db, "", ""); err != nil {
		t.Fatal(err)
	}
}
