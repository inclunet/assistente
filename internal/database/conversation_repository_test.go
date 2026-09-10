package database

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGetConversationInfoRejectsEmptyIDBeforeQuery(t *testing.T) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir banco: %v", err)
	}

	var queries atomic.Int32
	const callbackName = "test:count_empty_conversation_queries"
	if err := testDB.Callback().Query().Before("gorm:query").Register(callbackName, func(*gorm.DB) {
		queries.Add(1)
	}); err != nil {
		t.Fatalf("registrar callback: %v", err)
	}
	t.Cleanup(func() {
		_ = testDB.Callback().Query().Remove(callbackName)
	})

	repo := NewConversationRepository(testDB)
	ctx := WithUserID(context.Background(), "user-1")
	if _, err := repo.GetConversationInfoWithContext(ctx, " \t "); !errors.Is(err, ErrConversationIDRequired) {
		t.Fatalf("erro = %v, esperava ErrConversationIDRequired", err)
	}
	if got := queries.Load(); got != 0 {
		t.Fatalf("consultas SQL = %d, esperava zero", got)
	}
}

func TestGetConversationInfoKeepsUserScopePrecondition(t *testing.T) {
	repo := NewConversationRepository(&gorm.DB{})
	if _, err := repo.GetConversationInfoWithContext(context.Background(), ""); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatalf("erro = %v, esperava ErrUserScopeRequired", err)
	}
}
