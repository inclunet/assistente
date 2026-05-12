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

func (c *TaskListController) GetTaskList(ctx context.Context, id string) (*database.TaskList, error) {
	return c.taskSvc.GetTaskList(ctx, id)
}

func (c *TaskListController) GetAllTaskLists(ctx context.Context) ([]database.TaskList, error) {
	return c.taskSvc.GetAllTaskLists(ctx)
}

func (c *TaskListController) UpdateTaskList(ctx context.Context, id string, title, description string) error {
	return c.taskSvc.UpdateTaskList(ctx, id, title, description)
}

func (c *TaskListController) SetTaskListViewMode(ctx context.Context, id string, viewMode string) error {
	return c.taskSvc.SetTaskListViewMode(ctx, id, viewMode)
}

func (c *TaskListController) CloneTaskList(ctx context.Context, id string, newTitle string) (*database.TaskList, error) {
	return c.taskSvc.CloneTaskList(ctx, id, newTitle)
}

func (c *TaskListController) ClearTaskList(ctx context.Context, id string) error {
	return c.taskSvc.ClearTaskList(ctx, id)
}

func (c *TaskListController) DeleteTaskList(ctx context.Context, id string) error {
	return c.taskSvc.DeleteTaskList(ctx, id)
}

// ==================== Workflow Operations ====================

func (c *TaskListController) GetWorkflow(ctx context.Context, taskListID string) (*database.TaskListWorkflow, error) {
	return c.taskSvc.GetWorkflow(ctx, taskListID)
}

func (c *TaskListController) UpdateWorkflow(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	return c.taskSvc.UpdateWorkflow(ctx, taskListID, statuses, transitions)
}

func (c *TaskListController) UpdateWorkflowFull(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int, statusMigration map[int]int) error {
	return c.taskSvc.UpdateWorkflowFull(ctx, taskListID, statuses, database.TaskListWorkflowTransitions(transitions), initialStatusID, statusMigration)
}

func (c *TaskListController) GetTaskCountsByStatus(ctx context.Context, taskListID string) (map[int]int64, error) {
	return c.taskSvc.GetTaskCountsByStatus(ctx, taskListID)
}

func (c *TaskListController) ReorderWorkflowStatuses(ctx context.Context, taskListID string, statusOrder []int) error {
	return c.taskSvc.ReorderWorkflowStatuses(ctx, taskListID, statusOrder)
}

func (c *TaskListController) ValidateStatusTransition(ctx context.Context, taskListID string, fromStatusID, toStatusID int) error {
	return c.taskSvc.ValidateStatusTransition(ctx, taskListID, fromStatusID, toStatusID)
}

// ==================== Task Operations ====================

func (c *TaskListController) CreateTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	return c.taskSvc.CreateTask(ctx, taskListID, title, description, code, link, parentID)
}

func (c *TaskListController) GetTask(ctx context.Context, id string) (*database.Task, error) {
	return c.taskSvc.GetTask(ctx, id)
}

func (c *TaskListController) GetTasksByTaskListID(ctx context.Context, taskListID string) ([]database.Task, error) {
	return c.taskSvc.GetTasksByTaskListID(ctx, taskListID)
}

func (c *TaskListController) GetTasksByStatus(ctx context.Context, taskListID string, statusID int) ([]database.Task, error) {
	return c.taskSvc.GetTasksByStatus(ctx, taskListID, statusID)
}

func (c *TaskListController) UpdateTask(ctx context.Context, id string, title, description, code, link string) error {
	return c.taskSvc.UpdateTask(ctx, id, title, description, code, link)
}

func (c *TaskListController) UpdateTaskFull(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return c.taskSvc.UpdateTaskFull(ctx, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}

func (c *TaskListController) UpdateTaskAssignee(ctx context.Context, id string, assigneeName, assigneeID string) error {
	return c.taskSvc.UpdateTaskAssignee(ctx, id, assigneeName, assigneeID)
}

func (c *TaskListController) UpdateTaskStatus(ctx context.Context, id string, statusID int) error {
	return c.taskSvc.UpdateTaskStatus(ctx, id, statusID)
}

func (c *TaskListController) ReorderTasks(ctx context.Context, taskListID string, statusID int, orderedIDs []string) error {
	return c.taskSvc.ReorderTasks(ctx, taskListID, statusID, orderedIDs)
}

func (c *TaskListController) PromoteTask(ctx context.Context, id string) error {
	return c.taskSvc.PromoteTask(ctx, id)
}

func (c *TaskListController) DemoteTask(ctx context.Context, id string, parentID string) error {
	return c.taskSvc.DemoteTask(ctx, id, parentID)
}

func (c *TaskListController) DeleteTask(ctx context.Context, id string) error {
	return c.taskSvc.DeleteTask(ctx, id)
}

func (c *TaskListController) GetSubtasks(ctx context.Context, parentID string) ([]database.Task, error) {
	return c.taskSvc.GetSubtasks(ctx, parentID)
}

// ==================== TaskNote Operations ====================

func (c *TaskListController) CreateTaskNote(ctx context.Context, taskID string, noteType int, content, authorName, authorID string) (*database.TaskNote, error) {
	return c.taskSvc.CreateTaskNote(ctx, taskID, noteType, content, authorName, authorID)
}

func (c *TaskListController) GetTaskNotes(ctx context.Context, taskID string) ([]database.TaskNote, error) {
	return c.taskSvc.GetTaskNotes(ctx, taskID)
}

func (c *TaskListController) UpdateTaskNote(ctx context.Context, noteID string, content string) error {
	return c.taskSvc.UpdateTaskNote(ctx, noteID, content)
}

func (c *TaskListController) DeleteTaskNote(ctx context.Context, noteID string) error {
	return c.taskSvc.DeleteTaskNote(ctx, noteID)
}

// ==================== Utility Operations ====================

func (c *TaskListController) GetTaskListStats(ctx context.Context, taskListID string) (map[string]interface{}, error) {
	return c.taskSvc.GetTaskListStats(ctx, taskListID)
}

func (c *TaskListController) GetTaskListWithHierarchy(ctx context.Context, id string) (*database.TaskList, error) {
	return c.taskSvc.GetTaskListWithHierarchy(ctx, id)
}
