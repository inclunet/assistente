package tasklist

import (
	"context"

	"assistente/internal/database"
)

// DBStore implementa TaskListRepository usando o banco de dados SQLite via GORM.
type DBStore struct {
	ctxProvider func() context.Context
	requireUser bool
}

// NewDBStore cria um DBStore pronto para uso.
func NewDBStore() *DBStore { return &DBStore{} }

func NewScopedDBStore(ctxProvider func() context.Context) *DBStore {
	return &DBStore{ctxProvider: ctxProvider, requireUser: true}
}

func (s *DBStore) ctx() (context.Context, error) {
	ctx := context.Background()
	if s.ctxProvider != nil {
		ctx = s.ctxProvider()
	}
	if s.requireUser {
		if _, err := database.RequireUserID(ctx); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

// ── Task List ──────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.CreateTaskListWithContext(ctx, title, description, templateWorkflow, slug)
}

func (s *DBStore) GetTaskList(id string) (*database.TaskList, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTaskListWithContext(ctx, id)
}

func (s *DBStore) GetAllTaskLists() ([]database.TaskList, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetAllTaskListsWithContext(ctx)
}

func (s *DBStore) UpdateTaskList(id string, title, description string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateTaskListWithContext(ctx, id, title, description)
}

func (s *DBStore) UpdateTaskListFull(id string, title, description, preferredViewMode string, slug *string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.UpdateTaskListFull(id, title, description, preferredViewMode, slug)
}

func (s *DBStore) ResolveTaskListRef(taskListID *string, taskListSlug string) (string, error) {
	if _, err := s.ctx(); err != nil {
		return "", err
	}
	return database.ResolveTaskListID(taskListID, taskListSlug)
}

func (s *DBStore) SetTaskListValidationPolicy(taskListID string, policyJSON string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.SetTaskListValidationPolicy(taskListID, policyJSON)
}

func (s *DBStore) SetTaskListViewMode(id string, viewMode string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.SetTaskListViewModeWithContext(ctx, id, viewMode)
}

func (s *DBStore) CloneTaskList(id string, newTitle string) (*database.TaskList, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.CloneTaskList(id, newTitle)
}

func (s *DBStore) ClearTaskList(id string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.ClearTaskList(id)
}

func (s *DBStore) DeleteTaskList(id string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.DeleteTaskList(id)
}

func (s *DBStore) GetTaskListStats(taskListID string) (map[string]interface{}, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetTaskListStats(taskListID)
}

func (s *DBStore) GetTaskListWithHierarchy(id string) (*database.TaskList, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetTaskListWithHierarchy(id)
}

// ── Workflow ───────────────────────────────────────────────────────────────────

func (s *DBStore) GetWorkflow(taskListID string) (*database.TaskListWorkflow, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetWorkflow(taskListID)
}

func (s *DBStore) UpdateWorkflow(taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.UpdateWorkflow(taskListID, statuses, transitions)
}

func (s *DBStore) UpdateWorkflowFull(taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.UpdateWorkflowFull(taskListID, statuses, transitions, initialStatusID, statusMigration)
}

func (s *DBStore) GetTaskCountsByStatus(taskListID string) (map[int]int64, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetTaskCountsByStatus(taskListID)
}

func (s *DBStore) ReorderWorkflowStatuses(taskListID string, statusOrder []int) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.ReorderWorkflowStatuses(taskListID, statusOrder)
}

func (s *DBStore) ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.ValidateStatusTransition(taskListID, fromStatusID, toStatusID)
}

// ── Task ──────────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTask(taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.CreateTask(taskListID, title, description, code, link, parentID)
}

func (s *DBStore) CreateTaskFull(taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.CreateTaskFull(taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
}

func (s *DBStore) GetTask(id string) (*database.Task, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetTask(id)
}

func (s *DBStore) GetTasksByTaskListID(taskListID string) ([]database.Task, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetTasksByTaskListID(taskListID)
}

func (s *DBStore) GetTasksByStatus(taskListID string, statusID int) ([]database.Task, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetTasksByStatus(taskListID, statusID)
}

func (s *DBStore) FindTaskByCode(taskListID string, code string) (*database.Task, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.FindTaskByCode(taskListID, code)
}

func (s *DBStore) ResolveTaskRef(taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	if _, err := s.ctx(); err != nil {
		return "", err
	}
	return database.ResolveTaskID(taskListID, taskListSlug, taskID, code)
}

func (s *DBStore) ResolveTaskIDByTaskCode(taskListID *string, taskCode string) (string, error) {
	if _, err := s.ctx(); err != nil {
		return "", err
	}
	return database.ResolveTaskIDByTaskCode(taskListID, taskCode)
}

func (s *DBStore) UpdateTask(id string, title, description, code, link string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.UpdateTask(id, title, description, code, link)
}

func (s *DBStore) UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}

func (s *DBStore) UpdateTaskAssignee(id string, assigneeName, assigneeID string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.UpdateTaskAssignee(id, assigneeName, assigneeID)
}

func (s *DBStore) UpdateTaskStatus(id string, newStatusID int) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.UpdateTaskStatus(id, newStatusID)
}

func (s *DBStore) ReorderTasks(taskListID string, statusID int, orderedIDs []string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.ReorderTasks(taskListID, statusID, orderedIDs)
}

func (s *DBStore) PromoteTask(id string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.PromoteTask(id)
}

func (s *DBStore) DemoteTask(id string, parentID string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.DemoteTask(id, parentID)
}

func (s *DBStore) MoveTaskToList(taskID string, targetTaskListID string) (*database.Task, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.MoveTaskToList(taskID, targetTaskListID)
}

func (s *DBStore) DeleteTask(id string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.DeleteTask(id)
}

func (s *DBStore) GetSubtasks(parentID string) ([]database.Task, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetSubtasks(parentID)
}

// ── Task Note ─────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTaskNote(taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.CreateTaskNote(taskID, noteType, content, authorName, authorID)
}

func (s *DBStore) UpsertTaskNoteByExternal(p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	if _, err := s.ctx(); err != nil {
		return nil, false, err
	}
	return database.UpsertTaskNoteByExternal(p)
}

func (s *DBStore) GetTaskNotes(taskID string) ([]database.TaskNote, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetTaskNotes(taskID)
}

func (s *DBStore) GetTaskNote(noteID string) (*database.TaskNote, error) {
	if _, err := s.ctx(); err != nil {
		return nil, err
	}
	return database.GetTaskNote(noteID)
}

func (s *DBStore) UpdateTaskNote(noteID string, content string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.UpdateTaskNote(noteID, content)
}

func (s *DBStore) DeleteTaskNote(noteID string) error {
	if _, err := s.ctx(); err != nil {
		return err
	}
	return database.DeleteTaskNote(noteID)
}
