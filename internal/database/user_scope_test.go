package database

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUserScopeTestDB(t *testing.T) {
	t.Helper()

	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&User{}, &Conversation{}, &ChatMessage{}, &LLMProvider{}, &TaskListWorkflow{}, &TaskList{}, &Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

func TestConversationQueriesCanBeScopedByUser(t *testing.T) {
	setupUserScopeTestDB(t)

	anaCtx := WithUserID(context.Background(), "user-ana")
	leoCtx := WithUserID(context.Background(), "user-leo")
	if _, err := CreateConversationWithContext(anaCtx, "Ana", ""); err != nil {
		t.Fatalf("create ana conversation: %v", err)
	}
	if _, err := CreateConversationWithContext(leoCtx, "Leo", ""); err != nil {
		t.Fatalf("create leo conversation: %v", err)
	}

	conversations, err := GetConversationsWithContext(anaCtx)
	if err != nil {
		t.Fatalf("get scoped conversations: %v", err)
	}
	if len(conversations) != 1 || conversations[0].UserID != "user-ana" {
		t.Fatalf("expected only ana conversation, got %+v", conversations)
	}

	all, err := GetConversations()
	if err != nil {
		t.Fatalf("get all conversations: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("unscoped compatibility query should return 2 conversations, got %d", len(all))
	}
}

func TestProviderAndTaskListQueriesCanBeScopedByUser(t *testing.T) {
	setupUserScopeTestDB(t)

	anaCtx := WithUserID(context.Background(), "user-ana")
	leoCtx := WithUserID(context.Background(), "user-leo")
	if err := SaveLLMProviderWithContext(anaCtx, &LLMProvider{ID: "ana-openai", Name: "OpenAI", Type: "openai", BaseURL: "https://api.openai.com/v1"}); err != nil {
		t.Fatalf("save ana provider: %v", err)
	}
	if err := SaveLLMProviderWithContext(leoCtx, &LLMProvider{ID: "leo-openai", Name: "OpenAI", Type: "openai", BaseURL: "https://api.openai.com/v1"}); err != nil {
		t.Fatalf("save leo provider: %v", err)
	}
	if _, err := CreateTaskListWithContext(anaCtx, "Ana tasks", "", nil, ""); err != nil {
		t.Fatalf("create ana task list: %v", err)
	}
	if _, err := CreateTaskListWithContext(leoCtx, "Leo tasks", "", nil, ""); err != nil {
		t.Fatalf("create leo task list: %v", err)
	}

	providers, err := GetLLMProvidersWithContext(anaCtx)
	if err != nil {
		t.Fatalf("get scoped providers: %v", err)
	}
	if len(providers) != 1 || providers[0].UserID != "user-ana" {
		t.Fatalf("expected only ana provider, got %+v", providers)
	}

	taskLists, err := GetAllTaskListsWithContext(leoCtx)
	if err != nil {
		t.Fatalf("get scoped task lists: %v", err)
	}
	if len(taskLists) != 1 || taskLists[0].UserID != "user-leo" {
		t.Fatalf("expected only leo task list, got %+v", taskLists)
	}
}
