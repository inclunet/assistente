package tasklist

import (
	"assistente/internal/database"
	"context"
)

// EventEmitter abstrai o envio de eventos para o frontend (Wails runtime).
// Permite testar o Service sem dependência de Wails.
type EventEmitter interface {
	Emit(event string, data any)
}

// ServiceConfig agrupa as dependências necessárias para criar um Service.
type ServiceConfig struct {
	Store   TaskListRepository
	Emitter EventEmitter
}

// Service encapsula toda a lógica de negócio de task lists.
// Chama TaskListRepository para persistência e EventEmitter para notificações.
type Service struct {
	store   TaskListRepository
	emitter EventEmitter
}

// NewService cria um Service com as dependências fornecidas.
func NewService(cfg ServiceConfig) *Service {
	return &Service{store: cfg.Store, emitter: cfg.Emitter}
}

// ── Task List ──────────────────────────────────────────────────────────────────

// CreateTaskList cria uma nova lista e recarrega o registro completo.
func (s *Service) CreateTaskList(ctx context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	tl, err := s.store.CreateTaskList(title, description, templateWorkflow, slug)
	if err != nil {
		return nil, err
	}
	full, err := s.store.GetTaskList(tl.ID)
	if err != nil {
		if tl.Tasks == nil {
			tl.Tasks = []database.Task{}
		}
		full = tl
	}
	s.emitter.Emit("taskList:created", full)
	return full, nil
}

func (s *Service) GetTaskList(id string) (*database.TaskList, error) {
	return s.store.GetTaskList(id)
}

func (s *Service) GetAllTaskLists() ([]database.TaskList, error) {
	return s.store.GetAllTaskLists()
}

func (s *Service) UpdateTaskList(id string, title, description string) error {
	if err := s.store.UpdateTaskList(id, title, description); err != nil {
		return err
	}
	tl, _ := s.store.GetTaskList(id)
	s.emitter.Emit("taskList:updated", tl)
	return nil
}

func (s *Service) UpdateTaskListFull(id string, title, description, preferredViewMode string, slug *string) error {
	return s.store.UpdateTaskListFull(id, title, description, preferredViewMode, slug)
}

func (s *Service) ResolveTaskListRef(taskListID *string, taskListSlug string) (string, error) {
	return s.store.ResolveTaskListRef(taskListID, taskListSlug)
}

func (s *Service) SetTaskListValidationPolicy(taskListID string, policyJSON string) error {
	return s.store.SetTaskListValidationPolicy(taskListID, policyJSON)
}

func (s *Service) SetTaskListViewMode(id string, viewMode string) error {
	if err := s.store.SetTaskListViewMode(id, viewMode); err != nil {
		return err
	}
	tl, _ := s.store.GetTaskList(id)
	s.emitter.Emit("taskList:updated", tl)
	return nil
}

// CloneTaskList clona uma lista e recarrega o registro completo.
func (s *Service) CloneTaskList(id string, newTitle string) (*database.TaskList, error) {
	tl, err := s.store.CloneTaskList(id, newTitle)
	if err != nil {
		return nil, err
	}
	full, err := s.store.GetTaskList(tl.ID)
	if err != nil {
		if tl.Tasks == nil {
			tl.Tasks = []database.Task{}
		}
		full = tl
	}
	s.emitter.Emit("taskList:created", full)
	return full, nil
}

func (s *Service) ClearTaskList(id string) error {
	if err := s.store.ClearTaskList(id); err != nil {
		return err
	}
	s.emitter.Emit("taskList:cleared", id)
	return nil
}

func (s *Service) DeleteTaskList(id string) error {
	if err := s.store.DeleteTaskList(id); err != nil {
		return err
	}
	s.emitter.Emit("taskList:deleted", id)
	return nil
}

func (s *Service) GetTaskListStats(taskListID string) (map[string]interface{}, error) {
	return s.store.GetTaskListStats(taskListID)
}

func (s *Service) GetTaskListWithHierarchy(id string) (*database.TaskList, error) {
	return s.store.GetTaskListWithHierarchy(id)
}

// ── Workflow ───────────────────────────────────────────────────────────────────

func (s *Service) GetWorkflow(taskListID string) (*database.TaskListWorkflow, error) {
	return s.store.GetWorkflow(taskListID)
}

func (s *Service) UpdateWorkflow(taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	if err := s.store.UpdateWorkflow(taskListID, statuses, transitions); err != nil {
		return err
	}
	wf, _ := s.store.GetWorkflow(taskListID)
	s.emitter.Emit("workflow:updated", wf)
	return nil
}

func (s *Service) UpdateWorkflowFull(taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	if err := s.store.UpdateWorkflowFull(taskListID, statuses, transitions, initialStatusID, statusMigration); err != nil {
		return err
	}
	tl, _ := s.store.GetTaskList(taskListID)
	if tl != nil && tl.Workflow != nil {
		s.emitter.Emit("workflow:updated", tl.Workflow)
	}
	s.emitter.Emit("taskList:updated", tl)
	return nil
}

func (s *Service) GetTaskCountsByStatus(taskListID string) (map[int]int64, error) {
	return s.store.GetTaskCountsByStatus(taskListID)
}

func (s *Service) ReorderWorkflowStatuses(taskListID string, statusOrder []int) error {
	if err := s.store.ReorderWorkflowStatuses(taskListID, statusOrder); err != nil {
		return err
	}
	wf, _ := s.store.GetWorkflow(taskListID)
	s.emitter.Emit("workflow:updated", wf)
	return nil
}

func (s *Service) ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error {
	return s.store.ValidateStatusTransition(taskListID, fromStatusID, toStatusID)
}

// ── Task ───────────────────────────────────────────────────────────────────────

func (s *Service) CreateTask(taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	task, err := s.store.CreateTask(taskListID, title, description, code, link, parentID)
	if err != nil {
		return nil, err
	}
	s.emitter.Emit("task:created", task)
	return task, nil
}

func (s *Service) CreateTaskFull(taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error) {
	task, err := s.store.CreateTaskFull(taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
	if err != nil {
		return nil, err
	}
	s.emitter.Emit("task:created", task)
	return task, nil
}

func (s *Service) GetTask(id string) (*database.Task, error) {
	return s.store.GetTask(id)
}

func (s *Service) GetTasksByTaskListID(taskListID string) ([]database.Task, error) {
	return s.store.GetTasksByTaskListID(taskListID)
}

func (s *Service) GetTasksByStatus(taskListID string, statusID int) ([]database.Task, error) {
	return s.store.GetTasksByStatus(taskListID, statusID)
}

func (s *Service) FindTaskByCode(taskListID string, code string) (*database.Task, error) {
	return s.store.FindTaskByCode(taskListID, code)
}

func (s *Service) ResolveTaskRef(taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	return s.store.ResolveTaskRef(taskListID, taskListSlug, taskID, code)
}

func (s *Service) ResolveTaskIDByTaskCode(taskListID *string, taskCode string) (string, error) {
	return s.store.ResolveTaskIDByTaskCode(taskListID, taskCode)
}

func (s *Service) UpdateTask(id string, title, description, code, link string) error {
	if err := s.store.UpdateTask(id, title, description, code, link); err != nil {
		return err
	}
	task, _ := s.store.GetTask(id)
	s.emitter.Emit("task:updated", task)
	return nil
}

func (s *Service) UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	if err := s.store.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID); err != nil {
		return err
	}
	task, _ := s.store.GetTask(id)
	s.emitter.Emit("task:updated", task)
	return nil
}

// UpdateTaskAssignee atualiza o responsável da task e cria nota de auditoria se houve mudança.
func (s *Service) UpdateTaskAssignee(id string, assigneeName, assigneeID string) error {
	oldTask, _ := s.store.GetTask(id)
	if err := s.store.UpdateTaskAssignee(id, assigneeName, assigneeID); err != nil {
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
		note, _ := s.store.CreateTaskNote(id, database.TaskNoteSystem, content, "system", "")
		if note != nil {
			s.emitter.Emit("taskNote:created", note)
		}
	}

	task, _ := s.store.GetTask(id)
	s.emitter.Emit("task:updated", task)
	return nil
}

func (s *Service) UpdateTaskStatus(id string, statusID int) error {
	if err := s.store.UpdateTaskStatus(id, statusID); err != nil {
		return err
	}
	task, _ := s.store.GetTask(id)
	s.emitter.Emit("task:updated", task)
	return nil
}

func (s *Service) ReorderTasks(taskListID string, statusID int, orderedIDs []string) error {
	if err := s.store.ReorderTasks(taskListID, statusID, orderedIDs); err != nil {
		return err
	}
	s.emitter.Emit("taskList:updated", taskListID)
	return nil
}

func (s *Service) PromoteTask(id string) error {
	if err := s.store.PromoteTask(id); err != nil {
		return err
	}
	task, _ := s.store.GetTask(id)
	s.emitter.Emit("task:updated", task)
	return nil
}

func (s *Service) DemoteTask(id string, parentID string) error {
	if err := s.store.DemoteTask(id, parentID); err != nil {
		return err
	}
	task, _ := s.store.GetTask(id)
	s.emitter.Emit("task:updated", task)
	return nil
}

func (s *Service) MoveTaskToList(taskID string, targetTaskListID string) (*database.Task, error) {
	oldTask, err := s.store.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	oldListID := oldTask.TaskListID

	task, err := s.store.MoveTaskToList(taskID, targetTaskListID)
	if err != nil {
		return nil, err
	}

	if oldListID != targetTaskListID {
		s.emitter.Emit("task:updated", task)
		s.emitter.Emit("taskList:updated", oldListID)
		s.emitter.Emit("taskList:updated", targetTaskListID)
	}
	return task, nil
}

func (s *Service) DeleteTask(id string) error {
	if err := s.store.DeleteTask(id); err != nil {
		return err
	}
	s.emitter.Emit("task:deleted", id)
	return nil
}

func (s *Service) GetSubtasks(parentID string) ([]database.Task, error) {
	return s.store.GetSubtasks(parentID)
}

// ── Task Note ─────────────────────────────────────────────────────────────────

func (s *Service) CreateTaskNote(taskID string, noteType int, content, authorName, authorID string) (*database.TaskNote, error) {
	note, err := s.store.CreateTaskNote(taskID, database.TaskNoteType(noteType), content, authorName, authorID)
	if err != nil {
		return nil, err
	}
	s.emitter.Emit("taskNote:created", note)
	return note, nil
}

func (s *Service) UpsertTaskNoteByExternal(p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	note, created, err := s.store.UpsertTaskNoteByExternal(p)
	if err != nil {
		return nil, false, err
	}
	if created {
		s.emitter.Emit("taskNote:created", note)
	} else {
		s.emitter.Emit("taskNote:updated", note.ID)
	}
	return note, created, nil
}

func (s *Service) GetTaskNotes(taskID string) ([]database.TaskNote, error) {
	return s.store.GetTaskNotes(taskID)
}

func (s *Service) GetTaskNote(noteID string) (*database.TaskNote, error) {
	return s.store.GetTaskNote(noteID)
}

func (s *Service) UpdateTaskNote(noteID string, content string) error {
	if err := s.store.UpdateTaskNote(noteID, content); err != nil {
		return err
	}
	s.emitter.Emit("taskNote:updated", noteID)
	return nil
}

func (s *Service) DeleteTaskNote(noteID string) error {
	if err := s.store.DeleteTaskNote(noteID); err != nil {
		return err
	}
	s.emitter.Emit("taskNote:deleted", noteID)
	return nil
}
