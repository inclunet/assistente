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
	Store        TaskListRepository
	Emitter      EventEmitter
	DomainEvents DomainEventSink // opcional: ponte para o EventBus de jobs (AEP-0067)
}

// Service encapsula toda a lógica de negócio de task lists.
// Chama TaskListRepository para persistência e EventEmitter para notificações.
type Service struct {
	store   TaskListRepository
	emitter EventEmitter
	domain  DomainEventSink
}

// NewService cria um Service com as dependências fornecidas.
func NewService(cfg ServiceConfig) *Service {
	return &Service{store: cfg.Store, emitter: cfg.Emitter, domain: cfg.DomainEvents}
}

// SetDomainEventSink injeta (ou substitui) o sink de eventos de domínio.
// Usado pela wiring da app, onde o jobs.Manager só existe após o Service.
func (s *Service) SetDomainEventSink(sink DomainEventSink) {
	s.domain = sink
}

// ── Task List ──────────────────────────────────────────────────────────────────

// CreateTaskList cria uma nova lista e recarrega o registro completo.
func (s *Service) CreateTaskList(ctx context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	tl, err := s.store.CreateTaskList(ctx, title, description, templateWorkflow, slug)
	if err != nil {
		return nil, err
	}
	full, err := s.store.GetTaskList(ctx, tl.ID)
	if err != nil {
		if tl.Tasks == nil {
			tl.Tasks = []database.Task{}
		}
		full = tl
	}
	s.emitter.Emit("taskList:created", full)
	if s.wantsDomain("tasklist.list.created") {
		s.publishDomain(ctx, "tasklist.list.created", listPayload(full))
	}
	return full, nil
}

func (s *Service) GetTaskList(ctx context.Context, id string) (*database.TaskList, error) {
	return s.store.GetTaskList(ctx, id)
}

func (s *Service) GetAllTaskLists(ctx context.Context) ([]database.TaskList, error) {
	return s.store.GetAllTaskLists(ctx)
}

func (s *Service) UpdateTaskList(ctx context.Context, id string, title, description string) error {
	if err := s.store.UpdateTaskList(ctx, id, title, description); err != nil {
		return err
	}
	tl, _ := s.store.GetTaskList(ctx, id)
	s.emitter.Emit("taskList:updated", tl)
	if s.wantsDomain("tasklist.list.updated") {
		s.publishDomain(ctx, "tasklist.list.updated", s.listEventPayload(ctx, tl, id))
	}
	return nil
}

func (s *Service) UpdateTaskListFull(ctx context.Context, id string, title, description, preferredViewMode string, slug *string) error {
	if err := s.store.UpdateTaskListFull(ctx, id, title, description, preferredViewMode, slug); err != nil {
		return err
	}
	if s.wantsDomain("tasklist.list.updated") {
		tl, _ := s.store.GetTaskList(ctx, id)
		s.publishDomain(ctx, "tasklist.list.updated", s.listEventPayload(ctx, tl, id))
	}
	return nil
}

func (s *Service) ResolveTaskListRef(ctx context.Context, taskListID *string, taskListSlug string) (string, error) {
	return s.store.ResolveTaskListRef(ctx, taskListID, taskListSlug)
}

func (s *Service) SetTaskListValidationPolicy(ctx context.Context, taskListID string, policyJSON string) error {
	return s.store.SetTaskListValidationPolicy(ctx, taskListID, policyJSON)
}

func (s *Service) SetTaskListViewMode(ctx context.Context, id string, viewMode string) error {
	if err := s.store.SetTaskListViewMode(ctx, id, viewMode); err != nil {
		return err
	}
	tl, _ := s.store.GetTaskList(ctx, id)
	s.emitter.Emit("taskList:updated", tl)
	if s.wantsDomain("tasklist.list.updated") {
		s.publishDomain(ctx, "tasklist.list.updated", s.listEventPayload(ctx, tl, id))
	}
	return nil
}

// CloneTaskList clona uma lista e recarrega o registro completo.
func (s *Service) CloneTaskList(ctx context.Context, id string, newTitle string) (*database.TaskList, error) {
	tl, err := s.store.CloneTaskList(ctx, id, newTitle)
	if err != nil {
		return nil, err
	}
	full, err := s.store.GetTaskList(ctx, tl.ID)
	if err != nil {
		if tl.Tasks == nil {
			tl.Tasks = []database.Task{}
		}
		full = tl
	}
	s.emitter.Emit("taskList:created", full)
	if s.wantsDomain("tasklist.list.cloned") {
		payload := listPayload(full)
		payload["source_task_list_id"] = id
		s.publishDomain(ctx, "tasklist.list.cloned", payload)
	}
	return full, nil
}

func (s *Service) ClearTaskList(ctx context.Context, id string) error {
	if err := s.store.ClearTaskList(ctx, id); err != nil {
		return err
	}
	s.emitter.Emit("taskList:cleared", id)
	if s.wantsDomain("tasklist.list.cleared") {
		s.publishDomain(ctx, "tasklist.list.cleared", map[string]any{
			"task_list_id":   id,
			"task_list_slug": s.taskListSlug(ctx, id),
		})
	}
	return nil
}

func (s *Service) DeleteTaskList(ctx context.Context, id string) error {
	// Resolve o slug antes de deletar (best-effort) para enriquecer o evento.
	var slug string
	if s.wantsDomain("tasklist.list.deleted") {
		slug = s.taskListSlug(ctx, id)
	}
	if err := s.store.DeleteTaskList(ctx, id); err != nil {
		return err
	}
	s.emitter.Emit("taskList:deleted", id)
	if s.wantsDomain("tasklist.list.deleted") {
		s.publishDomain(ctx, "tasklist.list.deleted", map[string]any{
			"task_list_id":   id,
			"task_list_slug": slug,
		})
	}
	return nil
}

func (s *Service) GetTaskListStats(ctx context.Context, taskListID string) (map[string]interface{}, error) {
	return s.store.GetTaskListStats(ctx, taskListID)
}

func (s *Service) GetTaskListWithHierarchy(ctx context.Context, id string) (*database.TaskList, error) {
	return s.store.GetTaskListWithHierarchy(ctx, id)
}

// ── Workflow ───────────────────────────────────────────────────────────────────

func (s *Service) GetWorkflow(ctx context.Context, taskListID string) (*database.TaskListWorkflow, error) {
	return s.store.GetWorkflow(ctx, taskListID)
}

func (s *Service) UpdateWorkflow(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	if err := s.store.UpdateWorkflow(ctx, taskListID, statuses, transitions); err != nil {
		return err
	}
	wf, _ := s.store.GetWorkflow(ctx, taskListID)
	s.emitter.Emit("workflow:updated", wf)
	if s.wantsDomain("tasklist.workflow.updated") {
		s.publishDomain(ctx, "tasklist.workflow.updated", s.workflowEventPayload(ctx, taskListID, wf))
	}
	return nil
}

func (s *Service) UpdateWorkflowFull(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	if err := s.store.UpdateWorkflowFull(ctx, taskListID, statuses, transitions, initialStatusID, statusMigration); err != nil {
		return err
	}
	tl, _ := s.store.GetTaskList(ctx, taskListID)
	if tl != nil && tl.Workflow != nil {
		s.emitter.Emit("workflow:updated", tl.Workflow)
	}
	s.emitter.Emit("taskList:updated", tl)
	if s.wantsDomain("tasklist.workflow.updated") {
		var wf *database.TaskListWorkflow
		if tl != nil {
			wf = tl.Workflow
		}
		s.publishDomain(ctx, "tasklist.workflow.updated", s.workflowEventPayload(ctx, taskListID, wf))
	}
	return nil
}

func (s *Service) GetTaskCountsByStatus(ctx context.Context, taskListID string) (map[int]int64, error) {
	return s.store.GetTaskCountsByStatus(ctx, taskListID)
}

func (s *Service) ReorderWorkflowStatuses(ctx context.Context, taskListID string, statusOrder []int) error {
	if err := s.store.ReorderWorkflowStatuses(ctx, taskListID, statusOrder); err != nil {
		return err
	}
	wf, _ := s.store.GetWorkflow(ctx, taskListID)
	s.emitter.Emit("workflow:updated", wf)
	if s.wantsDomain("tasklist.workflow.updated") {
		s.publishDomain(ctx, "tasklist.workflow.updated", s.workflowEventPayload(ctx, taskListID, wf))
	}
	return nil
}

func (s *Service) ValidateStatusTransition(ctx context.Context, taskListID string, fromStatusID, toStatusID int) error {
	return s.store.ValidateStatusTransition(ctx, taskListID, fromStatusID, toStatusID)
}

// ── Task ───────────────────────────────────────────────────────────────────────

func (s *Service) CreateTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	task, err := s.store.CreateTask(ctx, taskListID, title, description, code, link, parentID)
	if err != nil {
		return nil, err
	}
	s.emitter.Emit("task:created", task)
	if s.wantsDomain("tasklist.task.created") {
		if payload := s.taskEventPayload(ctx, task, nil); payload != nil {
			s.publishDomain(ctx, "tasklist.task.created", payload)
		}
	}
	return task, nil
}

func (s *Service) CreateTaskFull(ctx context.Context, taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error) {
	task, err := s.store.CreateTaskFull(ctx, taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
	if err != nil {
		return nil, err
	}
	s.emitter.Emit("task:created", task)
	if s.wantsDomain("tasklist.task.created") {
		if payload := s.taskEventPayload(ctx, task, nil); payload != nil {
			s.publishDomain(ctx, "tasklist.task.created", payload)
		}
	}
	return task, nil
}

func (s *Service) GetTask(ctx context.Context, id string) (*database.Task, error) {
	return s.store.GetTask(ctx, id)
}

func (s *Service) GetTasksByTaskListID(ctx context.Context, taskListID string) ([]database.Task, error) {
	return s.store.GetTasksByTaskListID(ctx, taskListID)
}

func (s *Service) GetTasksByStatus(ctx context.Context, taskListID string, statusID int) ([]database.Task, error) {
	return s.store.GetTasksByStatus(ctx, taskListID, statusID)
}

func (s *Service) FindTaskByCode(ctx context.Context, taskListID string, code string) (*database.Task, error) {
	return s.store.FindTaskByCode(ctx, taskListID, code)
}

func (s *Service) ResolveTaskRef(ctx context.Context, taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	return s.store.ResolveTaskRef(ctx, taskListID, taskListSlug, taskID, code)
}

func (s *Service) ResolveTaskIDByTaskCode(ctx context.Context, taskListID *string, taskCode string) (string, error) {
	return s.store.ResolveTaskIDByTaskCode(ctx, taskListID, taskCode)
}

func (s *Service) UpdateTask(ctx context.Context, id string, title, description, code, link string) error {
	var old *database.Task
	if s.wantsDomain("tasklist.task.updated") {
		old, _ = s.store.GetTask(ctx, id)
	}
	if err := s.store.UpdateTask(ctx, id, title, description, code, link); err != nil {
		return err
	}
	task, _ := s.store.GetTask(ctx, id)
	s.emitter.Emit("task:updated", task)
	if s.wantsDomain("tasklist.task.updated") {
		if payload := s.taskEventPayload(ctx, task, old); payload != nil {
			// Só preenche changed_fields quando há snapshot recarregado: no
			// fallback (task==nil) o diff ficaria vazio e induziria o consumidor
			// a achar que nada mudou. Melhor omitir do que mentir.
			if task != nil {
				payload["changed_fields"] = changedTaskFields(old, task)
			}
			s.publishDomain(ctx, "tasklist.task.updated", payload)
		}
	}
	return nil
}

func (s *Service) UpdateTaskFull(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	wantUpdated := s.wantsDomain("tasklist.task.updated")
	wantAssignee := s.wantsDomain("tasklist.task.assignee_changed")
	var old *database.Task
	if wantUpdated || wantAssignee {
		old, _ = s.store.GetTask(ctx, id)
	}
	if err := s.store.UpdateTaskFull(ctx, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID); err != nil {
		return err
	}
	task, _ := s.store.GetTask(ctx, id)
	s.emitter.Emit("task:updated", task)
	if wantUpdated {
		if payload := s.taskEventPayload(ctx, task, old); payload != nil {
			// Ver UpdateTask: changed_fields só com snapshot recarregado.
			if task != nil {
				payload["changed_fields"] = changedTaskFields(old, task)
			}
			s.publishDomain(ctx, "tasklist.task.updated", payload)
		}
	}
	// UpdateTaskFull também pode trocar o responsável: emite assignee_changed com
	// o mesmo contrato de UpdateTaskAssignee, para não esconder a troca de quem
	// escuta especificamente esse evento (e não só tasklist.task.updated).
	if wantAssignee && old != nil && old.AssigneeID != assigneeID {
		if payload := s.taskEventPayload(ctx, task, old); payload != nil {
			payload["from_assignee_id"] = old.AssigneeID
			s.publishDomain(ctx, "tasklist.task.assignee_changed", payload)
		}
	}
	return nil
}

// UpdateTaskAssignee atualiza o responsável da task e cria nota de auditoria se houve mudança.
func (s *Service) UpdateTaskAssignee(ctx context.Context, id string, assigneeName, assigneeID string) error {
	oldTask, _ := s.store.GetTask(ctx, id)
	if err := s.store.UpdateTaskAssignee(ctx, id, assigneeName, assigneeID); err != nil {
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
		note, _ := s.store.CreateTaskNote(ctx, id, database.TaskNoteSystem, content, "system", "")
		if note != nil {
			s.emitter.Emit("taskNote:created", note)
			if s.wantsDomain("tasklist.note.added") {
				s.publishDomain(ctx, "tasklist.note.added", notePayload(note, oldTask.TaskListID, s.taskListSlug(ctx, oldTask.TaskListID)))
			}
		}
	}

	task, _ := s.store.GetTask(ctx, id)
	s.emitter.Emit("task:updated", task)
	if s.wantsDomain("tasklist.task.assignee_changed") && oldTask != nil && oldTask.AssigneeID != assigneeID {
		if payload := s.taskEventPayload(ctx, task, oldTask); payload != nil {
			payload["from_assignee_id"] = oldTask.AssigneeID
			s.publishDomain(ctx, "tasklist.task.assignee_changed", payload)
		}
	}
	return nil
}

func (s *Service) UpdateTaskStatus(ctx context.Context, id string, statusID int) error {
	wantStatus := s.wantsDomain("tasklist.task.status_changed")
	wantCompleted := s.wantsDomain("tasklist.task.completed")
	var old *database.Task
	if wantStatus || wantCompleted {
		old, _ = s.store.GetTask(ctx, id)
	}
	if err := s.store.UpdateTaskStatus(ctx, id, statusID); err != nil {
		return err
	}
	task, _ := s.store.GetTask(ctx, id)
	s.emitter.Emit("task:updated", task)

	if wantStatus {
		// Não emite em no-op (status igual): evita disparar jobs à toa e facilita
		// o anti-loop. Só dá para detectar no-op quando há snapshot anterior; sem
		// ele (old==nil), publicamos best-effort.
		statusChanged := old == nil || old.StatusID != statusID
		// Best-effort mesmo se a recarga falhar (task==nil): cai para o snapshot
		// anterior e força status_id ao novo valor do input, mantendo o contrato.
		if statusChanged {
			if payload := s.taskEventPayload(ctx, task, old); payload != nil {
				payload["status_id"] = statusID
				if old != nil {
					payload["from_status_id"] = old.StatusID
				}
				s.publishDomain(ctx, "tasklist.task.status_changed", payload)
			}
		}
	}
	// Derivado: transição para concluído. Exige o snapshot recarregado, pois
	// depende de completed_at já estar setado pós-mutação (o snapshot antigo não
	// reflete isso).
	if wantCompleted && task != nil && task.CompletedAt != nil && (old == nil || old.CompletedAt == nil) {
		s.publishDomain(ctx, "tasklist.task.completed", taskPayload(task, s.taskListSlug(ctx, task.TaskListID)))
	}
	return nil
}

func (s *Service) ReorderTasks(ctx context.Context, taskListID string, statusID int, orderedIDs []string) error {
	if err := s.store.ReorderTasks(ctx, taskListID, statusID, orderedIDs); err != nil {
		return err
	}
	s.emitter.Emit("taskList:updated", taskListID)
	if s.wantsDomain("tasklist.task.reordered") {
		s.publishDomain(ctx, "tasklist.task.reordered", map[string]any{
			"task_list_id":   taskListID,
			"task_list_slug": s.taskListSlug(ctx, taskListID),
			"status_id":      statusID,
			"ordered_ids":    orderedIDs,
		})
	}
	return nil
}

func (s *Service) PromoteTask(ctx context.Context, id string) error {
	var old *database.Task
	if s.wantsDomain("tasklist.task.reparented") {
		old, _ = s.store.GetTask(ctx, id)
	}
	if err := s.store.PromoteTask(ctx, id); err != nil {
		return err
	}
	task, _ := s.store.GetTask(ctx, id)
	s.emitter.Emit("task:updated", task)
	if s.wantsDomain("tasklist.task.reparented") {
		if payload := s.taskEventPayload(ctx, task, old); payload != nil {
			if old != nil {
				payload["from_parent_id"] = parentOrEmpty(old.ParentID)
			}
			s.publishDomain(ctx, "tasklist.task.reparented", payload)
		}
	}
	return nil
}

func (s *Service) DemoteTask(ctx context.Context, id string, parentID string) error {
	var old *database.Task
	if s.wantsDomain("tasklist.task.reparented") {
		old, _ = s.store.GetTask(ctx, id)
	}
	if err := s.store.DemoteTask(ctx, id, parentID); err != nil {
		return err
	}
	task, _ := s.store.GetTask(ctx, id)
	s.emitter.Emit("task:updated", task)
	if s.wantsDomain("tasklist.task.reparented") {
		if payload := s.taskEventPayload(ctx, task, old); payload != nil {
			if old != nil {
				payload["from_parent_id"] = parentOrEmpty(old.ParentID)
			}
			s.publishDomain(ctx, "tasklist.task.reparented", payload)
		}
	}
	return nil
}

func (s *Service) MoveTaskToList(ctx context.Context, taskID string, targetTaskListID string) (*database.Task, error) {
	oldTask, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	oldListID := oldTask.TaskListID

	task, err := s.store.MoveTaskToList(ctx, taskID, targetTaskListID)
	if err != nil {
		return nil, err
	}

	if oldListID != targetTaskListID {
		s.emitter.Emit("task:updated", task)
		s.emitter.Emit("taskList:updated", oldListID)
		s.emitter.Emit("taskList:updated", targetTaskListID)
		if s.wantsDomain("tasklist.task.moved") {
			if payload := s.taskEventPayload(ctx, task, nil); payload != nil {
				payload["from_task_list_id"] = oldListID
				s.publishDomain(ctx, "tasklist.task.moved", payload)
			}
		}
	}
	return task, nil
}

func (s *Service) DeleteTask(ctx context.Context, id string) error {
	// Captura o card antes de deletar (best-effort) para enriquecer o evento.
	var old *database.Task
	if s.wantsDomain("tasklist.task.deleted") {
		old, _ = s.store.GetTask(ctx, id)
	}
	if err := s.store.DeleteTask(ctx, id); err != nil {
		return err
	}
	s.emitter.Emit("task:deleted", id)
	if s.wantsDomain("tasklist.task.deleted") {
		if old != nil {
			s.publishDomain(ctx, "tasklist.task.deleted", taskPayload(old, s.taskListSlug(ctx, old.TaskListID)))
		} else {
			s.publishDomain(ctx, "tasklist.task.deleted", map[string]any{"task_id": id})
		}
	}
	return nil
}

func (s *Service) GetSubtasks(ctx context.Context, parentID string) ([]database.Task, error) {
	return s.store.GetSubtasks(ctx, parentID)
}

// ── Task Note ─────────────────────────────────────────────────────────────────

func (s *Service) CreateTaskNote(ctx context.Context, taskID string, noteType int, content, authorName, authorID string) (*database.TaskNote, error) {
	note, err := s.store.CreateTaskNote(ctx, taskID, database.TaskNoteType(noteType), content, authorName, authorID)
	if err != nil {
		return nil, err
	}
	s.emitter.Emit("taskNote:created", note)
	if s.wantsDomain("tasklist.note.added") {
		listID, slug := s.listRefForTask(ctx, note.TaskID)
		s.publishDomain(ctx, "tasklist.note.added", notePayload(note, listID, slug))
	}
	return note, nil
}

func (s *Service) UpsertTaskNoteByExternal(ctx context.Context, p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	note, created, err := s.store.UpsertTaskNoteByExternal(ctx, p)
	if err != nil {
		return nil, false, err
	}
	if created {
		s.emitter.Emit("taskNote:created", note)
		if s.wantsDomain("tasklist.note.added") {
			listID, slug := s.listRefForTask(ctx, note.TaskID)
			s.publishDomain(ctx, "tasklist.note.added", notePayload(note, listID, slug))
		}
	} else {
		s.emitter.Emit("taskNote:updated", note.ID)
		if s.wantsDomain("tasklist.note.updated") {
			listID, slug := s.listRefForTask(ctx, note.TaskID)
			s.publishDomain(ctx, "tasklist.note.updated", notePayload(note, listID, slug))
		}
	}
	return note, created, nil
}

func (s *Service) GetTaskNotes(ctx context.Context, taskID string) ([]database.TaskNote, error) {
	return s.store.GetTaskNotes(ctx, taskID)
}

func (s *Service) GetTaskNote(ctx context.Context, noteID string) (*database.TaskNote, error) {
	return s.store.GetTaskNote(ctx, noteID)
}

func (s *Service) UpdateTaskNote(ctx context.Context, noteID string, content string) error {
	if err := s.store.UpdateTaskNote(ctx, noteID, content); err != nil {
		return err
	}
	s.emitter.Emit("taskNote:updated", noteID)
	if s.wantsDomain("tasklist.note.updated") {
		note, _ := s.store.GetTaskNote(ctx, noteID)
		if note != nil {
			listID, slug := s.listRefForTask(ctx, note.TaskID)
			s.publishDomain(ctx, "tasklist.note.updated", notePayload(note, listID, slug))
		} else {
			s.publishDomain(ctx, "tasklist.note.updated", map[string]any{"note_id": noteID})
		}
	}
	return nil
}

func (s *Service) DeleteTaskNote(ctx context.Context, noteID string) error {
	var old *database.TaskNote
	if s.wantsDomain("tasklist.note.deleted") {
		old, _ = s.store.GetTaskNote(ctx, noteID)
	}
	if err := s.store.DeleteTaskNote(ctx, noteID); err != nil {
		return err
	}
	s.emitter.Emit("taskNote:deleted", noteID)
	if s.wantsDomain("tasklist.note.deleted") {
		if old != nil {
			listID, slug := s.listRefForTask(ctx, old.TaskID)
			s.publishDomain(ctx, "tasklist.note.deleted", notePayload(old, listID, slug))
		} else {
			s.publishDomain(ctx, "tasklist.note.deleted", map[string]any{"note_id": noteID})
		}
	}
	return nil
}
