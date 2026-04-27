package tasklist

import "assistente/internal/database"

// TaskListManager abstrai as operações de gerenciamento de task lists,
// permitindo que as tools interajam sem acoplamento direto ao App.
type TaskListManager interface {
	CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error)
	GetTaskList(id string) (*database.TaskList, error)
	GetAllTaskLists() ([]database.TaskList, error)
	GetTaskListStats(taskListID string) (map[string]interface{}, error)
	UpdateTaskListFull(id string, title, description, preferredViewMode string, slug *string) error
	ResolveTaskListRef(taskListID *string, taskListSlug string) (string, error)
	SetTaskListValidationPolicy(taskListID string, policyJSON string) error
	UpdateWorkflowFull(taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error
	GetTaskCountsByStatus(taskListID string) (map[int]int64, error)
	CreateTask(taskListID string, title, description, code, link string, parentID *string) (*database.Task, error)
	CreateTaskFull(taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error)
	GetTask(id string) (*database.Task, error)
	FindTaskByCode(taskListID string, code string) (*database.Task, error)
	ResolveTaskRef(taskListID *string, taskListSlug string, taskID *string, code string) (string, error)
	ResolveTaskIDByTaskCode(taskListID *string, taskCode string) (string, error)
	UpdateTask(id string, title, description, code, link string) error
	UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error
	UpdateTaskAssignee(id string, assigneeName, assigneeID string) error
	UpdateTaskStatus(id string, newStatusID int) error
	MoveTaskToList(taskID string, targetTaskListID string) (*database.Task, error)
	DeleteTask(id string) error
	GetWorkflow(taskListID string) (*database.TaskListWorkflow, error)
	CreateTaskNote(taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error)
	UpsertTaskNoteByExternal(p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error)
	UpdateTaskNote(noteID string, content string) error
	GetTaskNotes(taskID string) ([]database.TaskNote, error)
	GetTaskNote(noteID string) (*database.TaskNote, error)
}
