package app

import (
	"context"
	"testing"

	"assistente/controllers"
	"assistente/internal/database"
	"assistente/internal/tasklist"
)

type customActionTaskMapStore struct {
	tasklist.TaskListRepository
}

func (s customActionTaskMapStore) GetTaskList(_ context.Context, id string) (*database.TaskList, error) {
	tl := &database.TaskList{Slug: "board"}
	tl.ID = id
	return tl, nil
}

func TestCustomActionTaskMapIncludesConversationID(t *testing.T) {
	app := &App{
		taskListCtrl: controllers.NewTaskListController(controllers.TaskListControllerConfig{
			TaskSvc: tasklist.NewService(tasklist.ServiceConfig{
				Store: customActionTaskMapStore{},
			}),
		}),
	}

	conversationID := "conversation-uuid"
	task := &database.Task{
		TaskListID:     "list-1",
		ConversationID: &conversationID,
	}
	task.ID = "task-1"

	got := app.customActionTaskMap(context.Background(), task)
	if got["conversation_id"] != conversationID {
		t.Fatalf("conversation_id = %q, want %q", got["conversation_id"], conversationID)
	}
}

func TestCustomActionTaskMapIncludesEmptyConversationIDWhenNil(t *testing.T) {
	app := &App{
		taskListCtrl: controllers.NewTaskListController(controllers.TaskListControllerConfig{
			TaskSvc: tasklist.NewService(tasklist.ServiceConfig{
				Store: customActionTaskMapStore{},
			}),
		}),
	}

	task := &database.Task{TaskListID: "list-1"}
	task.ID = "task-1"

	got := app.customActionTaskMap(context.Background(), task)
	if got["conversation_id"] != "" {
		t.Fatalf("conversation_id = %q, want empty string", got["conversation_id"])
	}
}

func TestEmptyTaskMapIncludesConversationID(t *testing.T) {
	got := emptyTaskMap()
	if got["conversation_id"] != "" {
		t.Fatalf("conversation_id = %q, want empty string", got["conversation_id"])
	}
}
