package main

import (
	"context"

	"assistente/internal/database"
	"assistente/internal/tasklist"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Re-exporta tipos do database para compatibilidade com o frontend Wails
type TaskList = database.TaskList
type Task = database.Task
type TaskNote = database.TaskNote
type TaskListWorkflow = database.TaskListWorkflow
type TaskListWorkflowStatus = database.TaskListWorkflowStatus

// wailsEmitter adapta runtime.EventsEmit para a interface tasklist.EventEmitter.
type wailsEmitter struct {
	ctx context.Context
}

func (e *wailsEmitter) Emit(event string, data ...interface{}) {
	runtime.EventsEmit(e.ctx, event, data...)
}

// newTaskListService cria o TaskListService com o emitter Wails.
func newTaskListService(ctx context.Context) *tasklist.Service {
	return tasklist.NewService(tasklist.ServiceConfig{
		Store:   tasklist.NewDBStore(),
		Emitter: &wailsEmitter{ctx: ctx},
	})
}

// ==================== TaskList Operations ====================

func (a *App) CreateTaskList(title, description string) (*TaskList, error) {
	return a.taskSvc.CreateTaskList(a.ctx, title, description, nil)
}

func (a *App) GetTaskList(id uint) (*TaskList, error) {
	return a.taskSvc.GetTaskList(id)
}

func (a *App) GetAllTaskLists() ([]TaskList, error) {
	return a.taskSvc.GetAllTaskLists()
}

func (a *App) UpdateTaskList(id uint, title, description string) error {
	return a.taskSvc.UpdateTaskList(id, title, description)
}

func (a *App) SetTaskListViewMode(id uint, viewMode string) error {
	return a.taskSvc.SetTaskListViewMode(id, viewMode)
}

func (a *App) CloneTaskList(id uint, newTitle string) (*TaskList, error) {
	return a.taskSvc.CloneTaskList(id, newTitle)
}

func (a *App) ClearTaskList(id uint) error {
	return a.taskSvc.ClearTaskList(id)
}

func (a *App) DeleteTaskList(id uint) error {
	return a.taskSvc.DeleteTaskList(id)
}

// ==================== Workflow Operations ====================

func (a *App) GetWorkflow(taskListID uint) (*TaskListWorkflow, error) {
	return a.taskSvc.GetWorkflow(taskListID)
}

func (a *App) UpdateWorkflow(taskListID uint, statuses []TaskListWorkflowStatus, transitions map[int][]int) error {
	return a.taskSvc.UpdateWorkflow(taskListID, statuses, transitions)
}

func (a *App) UpdateWorkflowFull(taskListID uint, statuses []TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int, statusMigration map[int]int) error {
	return a.taskSvc.UpdateWorkflowFull(taskListID, statuses, database.TaskListWorkflowTransitions(transitions), initialStatusID, statusMigration)
}

func (a *App) GetTaskCountsByStatus(taskListID uint) (map[int]int64, error) {
	return a.taskSvc.GetTaskCountsByStatus(taskListID)
}

func (a *App) ReorderWorkflowStatuses(taskListID uint, statusOrder []int) error {
	return a.taskSvc.ReorderWorkflowStatuses(taskListID, statusOrder)
}

func (a *App) ValidateStatusTransition(taskListID uint, fromStatusID, toStatusID int) error {
	return a.taskSvc.ValidateStatusTransition(taskListID, fromStatusID, toStatusID)
}

// ==================== Task Operations ====================

func (a *App) CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*Task, error) {
	return a.taskSvc.CreateTask(taskListID, title, description, code, link, parentID)
}

func (a *App) GetTask(id uint) (*Task, error) {
	return a.taskSvc.GetTask(id)
}

func (a *App) GetTasksByTaskListID(taskListID uint) ([]Task, error) {
	return a.taskSvc.GetTasksByTaskListID(taskListID)
}

func (a *App) GetTasksByStatus(taskListID uint, statusID int) ([]Task, error) {
	return a.taskSvc.GetTasksByStatus(taskListID, statusID)
}

func (a *App) UpdateTask(id uint, title, description, code, link string) error {
	return a.taskSvc.UpdateTask(id, title, description, code, link)
}

func (a *App) UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return a.taskSvc.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}

func (a *App) UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error {
	return a.taskSvc.UpdateTaskAssignee(id, assigneeName, assigneeID)
}

func (a *App) UpdateTaskStatus(id uint, statusID int) error {
	return a.taskSvc.UpdateTaskStatus(id, statusID)
}

func (a *App) ReorderTasks(taskListID uint, statusID int, orderedIDs []uint) error {
	return a.taskSvc.ReorderTasks(taskListID, statusID, orderedIDs)
}

func (a *App) PromoteTask(id uint) error {
	return a.taskSvc.PromoteTask(id)
}

func (a *App) DemoteTask(id uint, parentID uint) error {
	return a.taskSvc.DemoteTask(id, parentID)
}

func (a *App) DeleteTask(id uint) error {
	return a.taskSvc.DeleteTask(id)
}

func (a *App) GetSubtasks(parentID uint) ([]Task, error) {
	return a.taskSvc.GetSubtasks(parentID)
}

// ==================== TaskNote Operations ====================

func (a *App) CreateTaskNote(taskID uint, noteType int, content, authorName, authorID string) (*TaskNote, error) {
	return a.taskSvc.CreateTaskNote(taskID, noteType, content, authorName, authorID)
}

func (a *App) GetTaskNotes(taskID uint) ([]TaskNote, error) {
	return a.taskSvc.GetTaskNotes(taskID)
}

func (a *App) UpdateTaskNote(noteID uint, content string) error {
	return a.taskSvc.UpdateTaskNote(noteID, content)
}

func (a *App) DeleteTaskNote(noteID uint) error {
	return a.taskSvc.DeleteTaskNote(noteID)
}

// ==================== Utility Operations ====================

func (a *App) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	return a.taskSvc.GetTaskListStats(taskListID)
}

func (a *App) GetTaskListWithHierarchy(id uint) (*TaskList, error) {
	return a.taskSvc.GetTaskListWithHierarchy(id)
}
