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

func seedMessageWindowBenchmark(b *testing.B, messageCount, resultBytes int) string {
	b.Helper()
	conversation := createMessageWindowTestConversation(b, "Benchmark")
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
		b.Fatalf("seed messages: %v", err)
	}

	var catalog database.ToolCatalog
	if err := database.DB().Where("name = ?", "search").First(&catalog).Error; err != nil {
		b.Fatalf("load catalog: %v", err)
	}
	output, err := json.Marshal(map[string]any{
		"content":  strings.Repeat("x", resultBytes),
		"is_error": false,
	})
	if err != nil {
		b.Fatalf("marshal output: %v", err)
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
		b.Fatalf("marshal metadata: %v", err)
	}
	invocation := database.ToolInvocation{
		UUIDModel:     database.UUIDModel{ID: "benchmark-invocation", CreatedAt: now, UpdatedAt: completedAt},
		UserID:        messageWindowTestUserID,
		ToolCatalogID: catalog.ID,
		OriginType:    toolinvocations.OriginChat,
		OriginID:      turnID,
		ToolCallID:    "benchmark-call",
		Status:        toolinvocations.StatusSucceeded,
		Input:         `{"sample":true}`,
		Output:        string(output),
		Metadata:      string(metadata),
		QueuedAt:      now,
		StartedAt:     &now,
		CompletedAt:   &completedAt,
		DurationMs:    1000,
	}
	if err := database.DB().Create(&invocation).Error; err != nil {
		b.Fatalf("seed invocation: %v", err)
	}
	return conversation.ID
}
