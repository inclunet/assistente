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
	if err := db.AutoMigrate(&User{}, &Conversation{}, &ChatMessage{}, &LLMProvider{}, &TaskListWorkflow{}, &TaskList{}, &Task{}, &TaskNote{}); err != nil {
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

func TestTaskListRepositoryEnforcesUserScopeAcrossTasksAndNotes(t *testing.T) {
	setupUserScopeTestDB(t)

	anaCtx := WithUserID(context.Background(), "user-ana")
	leoCtx := WithUserID(context.Background(), "user-leo")

	anaList, err := CreateTaskListWithContext(anaCtx, "Ana tasks", "", nil, "ana")
	if err != nil {
		t.Fatalf("create ana list: %v", err)
	}
	leoList, err := CreateTaskListWithContext(leoCtx, "Leo tasks", "", nil, "leo")
	if err != nil {
		t.Fatalf("create leo list: %v", err)
	}
	anaTask, err := CreateTaskWithContext(anaCtx, anaList.ID, "Ana task", "", "ANA-1", "", nil)
	if err != nil {
		t.Fatalf("create ana task: %v", err)
	}
	leoTask, err := CreateTaskWithContext(leoCtx, leoList.ID, "Leo task", "", "LEO-1", "", nil)
	if err != nil {
		t.Fatalf("create leo task: %v", err)
	}
	leoNote, err := CreateTaskNoteWithContext(leoCtx, leoTask.ID, TaskNoteInternal, "secret", "", "")
	if err != nil {
		t.Fatalf("create leo note: %v", err)
	}

	if _, err := GetTaskListWithContext(anaCtx, leoList.ID); err == nil {
		t.Fatal("ana should not read leo task list")
	}
	if _, err := GetTaskWithContext(anaCtx, leoTask.ID); err == nil {
		t.Fatal("ana should not read leo task")
	}
	if _, err := GetTaskNoteWithContext(anaCtx, leoNote.ID); err == nil {
		t.Fatal("ana should not read leo task note")
	}
	if _, err := CreateTaskWithContext(anaCtx, leoList.ID, "cross", "", "", "", nil); err == nil {
		t.Fatal("ana should not create a task in leo task list")
	}
	if err := DemoteTaskWithContext(anaCtx, anaTask.ID, leoTask.ID); err == nil {
		t.Fatal("ana should not attach a task to leo parent")
	}
	if err := UpdateTaskNoteWithContext(anaCtx, leoNote.ID, "tampered"); err != nil {
		t.Fatalf("cross-user note update should be a no-op, not a SQL failure: %v", err)
	}

	got, err := GetTaskNoteWithContext(leoCtx, leoNote.ID)
	if err != nil {
		t.Fatalf("leo should still read note: %v", err)
	}
	if got.Content != "secret" {
		t.Fatalf("leo note was modified across user boundary: %q", got.Content)
	}
}

func TestMessageRepositoryEnforcesUserScopeByMessageID(t *testing.T) {
	setupUserScopeTestDB(t)

	anaCtx := WithUserID(context.Background(), "user-ana")
	leoCtx := WithUserID(context.Background(), "user-leo")

	anaConv, err := CreateConversationWithContext(anaCtx, "Ana", "")
	if err != nil {
		t.Fatalf("create ana conversation: %v", err)
	}
	leoConv, err := CreateConversationWithContext(leoCtx, "Leo", "")
	if err != nil {
		t.Fatalf("create leo conversation: %v", err)
	}
	anaMsg, err := CreateMessageWithContext(anaCtx, MessageOptions{
		ConversationID: anaConv.ID,
		Role:           "user",
		Content:        "ana",
	})
	if err != nil {
		t.Fatalf("create ana message: %v", err)
	}
	leoMsg, err := CreateMessageWithContext(leoCtx, MessageOptions{
		ConversationID: leoConv.ID,
		Role:           "user",
		Content:        "leo",
		TotalTokens:    10,
	})
	if err != nil {
		t.Fatalf("create leo message: %v", err)
	}

	if _, err := GetMessageWithContext(anaCtx, leoMsg.ID); err == nil {
		t.Fatal("ana should not read leo message by id")
	}
	if err := UpdateMessageContentWithContext(anaCtx, leoMsg.ID, "tampered", 0, 0, 0, ""); err != nil {
		t.Fatalf("cross-user update should be a no-op, not a SQL failure: %v", err)
	}
	if err := DeleteMessageWithContext(anaCtx, leoMsg.ID); err != nil {
		t.Fatalf("cross-user delete should be a no-op, not a SQL failure: %v", err)
	}

	got, err := GetMessageWithContext(leoCtx, leoMsg.ID)
	if err != nil {
		t.Fatalf("leo should still read own message: %v", err)
	}
	if got.Content != "leo" {
		t.Fatalf("leo message was modified across user boundary: %q", got.Content)
	}
	counts, err := CountChildrenWithContext(anaCtx, []string{anaMsg.ID, leoMsg.ID})
	if err != nil {
		t.Fatalf("count children: %v", err)
	}
	if _, ok := counts[leoMsg.ID]; ok {
		t.Fatalf("ana should not receive child counts for leo message: %#v", counts)
	}

	stats, err := GetAllTokenStatsWithContext(anaCtx)
	if err != nil {
		t.Fatalf("ana token stats: %v", err)
	}
	if stats["total_tokens"] != 0 {
		t.Fatalf("ana token stats leaked leo tokens: %#v", stats)
	}
	stats, err = GetAllTokenStatsWithContext(leoCtx)
	if err != nil {
		t.Fatalf("leo token stats: %v", err)
	}
	if stats["total_tokens"] != 10 {
		t.Fatalf("leo token stats = %#v, want own 10 tokens", stats)
	}
}
