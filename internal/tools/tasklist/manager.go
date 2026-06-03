package tasklist

import (
	"context"

	"assistente/internal/database"
)

// TaskListManager abstrai as operações de gerenciamento de task lists,
// permitindo que as tools interajam sem acoplamento direto ao App.
type TaskListManager interface {
	CreateTaskList(ctx context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error)
	GetTaskList(ctx context.Context, id string) (*database.TaskList, error)
	GetAllTaskLists(ctx context.Context) ([]database.TaskList, error)
	GetTaskListStats(ctx context.Context, taskListID string) (map[string]interface{}, error)
	UpdateTaskListFull(ctx context.Context, id string, title, description, preferredViewMode string, slug *string) error
	ResolveTaskListRef(ctx context.Context, taskListID *string, taskListSlug string) (string, error)
	SetTaskListValidationPolicy(ctx context.Context, taskListID string, policyJSON string) error
	GetTaskListCustomActions(ctx context.Context, taskListID string) (*database.TaskListCustomActions, error)
	SetTaskListCustomActions(ctx context.Context, taskListID string, actionsJSON string) error
	UpdateWorkflowFull(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error
	GetTaskCountsByStatus(ctx context.Context, taskListID string) (map[int]int64, error)
	CreateTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error)
	CreateTaskFull(ctx context.Context, taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error)
	GetTask(ctx context.Context, id string) (*database.Task, error)
	FindTaskByCode(ctx context.Context, taskListID string, code string) (*database.Task, error)
	ResolveTaskRef(ctx context.Context, taskListID *string, taskListSlug string, taskID *string, code string) (string, error)
	ResolveTaskIDByTaskCode(ctx context.Context, taskListID *string, taskCode string) (string, error)
	UpdateTask(ctx context.Context, id string, title, description, code, link string) error
	UpdateTaskFull(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error
	UpdateTaskAssignee(ctx context.Context, id string, assigneeName, assigneeID string) error
	UpdateTaskStatus(ctx context.Context, id string, newStatusID int) error
	MoveTaskToList(ctx context.Context, taskID string, targetTaskListID string) (*database.Task, error)
	DeleteTask(ctx context.Context, id string) error
	GetWorkflow(ctx context.Context, taskListID string) (*database.TaskListWorkflow, error)
	CreateTaskNote(ctx context.Context, taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error)
	UpsertTaskNoteByExternal(ctx context.Context, p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error)
	UpdateTaskNote(ctx context.Context, noteID string, content string) error
	GetTaskNotes(ctx context.Context, taskID string) ([]database.TaskNote, error)
	GetTaskNote(ctx context.Context, noteID string) (*database.TaskNote, error)
}
