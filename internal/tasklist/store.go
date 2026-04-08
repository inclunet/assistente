package tasklist

import "assistente/internal/database"

// TaskListRepository abstrai todas as operações de persistência de task lists, tasks e notas.
// Implementado por DBStore; pode ser mockado em testes.
type TaskListRepository interface {
	// ── Task List ──────────────────────────────────────────────────────────────
	CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow) (*database.TaskList, error)
	GetTaskList(id uint) (*database.TaskList, error)
	GetAllTaskLists() ([]database.TaskList, error)
	UpdateTaskList(id uint, title, description string) error
	UpdateTaskListFull(id uint, title, description, preferredViewMode string) error
	SetTaskListValidationPolicy(taskListID uint, policyJSON string) error
	SetTaskListViewMode(id uint, viewMode string) error
	CloneTaskList(id uint, newTitle string) (*database.TaskList, error)
	ClearTaskList(id uint) error
	DeleteTaskList(id uint) error
	GetTaskListStats(taskListID uint) (map[string]interface{}, error)
	GetTaskListWithHierarchy(id uint) (*database.TaskList, error)

	// ── Workflow ───────────────────────────────────────────────────────────────
	GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error)
	UpdateWorkflow(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error
	UpdateWorkflowFull(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error
	GetTaskCountsByStatus(taskListID uint) (map[int]int64, error)
	ReorderWorkflowStatuses(taskListID uint, statusOrder []int) error
	ValidateStatusTransition(taskListID uint, fromStatusID, toStatusID int) error

	// ── Task ───────────────────────────────────────────────────────────────────
	CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*database.Task, error)
	CreateTaskFull(taskListID uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *uint) (*database.Task, error)
	GetTask(id uint) (*database.Task, error)
	GetTasksByTaskListID(taskListID uint) ([]database.Task, error)
	GetTasksByStatus(taskListID uint, statusID int) ([]database.Task, error)
	FindTaskByCode(taskListID uint, code string) (*database.Task, error)
	UpdateTask(id uint, title, description, code, link string) error
	UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error
	UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error
	UpdateTaskStatus(id uint, newStatusID int) error
	ReorderTasks(taskListID uint, statusID int, orderedIDs []uint) error
	PromoteTask(id uint) error
	DemoteTask(id uint, parentID uint) error
	MoveTaskToList(taskID uint, targetTaskListID uint) (*database.Task, error)
	DeleteTask(id uint) error
	GetSubtasks(parentID uint) ([]database.Task, error)

	// ── Task Note ─────────────────────────────────────────────────────────────
	CreateTaskNote(taskID uint, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error)
	UpsertTaskNoteByExternal(p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error)
	GetTaskNotes(taskID uint) ([]database.TaskNote, error)
	GetTaskNote(noteID uint) (*database.TaskNote, error)
	UpdateTaskNote(noteID uint, content string) error
	DeleteTaskNote(noteID uint) error
}
