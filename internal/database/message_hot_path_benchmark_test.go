package database

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func BenchmarkHistoryWindowSQLite(b *testing.B) {
	benchmarkDB, err := gorm.Open(sqlite.Open("file:history-window-bench?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	if err := benchmarkDB.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		b.Fatal(err)
	}
	conv := &Conversation{UserID: testUserID, Title: "benchmark"}
	if err := benchmarkDB.Create(conv).Error; err != nil {
		b.Fatal(err)
	}
	messages := make([]ChatMessage, 1_000)
	for index := range messages {
		role := "assistant"
		if index%2 == 0 {
			role = "user"
		}
		messages[index] = ChatMessage{
			ConversationID: conv.ID,
			Role:           role,
			Content:        fmt.Sprintf("mensagem-%04d", index),
		}
	}
	if err := benchmarkDB.CreateInBatches(messages, 100).Error; err != nil {
		b.Fatal(err)
	}
	repo := NewMessageRepository(benchmarkDB)
	ctx := testCtx()

	b.Run("conversa-inteira-legada", func(b *testing.B) {
		for b.Loop() {
			loaded, err := repo.GetMessagesWithContext(ctx, conv.ID, nil)
			if err != nil {
				b.Fatal(err)
			}
			if len(loaded) != 1_000 {
				b.Fatalf("esperava 1000 mensagens, obteve %d", len(loaded))
			}
		}
	})
	b.Run("janela-canonica-50", func(b *testing.B) {
		for b.Loop() {
			window, err := repo.LoadHistoryWindowWithContext(ctx, conv.ID, 50)
			if err != nil {
				b.Fatal(err)
			}
			if len(window.Messages) > 52 {
				b.Fatalf("janela excedeu limite: %d", len(window.Messages))
			}
		}
	})
}
