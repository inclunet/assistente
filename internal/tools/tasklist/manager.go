package tasklist

import "assistente/internal/database"

// TaskListManager abstrai as operações de gerenciamento de task lists,
// permitindo que as tools interajam sem acoplamento direto ao App.
type TaskListManager interface {
	CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow) (*database.TaskList, error)
	GetTaskList(id uint) (*database.TaskList, error)
	GetAllTaskLists() ([]database.TaskList, error)
	GetTaskListStats(taskListID uint) (map[string]interface{}, error)
	CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*database.Task, error)
	GetTask(id uint) (*database.Task, error)
	UpdateTask(id uint, title, description, code, link string) error
	UpdateTaskStatus(id uint, newStatusID int) error
	DeleteTask(id uint) error
	GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error)
	CreateTaskNote(taskID uint, noteType database.TaskNoteType, content, author string) (*database.TaskNote, error)
	GetTaskNotes(taskID uint) ([]database.TaskNote, error)
}
