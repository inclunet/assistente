package app

import (
	"assistente/internal/database"
	"assistente/internal/tasklist"
)

// Re-exporta tipos do database para compatibilidade com o frontend Wails
type TaskList = database.TaskList
type Task = database.Task
type TaskNote = database.TaskNote
type TaskListWorkflow = database.TaskListWorkflow
type TaskListWorkflowStatus = database.TaskListWorkflowStatus

// newTaskListService cria o TaskListService com o emitter injetado.
func (a *App) newTaskListService() *tasklist.Service {
	return tasklist.NewService(tasklist.ServiceConfig{
		Store:   tasklist.NewDBStore(),
		Emitter: a.emitter,
	})
}

// ==================== TaskList Operations ====================

func (a *App) CreateTaskList(title, description, slug string) (*TaskList, error) {
	return a.taskListCtrl.CreateTaskList(a.authenticatedContext(), title, description, slug)
}
func (a *App) GetTaskList(id string) (*TaskList, error) {
	return a.taskListCtrl.GetTaskList(a.authenticatedContext(), id)
}
func (a *App) GetAllTaskLists() ([]TaskList, error) {
	return a.taskListCtrl.GetAllTaskLists(a.authenticatedContext())
}
func (a *App) UpdateTaskList(id string, title, description string) error {
	return a.taskListCtrl.UpdateTaskList(a.authenticatedContext(), id, title, description)
}
func (a *App) SetTaskListViewMode(id string, viewMode string) error {
	return a.taskListCtrl.SetTaskListViewMode(a.authenticatedContext(), id, viewMode)
}
func (a *App) CloneTaskList(id string, newTitle string) (*TaskList, error) {
	return a.taskListCtrl.CloneTaskList(a.authenticatedContext(), id, newTitle)
}
func (a *App) ClearTaskList(id string) error {
	return a.taskListCtrl.ClearTaskList(a.authenticatedContext(), id)
}
func (a *App) DeleteTaskList(id string) error {
	return a.taskListCtrl.DeleteTaskList(a.authenticatedContext(), id)
}

// ==================== Workflow Operations ====================

func (a *App) GetWorkflow(taskListID string) (*TaskListWorkflow, error) {
	return a.taskListCtrl.GetWorkflow(a.authenticatedContext(), taskListID)
}
func (a *App) UpdateWorkflow(taskListID string, statuses []TaskListWorkflowStatus, transitions map[int][]int) error {
	return a.taskListCtrl.UpdateWorkflow(a.authenticatedContext(), taskListID, statuses, transitions)
}
func (a *App) UpdateWorkflowFull(taskListID string, statuses []TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int, statusMigration map[int]int) error {
	return a.taskListCtrl.UpdateWorkflowFull(a.authenticatedContext(), taskListID, statuses, transitions, initialStatusID, statusMigration)
}
func (a *App) GetTaskCountsByStatus(taskListID string) (map[int]int64, error) {
	return a.taskListCtrl.GetTaskCountsByStatus(a.authenticatedContext(), taskListID)
}
func (a *App) ReorderWorkflowStatuses(taskListID string, statusOrder []int) error {
	return a.taskListCtrl.ReorderWorkflowStatuses(a.authenticatedContext(), taskListID, statusOrder)
}
func (a *App) ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error {
	return a.taskListCtrl.ValidateStatusTransition(a.authenticatedContext(), taskListID, fromStatusID, toStatusID)
}

// ==================== Task Operations ====================

func (a *App) CreateTask(taskListID string, title, description, code, link string, parentID *string) (*Task, error) {
	return a.taskListCtrl.CreateTask(a.authenticatedContext(), taskListID, title, description, code, link, parentID)
}
func (a *App) GetTask(id string) (*Task, error) {
	return a.taskListCtrl.GetTask(a.authenticatedContext(), id)
}
func (a *App) GetTasksByTaskListID(taskListID string) ([]Task, error) {
	return a.taskListCtrl.GetTasksByTaskListID(a.authenticatedContext(), taskListID)
}
func (a *App) GetTasksByStatus(taskListID string, statusID int) ([]Task, error) {
	return a.taskListCtrl.GetTasksByStatus(a.authenticatedContext(), taskListID, statusID)
}
func (a *App) UpdateTask(id string, title, description, code, link string) error {
	return a.taskListCtrl.UpdateTask(a.authenticatedContext(), id, title, description, code, link)
}
func (a *App) UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return a.taskListCtrl.UpdateTaskFull(a.authenticatedContext(), id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}
func (a *App) UpdateTaskAssignee(id string, assigneeName, assigneeID string) error {
	return a.taskListCtrl.UpdateTaskAssignee(a.authenticatedContext(), id, assigneeName, assigneeID)
}
func (a *App) UpdateTaskStatus(id string, statusID int) error {
	return a.taskListCtrl.UpdateTaskStatus(a.authenticatedContext(), id, statusID)
}
func (a *App) ReorderTasks(taskListID string, statusID int, orderedIDs []string) error {
	return a.taskListCtrl.ReorderTasks(a.authenticatedContext(), taskListID, statusID, orderedIDs)
}
func (a *App) PromoteTask(id string) error {
	return a.taskListCtrl.PromoteTask(a.authenticatedContext(), id)
}
func (a *App) DemoteTask(id string, parentID string) error {
	return a.taskListCtrl.DemoteTask(a.authenticatedContext(), id, parentID)
}
func (a *App) DeleteTask(id string) error {
	return a.taskListCtrl.DeleteTask(a.authenticatedContext(), id)
}
func (a *App) GetSubtasks(parentID string) ([]Task, error) {
	return a.taskListCtrl.GetSubtasks(a.authenticatedContext(), parentID)
}

// ==================== TaskNote Operations ====================

func (a *App) CreateTaskNote(taskID string, noteType int, content, authorName, authorID string) (*TaskNote, error) {
	return a.taskListCtrl.CreateTaskNote(a.authenticatedContext(), taskID, noteType, content, authorName, authorID)
}
func (a *App) GetTaskNotes(taskID string) ([]TaskNote, error) {
	return a.taskListCtrl.GetTaskNotes(a.authenticatedContext(), taskID)
}
func (a *App) UpdateTaskNote(noteID string, content string) error {
	return a.taskListCtrl.UpdateTaskNote(a.authenticatedContext(), noteID, content)
}
func (a *App) DeleteTaskNote(noteID string) error {
	return a.taskListCtrl.DeleteTaskNote(a.authenticatedContext(), noteID)
}

// ==================== Utility Operations ====================

func (a *App) GetTaskListStats(taskListID string) (map[string]interface{}, error) {
	return a.taskListCtrl.GetTaskListStats(a.authenticatedContext(), taskListID)
}
func (a *App) GetTaskListWithHierarchy(id string) (*TaskList, error) {
	return a.taskListCtrl.GetTaskListWithHierarchy(a.authenticatedContext(), id)
}
