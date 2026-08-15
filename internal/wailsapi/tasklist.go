package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/database"
	"context"
	"sync"
)

// Tasklist é o bind Wails do domínio tasklist CRUD (AEP-0088).
// Auth só via WithUser — sem chamar o helper de auth do App no call site.
// Custom actions ficam em TasklistActions (domínio separado).
type Tasklist struct {
	mu      sync.RWMutex
	session Session
	ctrl    *controllers.TaskListController
}

// NewTasklist cria o bind vazio; AttachTasklist preenche deps no startup.
func NewTasklist() *Tasklist {
	return &Tasklist{}
}

// AttachTasklist associa Session e controller após o startup montar as deps.
// Função de pacote (não método) para não entrar no Bind do Wails.
func AttachTasklist(api *Tasklist, session Session, ctrl *controllers.TaskListController) {
	if api == nil {
		return
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	api.session = session
	api.ctrl = ctrl
}

func (api *Tasklist) deps() (Session, *controllers.TaskListController, error) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	if api.session == nil || api.ctrl == nil {
		return nil, nil, ErrTasklistNotWired
	}
	return api.session, api.ctrl, nil
}

// ==================== TaskList Operations ====================

// CreateTaskList cria uma lista de tarefas.
func (api *Tasklist) CreateTaskList(title, description, slug string) (*database.TaskList, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.TaskList, error) {
		return ctrl.CreateTaskList(ctx, title, description, slug)
	})
}

// GetTaskList retorna uma lista pelo id.
func (api *Tasklist) GetTaskList(id string) (*database.TaskList, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.TaskList, error) {
		return ctrl.GetTaskList(ctx, id)
	})
}

// GetAllTaskLists lista todas as listas do usuário.
func (api *Tasklist) GetAllTaskLists() ([]database.TaskList, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.TaskList, error) {
		return ctrl.GetAllTaskLists(ctx)
	})
}

// UpdateTaskList atualiza título/descrição da lista.
func (api *Tasklist) UpdateTaskList(id string, title, description string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateTaskList(ctx, id, title, description)
	})
	return err
}

// SetTaskListViewMode altera o modo de visualização da lista.
func (api *Tasklist) SetTaskListViewMode(id string, viewMode string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetTaskListViewMode(ctx, id, viewMode)
	})
	return err
}

// SetTaskListConversation vincula (ou desvincula) a lista a uma conversa.
func (api *Tasklist) SetTaskListConversation(id string, conversationID *string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetTaskListConversation(ctx, id, conversationID)
	})
	return err
}

// GetTaskListsByConversation lista as listas vinculadas a uma conversa.
func (api *Tasklist) GetTaskListsByConversation(conversationID string) ([]database.TaskList, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.TaskList, error) {
		return ctrl.GetTaskListsByConversation(ctx, conversationID)
	})
}

// CloneTaskList clona uma lista com novo título.
func (api *Tasklist) CloneTaskList(id string, newTitle string) (*database.TaskList, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.TaskList, error) {
		return ctrl.CloneTaskList(ctx, id, newTitle)
	})
}

// ClearTaskList remove todas as tarefas da lista.
func (api *Tasklist) ClearTaskList(id string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ClearTaskList(ctx, id)
	})
	return err
}

// DeleteTaskList remove a lista.
func (api *Tasklist) DeleteTaskList(id string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteTaskList(ctx, id)
	})
	return err
}

// ==================== Workflow Operations ====================

// GetWorkflow retorna o workflow da lista.
func (api *Tasklist) GetWorkflow(taskListID string) (*database.TaskListWorkflow, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.TaskListWorkflow, error) {
		return ctrl.GetWorkflow(ctx, taskListID)
	})
}

// UpdateWorkflow atualiza statuses e transições do workflow.
func (api *Tasklist) UpdateWorkflow(taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateWorkflow(ctx, taskListID, statuses, transitions)
	})
	return err
}

// UpdateWorkflowFull atualiza workflow com status inicial e migração.
func (api *Tasklist) UpdateWorkflowFull(taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int, initialStatusID int, statusMigration map[int]int) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateWorkflowFull(ctx, taskListID, statuses, transitions, initialStatusID, statusMigration)
	})
	return err
}

// GetTaskCountsByStatus retorna contagem de tarefas por status.
func (api *Tasklist) GetTaskCountsByStatus(taskListID string) (map[int]int64, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[int]int64, error) {
		return ctrl.GetTaskCountsByStatus(ctx, taskListID)
	})
}

// ReorderWorkflowStatuses reordena os statuses do workflow.
func (api *Tasklist) ReorderWorkflowStatuses(taskListID string, statusOrder []int) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ReorderWorkflowStatuses(ctx, taskListID, statusOrder)
	})
	return err
}

// ValidateStatusTransition valida se a transição de status é permitida.
func (api *Tasklist) ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ValidateStatusTransition(ctx, taskListID, fromStatusID, toStatusID)
	})
	return err
}

// ==================== Task Operations ====================

// CreateTask cria uma tarefa na lista.
func (api *Tasklist) CreateTask(taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.Task, error) {
		return ctrl.CreateTask(ctx, taskListID, title, description, code, link, parentID)
	})
}

// GetTask retorna uma tarefa pelo id.
func (api *Tasklist) GetTask(id string) (*database.Task, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.Task, error) {
		return ctrl.GetTask(ctx, id)
	})
}

// GetTasksByTaskListID lista tarefas de uma lista.
func (api *Tasklist) GetTasksByTaskListID(taskListID string) ([]database.Task, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.Task, error) {
		return ctrl.GetTasksByTaskListID(ctx, taskListID)
	})
}

// GetTasksByStatus lista tarefas de um status na lista.
func (api *Tasklist) GetTasksByStatus(taskListID string, statusID int) ([]database.Task, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.Task, error) {
		return ctrl.GetTasksByStatus(ctx, taskListID, statusID)
	})
}

// UpdateTask atualiza campos básicos da tarefa.
func (api *Tasklist) UpdateTask(id string, title, description, code, link string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateTask(ctx, id, title, description, code, link)
	})
	return err
}

// UpdateTaskFull atualiza a tarefa incluindo assignee/creator.
func (api *Tasklist) UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateTaskFull(ctx, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
	})
	return err
}

// UpdateTaskAssignee atualiza o responsável da tarefa.
func (api *Tasklist) UpdateTaskAssignee(id string, assigneeName, assigneeID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateTaskAssignee(ctx, id, assigneeName, assigneeID)
	})
	return err
}

// SetTaskConversation vincula (ou desvincula) a tarefa a uma conversa.
func (api *Tasklist) SetTaskConversation(id string, conversationID *string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.SetTaskConversation(ctx, id, conversationID)
	})
	return err
}

// GetTasksByConversation lista tarefas vinculadas a uma conversa.
func (api *Tasklist) GetTasksByConversation(conversationID string) ([]database.Task, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.Task, error) {
		return ctrl.GetTasksByConversation(ctx, conversationID)
	})
}

// UpdateTaskStatus altera o status da tarefa.
func (api *Tasklist) UpdateTaskStatus(id string, statusID int) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateTaskStatus(ctx, id, statusID)
	})
	return err
}

// ReorderTasks reordena tarefas dentro de um status.
func (api *Tasklist) ReorderTasks(taskListID string, statusID int, orderedIDs []string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.ReorderTasks(ctx, taskListID, statusID, orderedIDs)
	})
	return err
}

// PromoteTask promove uma subtarefa a tarefa raiz.
func (api *Tasklist) PromoteTask(id string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.PromoteTask(ctx, id)
	})
	return err
}

// DemoteTask transforma a tarefa em subtarefa de parentID.
func (api *Tasklist) DemoteTask(id string, parentID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DemoteTask(ctx, id, parentID)
	})
	return err
}

// DeleteTask remove a tarefa.
func (api *Tasklist) DeleteTask(id string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteTask(ctx, id)
	})
	return err
}

// GetSubtasks lista subtarefas de um parent.
func (api *Tasklist) GetSubtasks(parentID string) ([]database.Task, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.Task, error) {
		return ctrl.GetSubtasks(ctx, parentID)
	})
}

// ==================== TaskNote Operations ====================

// CreateTaskNote cria uma nota em uma tarefa.
func (api *Tasklist) CreateTaskNote(taskID string, noteType int, content, authorName, authorID string) (*database.TaskNote, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.TaskNote, error) {
		return ctrl.CreateTaskNote(ctx, taskID, noteType, content, authorName, authorID)
	})
}

// GetTaskNotes lista notas de uma tarefa.
func (api *Tasklist) GetTaskNotes(taskID string) ([]database.TaskNote, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) ([]database.TaskNote, error) {
		return ctrl.GetTaskNotes(ctx, taskID)
	})
}

// UpdateTaskNote atualiza o conteúdo de uma nota.
func (api *Tasklist) UpdateTaskNote(noteID string, content string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.UpdateTaskNote(ctx, noteID, content)
	})
	return err
}

// DeleteTaskNote remove uma nota.
func (api *Tasklist) DeleteTaskNote(noteID string) error {
	session, ctrl, err := api.deps()
	if err != nil {
		return err
	}
	_, err = WithUser(session, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, ctrl.DeleteTaskNote(ctx, noteID)
	})
	return err
}

// ==================== Utility Operations ====================

// GetTaskListStats retorna estatísticas da lista.
func (api *Tasklist) GetTaskListStats(taskListID string) (map[string]interface{}, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (map[string]interface{}, error) {
		return ctrl.GetTaskListStats(ctx, taskListID)
	})
}

// GetTaskListWithHierarchy retorna a lista com hierarquia de tarefas.
func (api *Tasklist) GetTaskListWithHierarchy(id string) (*database.TaskList, error) {
	session, ctrl, err := api.deps()
	if err != nil {
		return nil, err
	}
	return WithUser(session, func(ctx context.Context) (*database.TaskList, error) {
		return ctrl.GetTaskListWithHierarchy(ctx, id)
	})
}
