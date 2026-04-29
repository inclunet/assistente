package controllers

import (
	"context"

	"assistente/internal/database"
	"assistente/internal/tasklist"
)

// TaskListControllerConfig agrupa as dependências do TaskListController.
type TaskListControllerConfig struct {
	TaskSvc *tasklist.Service
}

// TaskListController é o Inbound Adapter para operações de listas de tarefas.
type TaskListController struct {
	taskSvc *tasklist.Service
}

// NewTaskListController cria um TaskListController com as dependências injetadas.
func NewTaskListController(cfg TaskListControllerConfig) *TaskListController {
	return &TaskListController{taskSvc: cfg.TaskSvc}
}

// ==================== TaskList Operations ====================

func (c *TaskListController) CreateTaskList(ctx context.Context, title, description string, slug string) (*database.TaskList, error) {
	return c.taskSvc.CreateTaskList(ctx, title, description, nil, slug)
}

func (c *TaskListController) GetTaskList(id string) (*database.TaskList, error) {
	return c.taskSvc.GetTaskList(id)
}

func (c *TaskListController) GetAllTaskLists() ([]database.TaskList, error) {
	return c.taskSvc.GetAllTaskLists()
}

func (c *TaskListController) UpdateTaskList(id string, title, description string) error {
	return c.taskSvc.UpdateTaskList(id, title, description)
}

func (c *TaskListController) SetTaskListViewMode(id string, viewMode string) error {
	return c.taskSvc.SetTaskListViewMode(id, viewMode)
}

func (c *TaskListController) CloneTaskList(id string, newTitle string) (*database.TaskList, error) {
	return c.taskSvc.CloneTaskList(id, newTitle)
}

func (c *TaskListController) ClearTaskList(id string) error {
	return c.taskSvc.ClearTaskList(id)
}

func (c *TaskListController) DeleteTaskList(id string) error {
	return c.taskSvc.DeleteTaskList(id)
}

// ==================== Workflow Operations ====================

func (c *TaskListController) GetWorkflow(taskListID string) (*database.TaskListWorkflow, error) {
	return c.taskSvc.GetWorkflow(taskListID)
}

func (c *TaskListController) UpdateWorkflow(taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	return c.taskSvc.UpdateWorkflow(taskListID, statuses, transitions)
}

func (c *TaskListController) UpdateWorkflowFull(taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int, statusMigration map[int]int) error {
	return c.taskSvc.UpdateWorkflowFull(taskListID, statuses, database.TaskListWorkflowTransitions(transitions), initialStatusID, statusMigration)
}

func (c *TaskListController) GetTaskCountsByStatus(taskListID string) (map[int]int64, error) {
	return c.taskSvc.GetTaskCountsByStatus(taskListID)
}

func (c *TaskListController) ReorderWorkflowStatuses(taskListID string, statusOrder []int) error {
	return c.taskSvc.ReorderWorkflowStatuses(taskListID, statusOrder)
}

func (c *TaskListController) ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error {
	return c.taskSvc.ValidateStatusTransition(taskListID, fromStatusID, toStatusID)
}

// ==================== Task Operations ====================

func (c *TaskListController) CreateTask(taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	return c.taskSvc.CreateTask(taskListID, title, description, code, link, parentID)
}

func (c *TaskListController) GetTask(id string) (*database.Task, error) {
	return c.taskSvc.GetTask(id)
}

func (c *TaskListController) GetTasksByTaskListID(taskListID string) ([]database.Task, error) {
	return c.taskSvc.GetTasksByTaskListID(taskListID)
}

func (c *TaskListController) GetTasksByStatus(taskListID string, statusID int) ([]database.Task, error) {
	return c.taskSvc.GetTasksByStatus(taskListID, statusID)
}

func (c *TaskListController) UpdateTask(id string, title, description, code, link string) error {
	return c.taskSvc.UpdateTask(id, title, description, code, link)
}

func (c *TaskListController) UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return c.taskSvc.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}

func (c *TaskListController) UpdateTaskAssignee(id string, assigneeName, assigneeID string) error {
	return c.taskSvc.UpdateTaskAssignee(id, assigneeName, assigneeID)
}

func (c *TaskListController) UpdateTaskStatus(id string, statusID int) error {
	return c.taskSvc.UpdateTaskStatus(id, statusID)
}

func (c *TaskListController) ReorderTasks(taskListID string, statusID int, orderedIDs []string) error {
	return c.taskSvc.ReorderTasks(taskListID, statusID, orderedIDs)
}

func (c *TaskListController) PromoteTask(id string) error {
	return c.taskSvc.PromoteTask(id)
}

func (c *TaskListController) DemoteTask(id string, parentID string) error {
	return c.taskSvc.DemoteTask(id, parentID)
}

func (c *TaskListController) DeleteTask(id string) error {
	return c.taskSvc.DeleteTask(id)
}

func (c *TaskListController) GetSubtasks(parentID string) ([]database.Task, error) {
	return c.taskSvc.GetSubtasks(parentID)
}

// ==================== TaskNote Operations ====================

func (c *TaskListController) CreateTaskNote(taskID string, noteType int, content, authorName, authorID string) (*database.TaskNote, error) {
	return c.taskSvc.CreateTaskNote(taskID, noteType, content, authorName, authorID)
}

func (c *TaskListController) GetTaskNotes(taskID string) ([]database.TaskNote, error) {
	return c.taskSvc.GetTaskNotes(taskID)
}

func (c *TaskListController) UpdateTaskNote(noteID string, content string) error {
	return c.taskSvc.UpdateTaskNote(noteID, content)
}

func (c *TaskListController) DeleteTaskNote(noteID string) error {
	return c.taskSvc.DeleteTaskNote(noteID)
}

// ==================== Utility Operations ====================

func (c *TaskListController) GetTaskListStats(taskListID string) (map[string]interface{}, error) {
	return c.taskSvc.GetTaskListStats(taskListID)
}

func (c *TaskListController) GetTaskListWithHierarchy(id string) (*database.TaskList, error) {
	return c.taskSvc.GetTaskListWithHierarchy(id)
}
