package main

import (
	"assistente/internal/database"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Re-exporta tipos do database para compatibilidade
type TaskList = database.TaskList
type Task = database.Task
type TaskNote = database.TaskNote
type TaskListWorkflow = database.TaskListWorkflow
type TaskListWorkflowStatus = database.TaskListWorkflowStatus

// ==================== TaskList Operations ====================

// CreateTaskList cria uma nova lista de tarefas
func (a *App) CreateTaskList(title, description string) (*TaskList, error) {
	taskList, err := database.CreateTaskList(title, description, nil)
	if err != nil {
		return nil, err
	}

	// Recarrega a lista completa (com Workflow e Tasks)
	fullTaskList, err := database.GetTaskList(taskList.ID)
	if err != nil {
		// Se falhar ao recarregar, retorna o que temos (com Workflow)
		if taskList.Tasks == nil {
			taskList.Tasks = []Task{}
		}
		fullTaskList = taskList
	}

	// Emite evento para frontend
	runtime.EventsEmit(a.ctx, "taskList:created", fullTaskList)
	return fullTaskList, nil
}

// GetTaskList retorna uma lista de tarefas com seu workflow e tasks
func (a *App) GetTaskList(id uint) (*TaskList, error) {
	return database.GetTaskList(id)
}

// GetAllTaskLists retorna todas as listas de tarefas do usuário
func (a *App) GetAllTaskLists() ([]TaskList, error) {
	return database.GetAllTaskLists()
}

// UpdateTaskList atualiza title e description de uma lista
func (a *App) UpdateTaskList(id uint, title, description string) error {
	err := database.UpdateTaskList(id, title, description)
	if err != nil {
		return err
	}

	// Busca e emite evento completo
	taskList, _ := database.GetTaskList(id)
	runtime.EventsEmit(a.ctx, "taskList:updated", taskList)
	return nil
}

// SetTaskListViewMode define o modo de visualização (list ou kanban)
func (a *App) SetTaskListViewMode(id uint, viewMode string) error {
	err := database.SetTaskListViewMode(id, viewMode)
	if err != nil {
		return err
	}

	taskList, _ := database.GetTaskList(id)
	runtime.EventsEmit(a.ctx, "taskList:updated", taskList)
	return nil
}

// CloneTaskList clona uma lista com seu workflow
func (a *App) CloneTaskList(id uint, newTitle string) (*TaskList, error) {
	taskList, err := database.CloneTaskList(id, newTitle)
	if err != nil {
		return nil, err
	}

	// Recarrega a lista completa (com Workflow e Tasks)
	fullTaskList, err := database.GetTaskList(taskList.ID)
	if err != nil {
		// Se falhar ao recarregar, retorna o que temos
		if taskList.Tasks == nil {
			taskList.Tasks = []Task{}
		}
		fullTaskList = taskList
	}

	runtime.EventsEmit(a.ctx, "taskList:created", fullTaskList)
	return fullTaskList, nil
}

// ClearTaskList remove todas as tasks de uma lista, mantendo a lista e o workflow
func (a *App) ClearTaskList(id uint) error {
	err := database.ClearTaskList(id)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "taskList:cleared", id)
	return nil
}

// DeleteTaskList deleta uma lista e todas suas tasks
func (a *App) DeleteTaskList(id uint) error {
	err := database.DeleteTaskList(id)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "taskList:deleted", id)
	return nil
}

// ==================== Workflow Operations ====================

// GetWorkflow retorna o workflow de uma lista
func (a *App) GetWorkflow(taskListID uint) (*TaskListWorkflow, error) {
	return database.GetWorkflow(taskListID)
}

// UpdateWorkflow atualiza statuses e transitions de um workflow
func (a *App) UpdateWorkflow(taskListID uint, statuses []TaskListWorkflowStatus, transitions map[int][]int) error {
	err := database.UpdateWorkflow(taskListID, statuses, transitions)
	if err != nil {
		return err
	}

	workflow, _ := database.GetWorkflow(taskListID)
	runtime.EventsEmit(a.ctx, "workflow:updated", workflow)
	return nil
}

// UpdateWorkflowFull atualiza workflow completo (statuses, transitions, initial_status_id)
// com validação e migração segura de tasks
func (a *App) UpdateWorkflowFull(taskListID uint, statuses []TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int, statusMigration map[int]int) error {
	err := database.UpdateWorkflowFull(taskListID, statuses, database.TaskListWorkflowTransitions(transitions), initialStatusID, statusMigration)
	if err != nil {
		return err
	}

	taskList, _ := database.GetTaskList(taskListID)
	runtime.EventsEmit(a.ctx, "workflow:updated", taskList.Workflow)
	runtime.EventsEmit(a.ctx, "taskList:updated", taskList)
	return nil
}

// GetTaskCountsByStatus retorna contagem de tasks por status_id
func (a *App) GetTaskCountsByStatus(taskListID uint) (map[int]int64, error) {
	return database.GetTaskCountsByStatus(taskListID)
}

// ReorderWorkflowStatuses reordena os statuses de um workflow
func (a *App) ReorderWorkflowStatuses(taskListID uint, statusOrder []int) error {
	err := database.ReorderWorkflowStatuses(taskListID, statusOrder)
	if err != nil {
		return err
	}

	workflow, _ := database.GetWorkflow(taskListID)
	runtime.EventsEmit(a.ctx, "workflow:updated", workflow)
	return nil
}

// ValidateStatusTransition valida se uma transição de status é permitida
func (a *App) ValidateStatusTransition(taskListID uint, fromStatusID, toStatusID int) error {
	return database.ValidateStatusTransition(taskListID, fromStatusID, toStatusID)
}

// ==================== Task Operations ====================

// CreateTask cria uma nova tarefa
func (a *App) CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*Task, error) {
	task, err := database.CreateTask(taskListID, title, description, code, link, parentID)
	if err != nil {
		return nil, err
	}

	runtime.EventsEmit(a.ctx, "task:created", task)
	return task, nil
}

// GetTask retorna uma tarefa com suas subtasks
func (a *App) GetTask(id uint) (*Task, error) {
	return database.GetTask(id)
}

// GetTasksByTaskListID retorna todas as tasks de uma lista
func (a *App) GetTasksByTaskListID(taskListID uint) ([]Task, error) {
	return database.GetTasksByTaskListID(taskListID)
}

// GetTasksByStatus retorna tasks de um status específico
func (a *App) GetTasksByStatus(taskListID uint, statusID int) ([]Task, error) {
	return database.GetTasksByStatus(taskListID, statusID)
}

// UpdateTask atualiza title, description, code e link de uma task
func (a *App) UpdateTask(id uint, title, description, code, link string) error {
	err := database.UpdateTask(id, title, description, code, link)
	if err != nil {
		return err
	}

	task, _ := database.GetTask(id)
	runtime.EventsEmit(a.ctx, "task:updated", task)
	return nil
}

// UpdateTaskFull atualiza todos os campos editáveis de uma task, incluindo assignee e creator
func (a *App) UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	err := database.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
	if err != nil {
		return err
	}

	task, _ := database.GetTask(id)
	runtime.EventsEmit(a.ctx, "task:updated", task)
	return nil
}

// UpdateTaskAssignee atualiza apenas o assignee de uma task
func (a *App) UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error {
	oldTask, _ := database.GetTask(id)
	err := database.UpdateTaskAssignee(id, assigneeName, assigneeID)
	if err != nil {
		return err
	}

	if oldTask != nil && oldTask.AssigneeName != assigneeName {
		var content string
		switch {
		case oldTask.AssigneeName == "" && assigneeName != "":
			content = "Responsável definido: " + assigneeName
		case oldTask.AssigneeName != "" && assigneeName == "":
			content = "Responsável removido (era " + oldTask.AssigneeName + ")"
		default:
			content = "Responsável alterado de " + oldTask.AssigneeName + " para " + assigneeName
		}
		note, _ := database.CreateTaskNote(id, database.TaskNoteSystem, content, "system", "")
		if note != nil {
			runtime.EventsEmit(a.ctx, "taskNote:created", note)
		}
	}

	task, _ := database.GetTask(id)
	runtime.EventsEmit(a.ctx, "task:updated", task)
	return nil
}

// UpdateTaskStatus atualiza o status de uma tarefa com validação
func (a *App) UpdateTaskStatus(id uint, statusID int) error {
	err := database.UpdateTaskStatus(id, statusID)
	if err != nil {
		return err
	}

	task, _ := database.GetTask(id)
	runtime.EventsEmit(a.ctx, "task:updated", task)
	return nil
}

// ReorderTasks reordena tasks dentro de um status
func (a *App) ReorderTasks(taskListID uint, statusID int, orderedIDs []uint) error {
	err := database.ReorderTasks(taskListID, statusID, orderedIDs)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "taskList:updated", taskListID)
	return nil
}

// PromoteTask move uma subtask para task principal
func (a *App) PromoteTask(id uint) error {
	err := database.PromoteTask(id)
	if err != nil {
		return err
	}

	task, _ := database.GetTask(id)
	runtime.EventsEmit(a.ctx, "task:updated", task)
	return nil
}

// DemoteTask move uma task para ser subtask de outra
func (a *App) DemoteTask(id uint, parentID uint) error {
	err := database.DemoteTask(id, parentID)
	if err != nil {
		return err
	}

	task, _ := database.GetTask(id)
	runtime.EventsEmit(a.ctx, "task:updated", task)
	return nil
}

// DeleteTask deleta uma tarefa e suas subtasks
func (a *App) DeleteTask(id uint) error {
	err := database.DeleteTask(id)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "task:deleted", id)
	return nil
}

// GetSubtasks retorna subtasks de uma task
func (a *App) GetSubtasks(parentID uint) ([]Task, error) {
	return database.GetSubtasks(parentID)
}

// ==================== TaskNote Operations ====================

// CreateTaskNote cria uma nova nota/interação para uma task
func (a *App) CreateTaskNote(taskID uint, noteType int, content, authorName, authorID string) (*TaskNote, error) {
	note, err := database.CreateTaskNote(taskID, database.TaskNoteType(noteType), content, authorName, authorID)
	if err != nil {
		return nil, err
	}

	runtime.EventsEmit(a.ctx, "taskNote:created", note)
	return note, nil
}

// GetTaskNotes retorna todas as notas de uma task
func (a *App) GetTaskNotes(taskID uint) ([]TaskNote, error) {
	return database.GetTaskNotes(taskID)
}

// UpdateTaskNote atualiza o conteúdo de uma nota
func (a *App) UpdateTaskNote(noteID uint, content string) error {
	err := database.UpdateTaskNote(noteID, content)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "taskNote:updated", noteID)
	return nil
}

// DeleteTaskNote remove uma nota
func (a *App) DeleteTaskNote(noteID uint) error {
	err := database.DeleteTaskNote(noteID)
	if err != nil {
		return err
	}

	runtime.EventsEmit(a.ctx, "taskNote:deleted", noteID)
	return nil
}

// ==================== Utility Operations ====================

// GetTaskListStats retorna estatísticas de uma lista
func (a *App) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	return database.GetTaskListStats(taskListID)
}

// GetTaskListWithHierarchy retorna lista com hierarquia completa
func (a *App) GetTaskListWithHierarchy(id uint) (*TaskList, error) {
	return database.GetTaskListWithHierarchy(id)
}

