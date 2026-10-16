package chat

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestHistoryLoader_AnchoredStoreParityWithLegacyOracle(t *testing.T) {
	testDB, ctx, conversationID := setupHotPathDB(t)
	if err := testDB.Exec("CREATE INDEX IF NOT EXISTS idx_chat_messages_window ON chat_messages (conversation_id, parent_id, created_at, id)").Error; err != nil {
		t.Fatalf("create retry window index: %v", err)
	}
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	boundary := "retry-a1"
	messages := []database.ChatMessage{
		{UUIDModel: database.UUIDModel{ID: "retry-u0", CreatedAt: base}, ConversationID: conversationID, Role: "user", Content: "pergunta inicial"},
		{UUIDModel: database.UUIDModel{ID: boundary, CreatedAt: base.Add(time.Minute)}, ConversationID: conversationID, Role: "assistant", Content: "resposta anterior"},
		{UUIDModel: database.UUIDModel{ID: "retry-system", CreatedAt: base.Add(2 * time.Minute)}, ConversationID: conversationID, Role: "system", Content: "contexto irregular"},
		{UUIDModel: database.UUIDModel{ID: "retry-tool", CreatedAt: base.Add(3 * time.Minute)}, ConversationID: conversationID, Role: "tool", Content: "resultado"},
		{UUIDModel: database.UUIDModel{ID: "retry-target", CreatedAt: base.Add(4 * time.Minute)}, ConversationID: conversationID, Role: "user", Content: "pergunta com mídia", Media: `[{"type":"image","url":"fixture"}]`},
		{UUIDModel: database.UUIDModel{ID: "retry-after", CreatedAt: base.Add(5 * time.Minute)}, ConversationID: conversationID, Role: "assistant", Content: "não deve vazar"},
	}
	if err := testDB.Create(&messages).Error; err != nil {
		t.Fatalf("create parity fixture: %v", err)
	}
	if err := testDB.Model(&database.Conversation{}).Where("id = ?", conversationID).Updates(map[string]any{
		"summary":                  "resumo até A1",
		"summary_up_to_message_id": boundary,
	}).Error; err != nil {
		t.Fatalf("set summary: %v", err)
	}
	full, err := database.GetMessagesWithContext(ctx, conversationID, nil)
	if err != nil {
		t.Fatalf("load oracle history: %v", err)
	}

	for _, maxMessages := range []int{2, 3} {
		t.Run(fmt.Sprintf("max-%d", maxMessages), func(t *testing.T) {
			legacy := &stubRepo{messages: full, summary: "resumo até A1", sumUpTo: boundary}
			wantMessages, wantSummary, err := (&HistoryLoader{Repo: legacy, MaxMsgs: maxMessages}).LoadThroughMessage(ctx, conversationID, "retry-target")
			if err != nil {
				t.Fatalf("legacy oracle: %v", err)
			}
			gotMessages, gotSummary, err := (&HistoryLoader{Repo: NewDBMessageStore(), MaxMsgs: maxMessages}).LoadThroughMessage(ctx, conversationID, "retry-target")
			if err != nil {
				t.Fatalf("anchored store: %v", err)
			}
			if gotSummary != wantSummary || !reflect.DeepEqual(gotMessages, wantMessages) {
				t.Fatalf("loader parity mismatch:\n got messages=%+v summary=%q\nwant messages=%+v summary=%q", gotMessages, gotSummary, wantMessages, wantSummary)
			}
			if maxMessages == 3 {
				var target *Message
				for i := range gotMessages {
					if gotMessages[i].ID == "retry-target" {
						target = &gotMessages[i]
					}
				}
				if target == nil || target.Media == "" {
					t.Fatalf("target media was not preserved: %+v", gotMessages)
				}
			}
		})
	}
}

func TestHistoryLoader_AnchoredStoreContextCancellation(t *testing.T) {
	_, ctx, conversationID := setupHotPathDB(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, _, err := (&HistoryLoader{Repo: NewDBMessageStore(), MaxMsgs: 3}).LoadThroughMessage(canceled, conversationID, "missing")
	if err == nil {
		t.Fatal("expected canceled context error")
	}
}
