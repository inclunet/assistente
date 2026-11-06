package database

import (
	"assistente/internal/turnlease"
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestTurnExecutionAtomicResponseAndLateWrites(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user")
	repo := NewChatTurnRepository(db)
	attempt, err := repo.Begin(ctx, "conv", "turn", "", TurnModeRetry)
	if err != nil {
		t.Fatal(err)
	}
	responseID, err := repo.EnsureResponse(ctx, "conv", "turn", attempt.ExecutionID)
	if err != nil || responseID == "response" || responseID == "" {
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
	if message.Content != "" || message.TotalTokens != 0 {
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
	nextResponseID, err := repo.EnsureResponse(ctx, "conv", "turn", next.ExecutionID)
	if err != nil || nextResponseID == responseID {
		t.Fatalf("retry reutilizou resposta: %q %v", nextResponseID, err)
	}
	if err := repo.WritePartial(ctx, next.ExecutionID, responseID, "indevida", "", false); !errors.Is(err, ErrChatTurnStale) {
		t.Fatalf("execução nova alterou resposta antiga: %v", err)
	}
	if err := repo.WritePartial(ctx, next.ExecutionID, nextResponseID, "parcial", "", false); err != nil {
		t.Fatal(err)
	}
	message = ChatMessage{}
	if err := db.First(&message, "id=?", nextResponseID).Error; err != nil {
		t.Fatal(err)
	}
	if message.Content != "parcial" || message.TotalTokens != 0 || message.CacheReadTokens != 0 {
		t.Fatalf("partial herdou usage: %+v", message)
	}
	message = ChatMessage{}
	if err := db.First(&message, "id=?", responseID).Error; err != nil {
		t.Fatal(err)
	}
	if message.Content != "nova resposta" || message.TotalTokens != 11 || message.CacheReadTokens != 3 {
		t.Fatalf("execução anterior alterada: %+v", message)
	}

}

func TestTurnIntermediateTransfersContentAtomically(t *testing.T) {
	for _, failure := range []string{"", "insert", "clear"} {
		t.Run(failure, func(t *testing.T) {
			db := newChatTurnTestDB(t, false)
			seedChatTurnTestMessages(t, db, "conv", "turn", "response")
			if err := MigrateChatTurns(db); err != nil {
				t.Fatal(err)
			}
			if err := MigrateChatExecutionContent(db); err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&ChatMessage{}).Where("id = ?", "response").Updates(map[string]any{"content": "prefixo", "reasoning": "pensamento"}).Error; err != nil {
				t.Fatal(err)
			}
			ctx := WithUserID(context.Background(), "user")
			repo := NewChatTurnRepository(db)
			attempt, err := repo.Begin(ctx, "conv", "turn", "", TurnModeContinue)
			if err != nil {
				t.Fatal(err)
			}
			responseID, err := repo.EnsureResponse(ctx, "conv", "turn", attempt.ExecutionID)
			if err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "insert":
				if err := db.Exec(`CREATE TRIGGER reject_intermediate BEFORE INSERT ON chat_messages BEGIN SELECT RAISE(ABORT,'inserção indisponível'); END`).Error; err != nil {
					t.Fatal(err)
				}
			case "clear":
				if err := db.Exec(`CREATE TRIGGER reject_transfer BEFORE UPDATE OF content ON chat_messages WHEN NEW.content='' BEGIN SELECT RAISE(ABORT,'transferência indisponível'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			turnID := "turn"
			opts := MessageOptions{ConversationID: "conv", TurnID: &turnID, ChatExecutionID: &attempt.ExecutionID, Role: "assistant", Content: "prefixo e novo", Reasoning: "pensamento e novo", Model: "modelo"}
			previousActivity := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
			if err := db.Model(&Conversation{}).Where("id = ?", "conv").Update("updated_at", previousActivity).Error; err != nil {
				t.Fatal(err)
			}
			intermediate, err := repo.SaveIntermediate(ctx, attempt.ExecutionID, responseID, opts)
			if (err != nil) != (failure != "") {
				t.Fatalf("transferência: %v", err)
			}
			var current, original ChatMessage
			var conversation Conversation
			if err := db.First(&conversation, "id = ?", "conv").Error; err != nil {
				t.Fatal(err)
			}
			if failure == "" && !conversation.UpdatedAt.After(previousActivity) || failure != "" && !conversation.UpdatedAt.Equal(previousActivity) {
				t.Fatalf("activity transaction: before=%v after=%v failure=%s", previousActivity, conversation.UpdatedAt, failure)
			}
			if err := db.First(&current, "id=?", responseID).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&original, "id=?", "response").Error; err != nil {
				t.Fatal(err)
			}
			if original.Content != "prefixo" || original.Reasoning != "pensamento" {
				t.Fatalf("base anterior modificada: %+v", original)
			}
			var count int64
			if err := db.Model(&ChatMessage{}).Where("chat_execution_id=? AND id<>?", attempt.ExecutionID, responseID).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if failure != "" {
				if intermediate != nil || count != 0 || current.Content != "prefixo" || current.Reasoning != "pensamento" {
					t.Fatalf("rollback incompleto: count=%d current=%+v intermediate=%+v", count, current, intermediate)
				}
				return
			}
			if count != 1 || current.Content != "" || current.Reasoning != "" || intermediate.Content != opts.Content || intermediate.Reasoning != opts.Reasoning {
				t.Fatalf("fontes duplicadas/perdidas: current=%+v intermediate=%+v", current, intermediate)
			}
			if _, err := repo.SaveIntermediate(ctx, attempt.ExecutionID, "response", opts); !errors.Is(err, ErrChatTurnStale) {
				t.Fatalf("aceitou representante anterior: %v", err)
			}
			if _, err := repo.SaveIntermediate(WithUserID(context.Background(), "outro"), attempt.ExecutionID, responseID, opts); err == nil {
				t.Fatal("aceitou outro usuário")
			}
			if err := repo.FinishResponse(ctx, "conv", turnID, attempt.ExecutionID, TurnInterrupted, "restart", "", nil); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.SaveIntermediate(ctx, attempt.ExecutionID, responseID, opts); !errors.Is(err, ErrChatTurnStale) {
				t.Fatalf("aceitou execução encerrada: %v", err)
			}
		})
	}
}

func TestTurnExecutionReconcilesLegacyButPreservesCanonicalResponse(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "old")
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err != nil {
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
	if err != nil || responseID == "final" || responseID == "" {
		t.Fatalf("reconciliação: %q %v", responseID, err)
	}
	var copy ChatMessage
	if err := db.First(&copy, "id=?", responseID).Error; err != nil {
		t.Fatal(err)
	}
	if copy.Content != "final histórico" || attempt.BaseExecutionID != nil || attempt.BaseResponseMessageID == nil || *attempt.BaseResponseMessageID != "final" {
		t.Fatalf("base legada não preservada: %+v %+v", copy, attempt)
	}
	continuationID := responseID

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
	if err != nil || responseID == "final" || responseID == continuationID || responseID == "" {
		t.Fatalf("reclassificou turno canônico: %q %v", responseID, err)
	}
	copy = ChatMessage{}
	if err := db.First(&copy, "id=?", continuationID).Error; err != nil {
		t.Fatal(err)
	}
	if copy.Content != "final histórico" {
		t.Fatal("retry apagou continuação anterior")
	}

}

func TestTurnExecutionStartupRecoversBeforeReconciliation(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "")
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err != nil {
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
	seedChatTurnTestMessages(t, db, "conv", "lost-lease", "")
	lost, err := repo.Begin(ctx, "conv", "lost-lease", "", TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	lostLease, err := turnlease.Create(ctx, t.TempDir(), lost.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turn_attempts SET lease_path=? WHERE execution_id=?", lostLease.Path, lost.ExecutionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := lostLease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(lostLease.Path); err != nil {
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
	unproven, err := repo.Get(ctx, "conv", "lost-lease")
	if err != nil {
		t.Fatal(err)
	}
	if unproven.ActiveExecutionID == nil || *unproven.ActiveExecutionID != lost.ExecutionID {
		t.Fatalf("inferiu morte sem posse: %+v", unproven)
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
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_unchanged BEFORE UPDATE ON chat_turns WHEN NEW.response_message_id IS OLD.response_message_id BEGIN SELECT RAISE(ABORT,'update redundante'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := reconcileLegacyChatTurns(db, "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestTurnExecutionStartupBeforeContentProvenanceMigration(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "")
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := migrateChatTurnLeases(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn("chat_turns", "current_execution_id") {
		t.Fatal("fixture já contém v44")
	}
	if err := ensureSchemaMigrationsTable(db); err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{41, 42} {
		if err := recordMigration(db, migration{Version: version, Name: "startup-fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := WithUserID(context.Background(), "user")
	lease, err := turnlease.Create(ctx, t.TempDir(), "01926b90-7a5a-7c4e-8d3f-000000000199")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO chat_turn_attempts(execution_id,turn_id,mode,state,started_at,terminal_reason,lease_path) VALUES(?,?,?,?,?,?,?)", "01926b90-7a5a-7c4e-8d3f-000000000199", "turn", TurnModeNew, TurnStreaming, time.Now(), "", lease.Path).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE chat_turns SET state=?,active_execution_id=? WHERE user_message_id=?", TurnStreaming, "01926b90-7a5a-7c4e-8d3f-000000000199", "turn").Error; err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := initializeChatTurnsBeforeWorkers(db); err != nil {
		t.Fatal(err)
	}
	turn, err := NewChatTurnRepository(db).Get(ctx, "conv", "turn")
	if err != nil || turn.State != TurnInterrupted || turn.ActiveExecutionID != nil {
		t.Fatalf("startup v42: %+v %v", turn, err)
	}
}
