package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
)

// BenchmarkConversationMessageWindowBaseline mede o caminho de produção:
// consulta SQLite, hidratação do ledger, montagem da timeline e serialização do
// payload que cruza a ponte Wails.
func BenchmarkConversationMessageWindowBaseline(b *testing.B) {
	for _, messageCount := range []int{100, 500, 1000} {
		for _, resultBytes := range []int{1 << 10, 100 << 10, 10 << 20} {
			name := fmt.Sprintf("messages_%d_result_%d", messageCount, resultBytes)
			b.Run(name, func(b *testing.B) {
				setupMessageWindowAppTestDB(b)
				conversationID := seedMessageWindowBenchmark(b, messageCount, resultBytes)
				controller := newMessageWindowTestController()
				request := chat.MessageWindowRequest{
					ConversationID: conversationID,
					Scope:          chat.MessageWindowScopeConversation,
					Anchor:         chat.MessageWindowAnchorEnd,
					Direction:      chat.MessageWindowDirectionBefore,
					Limit:          database.MaxMessageWindowRows,
				}

				b.ReportMetric(float64(resultBytes), "source_result_bytes")
				b.ReportMetric(float64(messageCount), "persisted_messages")
				b.ResetTimer()
				for range b.N {
					window, err := controller.GetConversationMessageWindow(messageWindowTestCtx(), request)
					if err != nil {
						b.Fatal(err)
					}
					payload, err := json.Marshal(window)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportMetric(float64(len(payload)), "window_bytes")
				}
			})
		}
	}
}

func TestConversationMessageWindowExcluiPayloadIntegralDaTool(t *testing.T) {
	setupMessageWindowAppTestDB(t)
	conversationID := seedMessageWindowBenchmark(t, 100, 100<<10)
	window, err := newMessageWindowTestController().GetConversationMessageWindow(messageWindowTestCtx(), chat.MessageWindowRequest{
		ConversationID: conversationID,
		Scope:          chat.MessageWindowScopeConversation,
		Anchor:         chat.MessageWindowAnchorEnd,
		Direction:      chat.MessageWindowDirectionBefore,
		Limit:          database.MaxMessageWindowRows,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(window)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), strings.Repeat("x", 256)) {
		t.Fatal("timeline leve contém trecho do resultado integral")
	}
	if len(payload) >= 64<<10 {
		t.Fatalf("timeline leve excedeu 64 KiB: %d bytes", len(payload))
	}
}

func seedMessageWindowBenchmark(tb testing.TB, messageCount, resultBytes int) string {
	tb.Helper()
	conversation := createMessageWindowTestConversation(tb, "Benchmark")
	now := time.Unix(1_700_000_000, 0).UTC()
	rows := make([]database.ChatMessage, 0, messageCount)
	for index := 0; index < messageCount; index++ {
		id := fmt.Sprintf("benchmark-message-%04d", index)
		role := "user"
		content := "pergunta sintética"
		var turnID *string
		if index%2 == 1 {
			role = "assistant"
			content = "resposta sintética"
			turn := fmt.Sprintf("benchmark-message-%04d", index-1)
			turnID = &turn
		}
		rows = append(rows, database.ChatMessage{
			UUIDModel:      database.UUIDModel{ID: id, CreatedAt: now.Add(time.Duration(index) * time.Millisecond), UpdatedAt: now.Add(time.Duration(index) * time.Millisecond)},
			ConversationID: conversation.ID,
			TurnID:         turnID,
			Role:           role,
			Content:        content,
		})
	}
	if err := database.DB().CreateInBatches(rows, 100).Error; err != nil {
		tb.Fatalf("seed messages: %v", err)
	}

	var catalog database.ToolCatalog
	if err := database.DB().Where("name = ?", "search").First(&catalog).Error; err != nil {
		tb.Fatalf("load catalog: %v", err)
	}
	output, err := json.Marshal(map[string]any{
		"content":  strings.Repeat("x", resultBytes),
		"is_error": false,
	})
	if err != nil {
		tb.Fatalf("marshal output: %v", err)
	}
	completedAt := now.Add(time.Second)
	turnIndex := messageCount - 2
	if turnIndex < 0 {
		turnIndex = 0
	}
	turnID := fmt.Sprintf("benchmark-message-%04d", turnIndex)
	assistantID := fmt.Sprintf("benchmark-message-%04d", turnIndex+1)
	metadata, err := json.Marshal(map[string]any{
		"display": map[string]any{
			"version":              1,
			"name":                 "search",
			"arguments":            `{"sample":true}`,
			"iteration":            1,
			"assistant_message_id": assistantID,
		},
	})
	if err != nil {
		tb.Fatalf("marshal metadata: %v", err)
	}
	invocation := database.ToolInvocation{
		UUIDModel:      database.UUIDModel{ID: "benchmark-invocation", CreatedAt: now, UpdatedAt: completedAt},
		UserID:         messageWindowTestUserID,
		ToolCatalogID:  catalog.ID,
		OriginType:     toolinvocations.OriginChat,
		OriginID:       turnID,
		ConversationID: &conversation.ID,
		TurnID:         &turnID,
		ToolCallID:     "benchmark-call",
		Status:         toolinvocations.StatusSucceeded,
		Input:          `{"sample":true}`,
		Output:         string(output),
		Metadata:       string(metadata),
		QueuedAt:       now,
		StartedAt:      &now,
		CompletedAt:    &completedAt,
		DurationMs:     1000,
	}
	invocation.InputBytes = int64(len(invocation.Input))
	invocation.OutputBytes = int64(len(invocation.Output))
	invocation.InputPreview = fmt.Sprintf(`{"bytes":%d}`, len(invocation.Input))
	invocation.OutputPreview = fmt.Sprintf(`{"bytes":%d}`, len(invocation.Output))
	invocation.ResultAvailability = "available"
	if err := database.DB().Create(&invocation).Error; err != nil {
		tb.Fatalf("seed invocation: %v", err)
	}
	return conversation.ID
}
