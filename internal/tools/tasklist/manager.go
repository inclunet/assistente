package tasklist

import "assistente/internal/database"

// TaskListManager abstrai as operações de gerenciamento de task lists,
// permitindo que as tools interajam sem acoplamento direto ao App.
type TaskListManager interface {
	CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow) (*database.TaskList, error)
	GetTaskList(id uint) (*database.TaskList, error)
	GetAllTaskLists() ([]database.TaskList, error)
	GetTaskListStats(taskListID uint) (map[string]interface{}, error)
	UpdateTaskListFull(id uint, title, description, preferredViewMode string) error
	UpdateWorkflowFull(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error
	GetTaskCountsByStatus(taskListID uint) (map[int]int64, error)
	CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*database.Task, error)
	CreateTaskFull(taskListID uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *uint) (*database.Task, error)
	GetTask(id uint) (*database.Task, error)
	FindTaskByCode(taskListID uint, code string) (*database.Task, error)
	UpdateTask(id uint, title, description, code, link string) error
	UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error
	UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error
	UpdateTaskStatus(id uint, newStatusID int) error
	DeleteTask(id uint) error
	GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error)
	CreateTaskNote(taskID uint, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error)
	GetTaskNotes(taskID uint) ([]database.TaskNote, error)
}
