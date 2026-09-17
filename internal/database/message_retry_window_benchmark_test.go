package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func BenchmarkLoadHistoryWindowThroughMessageAgainstFullHistory(b *testing.B) {
	// A linha legada mede somente a leitura integral do repositório seguida do
	// corte até o alvo; não representa o custo posterior de summary/filter do
	// HistoryLoader. A comparação isola o custo de materialização da janela.
	for _, size := range []int{100, 500, 1000} {
		b.Run(fmt.Sprintf("messages-%d", size), func(b *testing.B) {
			db, conversationID, targetID, cleanup := newRetryWindowBenchmarkDB(b, size)
			defer cleanup()
			repo := NewMessageRepository(db)
			ctx := WithUserID(context.Background(), testUserID)

			b.Run("legacy-full-and-cut", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					messages, err := repo.GetMessagesWithContext(ctx, conversationID, nil)
					if err != nil {
						b.Fatal(err)
					}
					cut := -1
					for index, message := range messages {
						if message.ID == targetID {
							cut = index + 1
							break
						}
					}
					if cut < 0 {
						b.Fatal("target missing from legacy history")
					}
					_ = messages[:cut]
				}
			})

			b.Run("anchored-window-50", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					window, err := repo.LoadHistoryWindowThroughMessageWithContext(ctx, conversationID, targetID, 50)
					if err != nil {
						b.Fatal(err)
					}
					if len(window.Messages) > 52 {
						b.Fatalf("anchored window exceeded first-two plus max tail: %d", len(window.Messages))
					}
					foundTarget := false
					for _, message := range window.Messages {
						if message.ID == targetID {
							foundTarget = true
							break
						}
					}
					if !foundTarget {
						b.Fatal("target missing from anchored window")
					}
				}
			})
		})
	}
}

func newRetryWindowBenchmarkDB(b *testing.B, size int) (*gorm.DB, string, string, func()) {
	b.Helper()
	dsn := fmt.Sprintf("file:retry-window-benchmark-%d?mode=memory&cache=shared", size)
	benchmarkDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	if err := benchmarkDB.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		b.Fatal(err)
	}
	if err := benchmarkDB.Exec("CREATE INDEX IF NOT EXISTS idx_chat_messages_window ON chat_messages (conversation_id, parent_id, created_at, id)").Error; err != nil {
		b.Fatal(err)
	}
	conversation := &Conversation{UUIDModel: UUIDModel{ID: fmt.Sprintf("bench-conv-%d", size)}, UserID: testUserID, Title: "benchmark"}
	if err := benchmarkDB.Create(conversation).Error; err != nil {
		b.Fatal(err)
	}
	base := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	messages := make([]ChatMessage, 0, size)
	content := strings.Repeat("historical context segment ", 16)
	for i := 0; i < size; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		messages = append(messages, ChatMessage{
			UUIDModel:      UUIDModel{ID: fmt.Sprintf("bench-%04d", i), CreatedAt: base.Add(time.Duration(i) * time.Second)},
			ConversationID: conversation.ID,
			Role:           role,
			Content:        content,
			Media:          `[{"type":"image","url":"fixture"}]`,
		})
	}
	if err := benchmarkDB.CreateInBatches(messages, 100).Error; err != nil {
		b.Fatal(err)
	}
	sqlDB, err := benchmarkDB.DB()
	if err != nil {
		b.Fatal(err)
	}
	return benchmarkDB, conversation.ID, fmt.Sprintf("bench-%04d", size-2), func() { _ = sqlDB.Close() }
}
