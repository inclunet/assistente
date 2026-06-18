package app

import (
	"context"
	"encoding/json"
	"strings"

	"assistente/internal/contextprovider"
	"assistente/internal/database"
	"assistente/internal/tasklist"
)

// linkedTaskListsForConversation resolve as task lists vinculadas a uma conversa
// e as mapeia para o Context Provider tasklist.
// Best-effort: erros (ou ctx sem usuário) resultam em nil → contexto vazio.
func (a *App) linkedTaskListsForConversation(ctx context.Context, conversationID string) []contextprovider.LinkedTaskList {
	if a.taskListCtrl == nil || strings.TrimSpace(conversationID) == "" {
		return nil
	}
	lists, err := a.taskListCtrl.GetTaskListsByConversation(ctx, conversationID)
	if err != nil || len(lists) == 0 {
		return nil
	}
	out := make([]contextprovider.LinkedTaskList, 0, len(lists))
	for i := range lists {
		l := lists[i]
		statusMeta := map[int]database.TaskListWorkflowStatus{}
		if l.Workflow != nil && strings.TrimSpace(l.Workflow.Statuses) != "" {
			var sts []database.TaskListWorkflowStatus
			if json.Unmarshal([]byte(l.Workflow.Statuses), &sts) == nil {
				for _, s := range sts {
					statusMeta[s.ID] = s
				}
			}
		}
		tasks := make([]contextprovider.LinkedTask, 0, len(l.Tasks))
		for _, tk := range l.Tasks {
			meta := statusMeta[tk.StatusID]
			tasks = append(tasks, contextprovider.LinkedTask{
				ID:         tk.ID,
				Title:      tk.Title,
				Status:     meta.Label,
				StatusIcon: meta.Icon,
			})
		}
		out = append(out, contextprovider.LinkedTaskList{
			ID:          l.ID,
			Title:       l.Title,
			Description: l.Description,
			Tasks:       tasks,
		})
	}
	return out
}

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
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.CreateTaskList(ctx, title, description, slug)
}
func (a *App) GetTaskList(id string) (*TaskList, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTaskList(ctx, id)
}
func (a *App) GetAllTaskLists() ([]TaskList, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetAllTaskLists(ctx)
}
func (a *App) UpdateTaskList(id string, title, description string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.UpdateTaskList(ctx, id, title, description)
}
func (a *App) SetTaskListViewMode(id string, viewMode string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.SetTaskListViewMode(ctx, id, viewMode)
}
func (a *App) SetTaskListConversation(id string, conversationID *string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.SetTaskListConversation(ctx, id, conversationID)
}
func (a *App) GetTaskListsByConversation(conversationID string) ([]TaskList, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTaskListsByConversation(ctx, conversationID)
}
func (a *App) CloneTaskList(id string, newTitle string) (*TaskList, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.CloneTaskList(ctx, id, newTitle)
}
func (a *App) ClearTaskList(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.ClearTaskList(ctx, id)
}
func (a *App) DeleteTaskList(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.DeleteTaskList(ctx, id)
}

// ==================== Workflow Operations ====================

func (a *App) GetWorkflow(taskListID string) (*TaskListWorkflow, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetWorkflow(ctx, taskListID)
}
func (a *App) UpdateWorkflow(taskListID string, statuses []TaskListWorkflowStatus, transitions map[int][]int) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.UpdateWorkflow(ctx, taskListID, statuses, transitions)
}
func (a *App) UpdateWorkflowFull(taskListID string, statuses []TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int, statusMigration map[int]int) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.UpdateWorkflowFull(ctx, taskListID, statuses, transitions, initialStatusID, statusMigration)
}
func (a *App) GetTaskCountsByStatus(taskListID string) (map[int]int64, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTaskCountsByStatus(ctx, taskListID)
}
func (a *App) ReorderWorkflowStatuses(taskListID string, statusOrder []int) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.ReorderWorkflowStatuses(ctx, taskListID, statusOrder)
}
func (a *App) ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.ValidateStatusTransition(ctx, taskListID, fromStatusID, toStatusID)
}

// ==================== Task Operations ====================

func (a *App) CreateTask(taskListID string, title, description, code, link string, parentID *string) (*Task, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.CreateTask(ctx, taskListID, title, description, code, link, parentID)
}
func (a *App) GetTask(id string) (*Task, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTask(ctx, id)
}
func (a *App) GetTasksByTaskListID(taskListID string) ([]Task, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTasksByTaskListID(ctx, taskListID)
}
func (a *App) GetTasksByStatus(taskListID string, statusID int) ([]Task, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTasksByStatus(ctx, taskListID, statusID)
}
func (a *App) UpdateTask(id string, title, description, code, link string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.UpdateTask(ctx, id, title, description, code, link)
}
func (a *App) UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.UpdateTaskFull(ctx, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}
func (a *App) UpdateTaskAssignee(id string, assigneeName, assigneeID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.UpdateTaskAssignee(ctx, id, assigneeName, assigneeID)
}
func (a *App) SetTaskConversation(id string, conversationID *string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.SetTaskConversation(ctx, id, conversationID)
}
func (a *App) GetTasksByConversation(conversationID string) ([]Task, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTasksByConversation(ctx, conversationID)
}
func (a *App) UpdateTaskStatus(id string, statusID int) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.UpdateTaskStatus(ctx, id, statusID)
}
func (a *App) ReorderTasks(taskListID string, statusID int, orderedIDs []string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.ReorderTasks(ctx, taskListID, statusID, orderedIDs)
}
func (a *App) PromoteTask(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.PromoteTask(ctx, id)
}
func (a *App) DemoteTask(id string, parentID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.DemoteTask(ctx, id, parentID)
}
func (a *App) DeleteTask(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.DeleteTask(ctx, id)
}
func (a *App) GetSubtasks(parentID string) ([]Task, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetSubtasks(ctx, parentID)
}

// ==================== TaskNote Operations ====================

func (a *App) CreateTaskNote(taskID string, noteType int, content, authorName, authorID string) (*TaskNote, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.CreateTaskNote(ctx, taskID, noteType, content, authorName, authorID)
}
func (a *App) GetTaskNotes(taskID string) ([]TaskNote, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTaskNotes(ctx, taskID)
}
func (a *App) UpdateTaskNote(noteID string, content string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.UpdateTaskNote(ctx, noteID, content)
}
func (a *App) DeleteTaskNote(noteID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return a.taskListCtrl.DeleteTaskNote(ctx, noteID)
}

// ==================== Utility Operations ====================

func (a *App) GetTaskListStats(taskListID string) (map[string]interface{}, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTaskListStats(ctx, taskListID)
}
func (a *App) GetTaskListWithHierarchy(id string) (*TaskList, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.taskListCtrl.GetTaskListWithHierarchy(ctx, id)
}
