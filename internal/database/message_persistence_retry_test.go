package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestMessageUpdatesSurviveWriterContention(t *testing.T) {
	for _, kind := range []string{"content", "text", "partial", "final", "cache"} {
		t.Run(kind, func(t *testing.T) {
			gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(filepath.Join(t.TempDir(), "messages.db"))+"&_pragma=journal_mode(WAL)")
			defer cleanup()
			if err := gdb.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
				t.Fatal(err)
			}
			conv := Conversation{UserID: "user-1", Title: "contention"}
			if err := gdb.Create(&conv).Error; err != nil {
				t.Fatal(err)
			}
			msg := ChatMessage{ConversationID: conv.ID, Role: "assistant", Content: "original", Model: "original"}
			if err := gdb.Create(&msg).Error; err != nil {
				t.Fatal(err)
			}
			repo := NewMessageRepository(gdb)
			ctx, cancel := context.WithTimeout(WithUserID(context.Background(), "user-1"), 5*time.Second)
			defer cancel()
			pool, err := gdb.DB()
			if err != nil {
				t.Fatal(err)
			}
			writer, err := pool.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = writer.Close() }()
			if _, err := writer.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			defer func() { _, _ = writer.ExecContext(context.Background(), "ROLLBACK") }()
			busy := make(chan struct{}, 1)
			if err := gdb.Callback().Update().After("gorm:commit_or_rollback_transaction").Register("test:observe_busy", func(tx *gorm.DB) {
				if IsSQLiteBusyError(tx.Error) {
					select {
					case busy <- struct{}{}:
					default:
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			released := make(chan error, 1)
			go func() {
				select {
				case <-busy:
					_, err := writer.ExecContext(ctx, "COMMIT")
					released <- err
				case <-ctx.Done():
					released <- ctx.Err()
				}
			}()
			switch kind {
			case "content":
				err = repo.UpdateMessageContentWithContext(ctx, msg.ID, "saved", 1, 2, 3, "model")
			case "text":
				err = repo.UpdateMessageTextWithContext(ctx, msg.ID, "saved")
			case "partial":
				err = repo.UpdateMessageContentAndReasoningWithContext(ctx, msg.ID, "saved", "reasoning", 1, 2, 3, "model")
			case "final":
				err = repo.UpdateMessageContentReasoningAndUsageWithContext(ctx, msg.ID, "saved", "reasoning", 1, 2, 3, 4, 5, 6, "model")
			case "cache":
				err = repo.UpdateMessageCacheTokensWithContext(ctx, msg.ID, 4, 5, 6)
			}
			releaseErr := <-released
			if err != nil || releaseErr != nil {
				t.Fatalf("update=%v, writer release=%v", err, releaseErr)
			}
			got, err := repo.GetMessageWithContext(ctx, msg.ID)
			if err != nil {
				t.Fatal(err)
			}
			expectedContent := "saved"
			if kind == "cache" {
				expectedContent = "original"
			}
			if got.Content != expectedContent {
				t.Fatalf("content lost: %+v", got)
			}
			if kind == "partial" || kind == "final" {
				if got.Reasoning != "reasoning" || got.TotalTokens != 3 || got.Model != "model" {
					t.Fatalf("metadata lost: %+v", got)
				}
			}
			if kind == "final" || kind == "cache" {
				if got.CacheReadTokens != 4 || got.CacheWriteTokens != 5 || got.CacheMissTokens != 6 {
					t.Fatalf("cache usage lost: %+v", got)
				}
			}
			// Authorization is still checked inside every atomic update.
			if err := repo.UpdateMessageTextWithContext(WithUserID(ctx, "other-user"), msg.ID, "foreign"); err != nil {
				t.Fatal(err)
			}
			got, err = repo.GetMessageWithContext(ctx, msg.ID)
			if err != nil || got.Content != expectedContent {
				t.Fatalf("foreign update changed message: %+v, %v", got, err)
			}
			var count int64
			if err := gdb.Model(&ChatMessage{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("duplicate messages: %d, %v", count, err)
			}
		})
	}
}

func TestMessageUpdateRetryStopsOnCancellationAndPermanentError(t *testing.T) {
	for _, cancelOnBusy := range []bool{false, true} {
		t.Run(map[bool]string{false: "permanent", true: "cancelled"}[cancelOnBusy], func(t *testing.T) {
			gdb, cleanup := openSQLitePolicyTestDB(t, sqliteDSN(filepath.Join(t.TempDir(), "failure.db")))
			defer cleanup()
			ctx, cancel := context.WithCancel(WithUserID(context.Background(), "user-1"))
			defer cancel()
			permanent := errors.New("permanent write failure")
			calls := 0
			if err := gdb.Callback().Update().Before("gorm:begin_transaction").Register("test:failure", func(tx *gorm.DB) {
				calls++
				if cancelOnBusy {
					_ = tx.AddError(errors.New("database is locked (SQLITE_BUSY)"))
					cancel()
				} else {
					_ = tx.AddError(permanent)
				}
			}); err != nil {
				t.Fatal(err)
			}
			err := NewMessageRepository(gdb).UpdateMessageTextWithContext(ctx, "message", "saved")
			expected := permanent
			if cancelOnBusy {
				expected = context.Canceled
			}
			if !errors.Is(err, expected) || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}
