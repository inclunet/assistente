package tasklist

import "assistente/internal/database"

// TaskListRepository abstrai todas as operações de persistência de task lists, tasks e notas.
// Implementado por DBStore; pode ser mockado em testes.
type TaskListRepository interface {
	// ── Task List ──────────────────────────────────────────────────────────────
	CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error)
	GetTaskList(id string) (*database.TaskList, error)
	GetAllTaskLists() ([]database.TaskList, error)
	UpdateTaskList(id string, title, description string) error
	UpdateTaskListFull(id string, title, description, preferredViewMode string, slug *string) error
	ResolveTaskListRef(taskListID *string, taskListSlug string) (string, error)
	SetTaskListValidationPolicy(taskListID string, policyJSON string) error
	SetTaskListViewMode(id string, viewMode string) error
	CloneTaskList(id string, newTitle string) (*database.TaskList, error)
	ClearTaskList(id string) error
	DeleteTaskList(id string) error
	GetTaskListStats(taskListID string) (map[string]interface{}, error)
	GetTaskListWithHierarchy(id string) (*database.TaskList, error)

	// ── Workflow ───────────────────────────────────────────────────────────────
	GetWorkflow(taskListID string) (*database.TaskListWorkflow, error)
	UpdateWorkflow(taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error
	UpdateWorkflowFull(taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error
	GetTaskCountsByStatus(taskListID string) (map[int]int64, error)
	ReorderWorkflowStatuses(taskListID string, statusOrder []int) error
	ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error

	// ── Task ───────────────────────────────────────────────────────────────────
	CreateTask(taskListID string, title, description, code, link string, parentID *string) (*database.Task, error)
	CreateTaskFull(taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error)
	GetTask(id string) (*database.Task, error)
	GetTasksByTaskListID(taskListID string) ([]database.Task, error)
	GetTasksByStatus(taskListID string, statusID int) ([]database.Task, error)
	FindTaskByCode(taskListID string, code string) (*database.Task, error)
	ResolveTaskRef(taskListID *string, taskListSlug string, taskID *string, code string) (string, error)
	ResolveTaskIDByTaskCode(taskListID *string, taskCode string) (string, error)
	UpdateTask(id string, title, description, code, link string) error
	UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error
	UpdateTaskAssignee(id string, assigneeName, assigneeID string) error
	UpdateTaskStatus(id string, newStatusID int) error
	ReorderTasks(taskListID string, statusID int, orderedIDs []string) error
	PromoteTask(id string) error
	DemoteTask(id string, parentID string) error
	MoveTaskToList(taskID string, targetTaskListID string) (*database.Task, error)
	DeleteTask(id string) error
	GetSubtasks(parentID string) ([]database.Task, error)

	// ── Task Note ─────────────────────────────────────────────────────────────
	CreateTaskNote(taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error)
	UpsertTaskNoteByExternal(p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error)
	GetTaskNotes(taskID string) ([]database.TaskNote, error)
	GetTaskNote(noteID string) (*database.TaskNote, error)
	UpdateTaskNote(noteID string, content string) error
	DeleteTaskNote(noteID string) error
}
