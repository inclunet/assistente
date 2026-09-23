package database

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestConversationContentSnapshotStableScopedAndEmptyRevisionRejected(t *testing.T) {
	setupTestDB(t)
	id := createTestConversation(t, "snapshot")
	createTestMessage(t, id, "user", "private text")
	first, err := ConversationContentSnapshotWithContext(testCtx(), id)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ConversationContentSnapshotWithContext(testCtx(), id)
	if err != nil || first != second || len(first) != 64 {
		t.Fatalf("unstable digest %q %q: %v", first, second, err)
	}
	if _, err := ConversationContentSnapshotWithContext(context.Background(), id); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatalf("scope error=%v", err)
	}
	err = WithConversationLifecycle(testCtx(), func() error { return ClearConversationContentIfUnchangedWithinLifecycleWithContext(testCtx(), id, "") })
	if !errors.Is(err, ErrConversationContentChanged) {
		t.Fatalf("empty revision accepted: %v", err)
	}
	third, err := ConversationContentSnapshotWithContext(testCtx(), id)
	if err != nil || third != first {
		t.Fatalf("failed clear changed content: %v", err)
	}
}

func TestConversationContentCompareWaitsForWriterAndRejectsNewMessage(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open(sqliteDSN(filepath.Join(t.TempDir(), "snapshot.sqlite"))), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := conn.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		t.Fatal(err)
	}
	conversation := Conversation{UserID: testUserID, Title: "Writer"}
	if err := conn.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewMessageRepository(conn)
	ctx, cancel := context.WithTimeout(testCtx(), 5*time.Second)
	defer cancel()
	expected, err := repo.ConversationContentSnapshotWithContext(ctx, conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	writerHeld := make(chan struct{})
	allowCommit := make(chan struct{})
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- WithSQLiteImmediateTransaction(ctx, conn, "test.writer", func(tx *gorm.DB) error {
			if err := tx.Create(&ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "arrived after confirmation"}).Error; err != nil {
				return err
			}
			close(writerHeld)
			select {
			case <-allowCommit:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-writerHeld:
	case err := <-writerDone:
		t.Fatalf("writer: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	clearDone := make(chan error, 1)
	go func() {
		clearDone <- WithConversationLifecycle(ctx, func() error {
			return repo.ClearConversationContentIfUnchangedWithinLifecycleWithContext(ctx, conversation.ID, expected)
		})
	}()
	close(allowCommit)
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	if err := <-clearDone; !errors.Is(err, ErrConversationContentChanged) {
		t.Fatalf("clear error=%v", err)
	}
	var count int64
	if err := conn.Model(&ChatMessage{}).Where("conversation_id = ?", conversation.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("new message removed: count=%d error=%v", count, err)
	}
}
