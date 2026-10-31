package database

import (
	"context"
	"errors"
	"testing"
)

func TestChatSelectionReadersReportDeferredMigration(t *testing.T) {
	db := newChatTurnTestDB(t, false)
	seedChatTurnTestMessages(t, db, "conv", "turn", "response")
	if err := ensureSchemaMigrationsTable(db); err != nil {
		t.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user")
	repo := NewMessageRepository(db)
	if messages, err := repo.GetMessagesWithContext(ctx, "conv", nil); !errors.Is(err, ErrChatTurnMigrationPending) || len(messages) != 0 {
		t.Fatalf("leitura antes do upgrade: %v %+v", err, messages)
	}
	if window, err := repo.LoadHistoryWindowWithContext(ctx, "conv", 10); !errors.Is(err, ErrChatTurnMigrationPending) || window != nil {
		t.Fatalf("contexto antes do upgrade: %v %+v", err, window)
	}
	if err := MigrateChatTurns(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateChatExecutionContent(db); err != nil {
		t.Fatal(err)
	}
	if messages, err := repo.GetMessagesWithContext(ctx, "conv", nil); err != nil || len(messages) != 2 {
		t.Fatalf("leitura após schema: %v %+v", err, messages)
	}
	if window, err := repo.LoadHistoryWindowWithContext(ctx, "conv", 10); err != nil || len(window.Messages) != 2 {
		t.Fatalf("contexto após schema: %v %+v", err, window)
	}
}
