package main

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
	return a.taskListCtrl.CreateTaskList(a.ctx, title, description, slug)
}
func (a *App) GetTaskList(id uint) (*TaskList, error) { return a.taskListCtrl.GetTaskList(id) }
func (a *App) GetAllTaskLists() ([]TaskList, error)   { return a.taskListCtrl.GetAllTaskLists() }
func (a *App) UpdateTaskList(id uint, title, description string) error {
	return a.taskListCtrl.UpdateTaskList(id, title, description)
}
func (a *App) SetTaskListViewMode(id uint, viewMode string) error {
	return a.taskListCtrl.SetTaskListViewMode(id, viewMode)
}
func (a *App) CloneTaskList(id uint, newTitle string) (*TaskList, error) {
	return a.taskListCtrl.CloneTaskList(id, newTitle)
}
func (a *App) ClearTaskList(id uint) error  { return a.taskListCtrl.ClearTaskList(id) }
func (a *App) DeleteTaskList(id uint) error { return a.taskListCtrl.DeleteTaskList(id) }

// ==================== Workflow Operations ====================

func (a *App) GetWorkflow(taskListID uint) (*TaskListWorkflow, error) {
	return a.taskListCtrl.GetWorkflow(taskListID)
}
func (a *App) UpdateWorkflow(taskListID uint, statuses []TaskListWorkflowStatus, transitions map[int][]int) error {
	return a.taskListCtrl.UpdateWorkflow(taskListID, statuses, transitions)
}
func (a *App) UpdateWorkflowFull(taskListID uint, statuses []TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int, statusMigration map[int]int) error {
	return a.taskListCtrl.UpdateWorkflowFull(taskListID, statuses, transitions, initialStatusID, statusMigration)
}
func (a *App) GetTaskCountsByStatus(taskListID uint) (map[int]int64, error) {
	return a.taskListCtrl.GetTaskCountsByStatus(taskListID)
}
func (a *App) ReorderWorkflowStatuses(taskListID uint, statusOrder []int) error {
	return a.taskListCtrl.ReorderWorkflowStatuses(taskListID, statusOrder)
}
func (a *App) ValidateStatusTransition(taskListID uint, fromStatusID, toStatusID int) error {
	return a.taskListCtrl.ValidateStatusTransition(taskListID, fromStatusID, toStatusID)
}

// ==================== Task Operations ====================

func (a *App) CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*Task, error) {
	return a.taskListCtrl.CreateTask(taskListID, title, description, code, link, parentID)
}
func (a *App) GetTask(id uint) (*Task, error) { return a.taskListCtrl.GetTask(id) }
func (a *App) GetTasksByTaskListID(taskListID uint) ([]Task, error) {
	return a.taskListCtrl.GetTasksByTaskListID(taskListID)
}
func (a *App) GetTasksByStatus(taskListID uint, statusID int) ([]Task, error) {
	return a.taskListCtrl.GetTasksByStatus(taskListID, statusID)
}
func (a *App) UpdateTask(id uint, title, description, code, link string) error {
	return a.taskListCtrl.UpdateTask(id, title, description, code, link)
}
func (a *App) UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return a.taskListCtrl.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}
func (a *App) UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error {
	return a.taskListCtrl.UpdateTaskAssignee(id, assigneeName, assigneeID)
}
func (a *App) UpdateTaskStatus(id uint, statusID int) error {
	return a.taskListCtrl.UpdateTaskStatus(id, statusID)
}
func (a *App) ReorderTasks(taskListID uint, statusID int, orderedIDs []uint) error {
	return a.taskListCtrl.ReorderTasks(taskListID, statusID, orderedIDs)
}
func (a *App) PromoteTask(id uint) error { return a.taskListCtrl.PromoteTask(id) }
func (a *App) DemoteTask(id uint, parentID uint) error {
	return a.taskListCtrl.DemoteTask(id, parentID)
}
func (a *App) DeleteTask(id uint) error { return a.taskListCtrl.DeleteTask(id) }
func (a *App) GetSubtasks(parentID uint) ([]Task, error) {
	return a.taskListCtrl.GetSubtasks(parentID)
}

// ==================== TaskNote Operations ====================

func (a *App) CreateTaskNote(taskID uint, noteType int, content, authorName, authorID string) (*TaskNote, error) {
	return a.taskListCtrl.CreateTaskNote(taskID, noteType, content, authorName, authorID)
}
func (a *App) GetTaskNotes(taskID uint) ([]TaskNote, error) {
	return a.taskListCtrl.GetTaskNotes(taskID)
}
func (a *App) UpdateTaskNote(noteID uint, content string) error {
	return a.taskListCtrl.UpdateTaskNote(noteID, content)
}
func (a *App) DeleteTaskNote(noteID uint) error { return a.taskListCtrl.DeleteTaskNote(noteID) }

// ==================== Utility Operations ====================

func (a *App) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	return a.taskListCtrl.GetTaskListStats(taskListID)
}
func (a *App) GetTaskListWithHierarchy(id uint) (*TaskList, error) {
	return a.taskListCtrl.GetTaskListWithHierarchy(id)
}
