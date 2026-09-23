package chat

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestAnchoredRetryLongHistoryMatchesLegacy(t *testing.T) {
	for _, size := range []int{100, 500, 1000} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			testDB, ctx, conversationID := setupHotPathDB(t)
			messages := make([]Message, size)
			for i := range messages {
				role := "assistant"
				if i%3 == 0 {
					role = "user"
				}
				messages[i] = Message{
					UUIDModel:      database.UUIDModel{ID: fmt.Sprintf("parity-%04d", i), CreatedAt: time.Date(2026, 1, 1, 0, 0, i/2, 0, time.UTC)},
					ConversationID: conversationID, Role: role, Content: fmt.Sprint(i),
				}
			}
			// Um retry no meio da conversa exercita tanto o prefixo omitido
			// quanto a exclusão de mensagens posteriores; os timestamps empatam.
			target := (size / 2 / 3) * 3
			messages[target].Media = `[{"type":"image","url":"fixture"}]`
			if err := testDB.CreateInBatches(messages, 100).Error; err != nil {
				t.Fatal(err)
			}
			full, err := database.GetMessagesWithContext(ctx, conversationID, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, boundary := range []string{"", "missing", messages[8].ID, messages[target].ID, messages[target+1].ID} {
				if err := testDB.Model(&database.Conversation{}).Where("id = ?", conversationID).Updates(map[string]any{
					"summary": "resumo", "summary_up_to_message_id": boundary,
				}).Error; err != nil {
					t.Fatal(err)
				}
				for _, limit := range []int{2, 3, 10, 50} {
					// filter pode reutilizar o slice recebido; cada oracle precisa
					// de uma cópia para não contaminar os casos seguintes.
					legacy := &HistoryLoader{Repo: &stubRepo{messages: slices.Clone(full), summary: "resumo", sumUpTo: boundary}, MaxMsgs: limit}
					want, wantSummary, err := legacy.LoadThroughMessage(ctx, conversationID, messages[target].ID)
					if err != nil {
						t.Fatal(err)
					}
					got, gotSummary, err := (&HistoryLoader{Repo: NewDBMessageStore(), MaxMsgs: limit}).LoadThroughMessage(ctx, conversationID, messages[target].ID)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) || gotSummary != wantSummary {
						t.Fatalf("size=%d boundary=%q limit=%d: got=%v/%q want=%v/%q", size, boundary, limit, got, gotSummary, want, wantSummary)
					}
				}
			}
		})
	}
}
