package tasklist

import (
	"context"

	"assistente/internal/database"
)

// TaskListRepository abstrai todas as operações de persistência de task lists, tasks e notas.
// Implementado por DBStore; pode ser mockado em testes.
type TaskListRepository interface {
	// ── Task List ──────────────────────────────────────────────────────────────
	CreateTaskList(ctx context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error)
	GetTaskList(ctx context.Context, id string) (*database.TaskList, error)
	GetAllTaskLists(ctx context.Context) ([]database.TaskList, error)
	UpdateTaskList(ctx context.Context, id string, title, description string) error
	UpdateTaskListFull(ctx context.Context, id string, title, description, preferredViewMode string, slug *string) error
	ResolveTaskListRef(ctx context.Context, taskListID *string, taskListSlug string) (string, error)
	SetTaskListValidationPolicy(ctx context.Context, taskListID string, policyJSON string) error
	SetTaskListViewMode(ctx context.Context, id string, viewMode string) error
	CloneTaskList(ctx context.Context, id string, newTitle string) (*database.TaskList, error)
	ClearTaskList(ctx context.Context, id string) error
	DeleteTaskList(ctx context.Context, id string) error
	GetTaskListStats(ctx context.Context, taskListID string) (map[string]interface{}, error)
	GetTaskListWithHierarchy(ctx context.Context, id string) (*database.TaskList, error)

	// ── Workflow ───────────────────────────────────────────────────────────────
	GetWorkflow(ctx context.Context, taskListID string) (*database.TaskListWorkflow, error)
	UpdateWorkflow(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error
	UpdateWorkflowFull(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error
	GetTaskCountsByStatus(ctx context.Context, taskListID string) (map[int]int64, error)
	ReorderWorkflowStatuses(ctx context.Context, taskListID string, statusOrder []int) error
	ValidateStatusTransition(ctx context.Context, taskListID string, fromStatusID, toStatusID int) error

	// ── Task ───────────────────────────────────────────────────────────────────
	CreateTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error)
	CreateTaskFull(ctx context.Context, taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error)
	GetTask(ctx context.Context, id string) (*database.Task, error)
	GetTasksByTaskListID(ctx context.Context, taskListID string) ([]database.Task, error)
	GetTasksByStatus(ctx context.Context, taskListID string, statusID int) ([]database.Task, error)
	FindTaskByCode(ctx context.Context, taskListID string, code string) (*database.Task, error)
	ResolveTaskRef(ctx context.Context, taskListID *string, taskListSlug string, taskID *string, code string) (string, error)
	ResolveTaskIDByTaskCode(ctx context.Context, taskListID *string, taskCode string) (string, error)
	UpdateTask(ctx context.Context, id string, title, description, code, link string) error
	UpdateTaskFull(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error
	UpdateTaskAssignee(ctx context.Context, id string, assigneeName, assigneeID string) error
	UpdateTaskStatus(ctx context.Context, id string, newStatusID int) error
	ReorderTasks(ctx context.Context, taskListID string, statusID int, orderedIDs []string) error
	PromoteTask(ctx context.Context, id string) error
	DemoteTask(ctx context.Context, id string, parentID string) error
	MoveTaskToList(ctx context.Context, taskID string, targetTaskListID string) (*database.Task, error)
	DeleteTask(ctx context.Context, id string) error
	GetSubtasks(ctx context.Context, parentID string) ([]database.Task, error)

	// ── Task Note ─────────────────────────────────────────────────────────────
	CreateTaskNote(ctx context.Context, taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error)
	UpsertTaskNoteByExternal(ctx context.Context, p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error)
	GetTaskNotes(ctx context.Context, taskID string) ([]database.TaskNote, error)
	GetTaskNote(ctx context.Context, noteID string) (*database.TaskNote, error)
	UpdateTaskNote(ctx context.Context, noteID string, content string) error
	DeleteTaskNote(ctx context.Context, noteID string) error
}
