package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/controllers"
	"assistente/internal/database"
	"assistente/internal/tasklist"
)

func TestTasklistActionsNotWired(t *testing.T) {
	t.Parallel()
	api := NewTasklistActions()
	if _, err := api.GetTaskListCustomActions("list"); !errors.Is(err, ErrTasklistActionsNotWired) {
		t.Fatalf("GetTaskListCustomActions: got %v", err)
	}
	if err := api.SetTaskListCustomActions("list", "{}"); !errors.Is(err, ErrTasklistActionsNotWired) {
		t.Fatalf("SetTaskListCustomActions: got %v", err)
	}
	if _, err := api.ListCardCustomActions("task", "card_menu"); !errors.Is(err, ErrTasklistActionsNotWired) {
		t.Fatalf("ListCardCustomActions: got %v", err)
	}
	if _, err := api.ListBoardCustomActions("list"); !errors.Is(err, ErrTasklistActionsNotWired) {
		t.Fatalf("ListBoardCustomActions: got %v", err)
	}
	if _, err := api.TriggerCustomAction("list", "task", "action"); !errors.Is(err, ErrTasklistActionsNotWired) {
		t.Fatalf("TriggerCustomAction: got %v", err)
	}
}

func TestTasklistActionsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "tasklist_actions.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("tasklist_actions.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("tasklist_actions.go deve chamar WithUser(session,")
	}
}

type customActionTaskMapStore struct {
	tasklist.TaskListRepository
}

func (s customActionTaskMapStore) GetTaskList(_ context.Context, id string) (*database.TaskList, error) {
	tl := &database.TaskList{Slug: "board"}
	tl.ID = id
	return tl, nil
}

func TestCustomActionTaskMapIncludesConversationID(t *testing.T) {
	t.Parallel()
	ctrl := controllers.NewTaskListController(controllers.TaskListControllerConfig{
		TaskSvc: tasklist.NewService(tasklist.ServiceConfig{
			Store: customActionTaskMapStore{},
		}),
	})

	conversationID := "conversation-uuid"
	task := &database.Task{
		TaskListID:     "list-1",
		ConversationID: &conversationID,
	}
	task.ID = "task-1"

	got := customActionTaskMap(context.Background(), ctrl, task)
	if got["conversation_id"] != conversationID {
		t.Fatalf("conversation_id = %q, want %q", got["conversation_id"], conversationID)
	}
}

func TestCustomActionTaskMapIncludesEmptyConversationIDWhenNil(t *testing.T) {
	t.Parallel()
	ctrl := controllers.NewTaskListController(controllers.TaskListControllerConfig{
		TaskSvc: tasklist.NewService(tasklist.ServiceConfig{
			Store: customActionTaskMapStore{},
		}),
	})

	task := &database.Task{TaskListID: "list-1"}
	task.ID = "task-1"

	got := customActionTaskMap(context.Background(), ctrl, task)
	if got["conversation_id"] != "" {
		t.Fatalf("conversation_id = %q, want empty string", got["conversation_id"])
	}
}

func TestEmptyTaskMapIncludesConversationID(t *testing.T) {
	t.Parallel()
	got := emptyTaskMap()
	if got["conversation_id"] != "" {
		t.Fatalf("conversation_id = %q, want empty string", got["conversation_id"])
	}
}
