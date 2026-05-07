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
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateTaskListFullWithContext(ctx, id, title, description, preferredViewMode, slug)
}

func (s *DBStore) ResolveTaskListRef(taskListID *string, taskListSlug string) (string, error) {
	ctx, err := s.ctx()
	if err != nil {
		return "", err
	}
	return database.ResolveTaskListIDWithContext(ctx, taskListID, taskListSlug)
}

func (s *DBStore) SetTaskListValidationPolicy(taskListID string, policyJSON string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.SetTaskListValidationPolicyWithContext(ctx, taskListID, policyJSON)
}

func (s *DBStore) SetTaskListViewMode(id string, viewMode string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.SetTaskListViewModeWithContext(ctx, id, viewMode)
}

func (s *DBStore) CloneTaskList(id string, newTitle string) (*database.TaskList, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.CloneTaskListWithContext(ctx, id, newTitle)
}

func (s *DBStore) ClearTaskList(id string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.ClearTaskListWithContext(ctx, id)
}

func (s *DBStore) DeleteTaskList(id string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.DeleteTaskListWithContext(ctx, id)
}

func (s *DBStore) GetTaskListStats(taskListID string) (map[string]interface{}, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTaskListStatsWithContext(ctx, taskListID)
}

func (s *DBStore) GetTaskListWithHierarchy(id string) (*database.TaskList, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTaskListWithHierarchyWithContext(ctx, id)
}

// ── Workflow ───────────────────────────────────────────────────────────────────

func (s *DBStore) GetWorkflow(taskListID string) (*database.TaskListWorkflow, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetWorkflowWithContext(ctx, taskListID)
}

func (s *DBStore) UpdateWorkflow(taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateWorkflowWithContext(ctx, taskListID, statuses, transitions)
}

func (s *DBStore) UpdateWorkflowFull(taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateWorkflowFullWithContext(ctx, taskListID, statuses, transitions, initialStatusID, statusMigration)
}

func (s *DBStore) GetTaskCountsByStatus(taskListID string) (map[int]int64, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTaskCountsByStatusWithContext(ctx, taskListID)
}

func (s *DBStore) ReorderWorkflowStatuses(taskListID string, statusOrder []int) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.ReorderWorkflowStatusesWithContext(ctx, taskListID, statusOrder)
}

func (s *DBStore) ValidateStatusTransition(taskListID string, fromStatusID, toStatusID int) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.ValidateStatusTransitionWithContext(ctx, taskListID, fromStatusID, toStatusID)
}

// ── Task ──────────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTask(taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.CreateTaskWithContext(ctx, taskListID, title, description, code, link, parentID)
}

func (s *DBStore) CreateTaskFull(taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.CreateTaskFullWithContext(ctx, taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
}

func (s *DBStore) GetTask(id string) (*database.Task, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTaskWithContext(ctx, id)
}

func (s *DBStore) GetTasksByTaskListID(taskListID string) ([]database.Task, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTasksByTaskListIDWithContext(ctx, taskListID)
}

func (s *DBStore) GetTasksByStatus(taskListID string, statusID int) ([]database.Task, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTasksByStatusWithContext(ctx, taskListID, statusID)
}

func (s *DBStore) FindTaskByCode(taskListID string, code string) (*database.Task, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.FindTaskByCodeWithContext(ctx, taskListID, code)
}

func (s *DBStore) ResolveTaskRef(taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	ctx, err := s.ctx()
	if err != nil {
		return "", err
	}
	return database.ResolveTaskIDWithContext(ctx, taskListID, taskListSlug, taskID, code)
}

func (s *DBStore) ResolveTaskIDByTaskCode(taskListID *string, taskCode string) (string, error) {
	ctx, err := s.ctx()
	if err != nil {
		return "", err
	}
	return database.ResolveTaskIDByTaskCodeWithContext(ctx, taskListID, taskCode)
}

func (s *DBStore) UpdateTask(id string, title, description, code, link string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateTaskWithContext(ctx, id, title, description, code, link)
}

func (s *DBStore) UpdateTaskFull(id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateTaskFullWithContext(ctx, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}

func (s *DBStore) UpdateTaskAssignee(id string, assigneeName, assigneeID string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateTaskAssigneeWithContext(ctx, id, assigneeName, assigneeID)
}

func (s *DBStore) UpdateTaskStatus(id string, newStatusID int) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateTaskStatusWithContext(ctx, id, newStatusID)
}

func (s *DBStore) ReorderTasks(taskListID string, statusID int, orderedIDs []string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.ReorderTasksWithContext(ctx, taskListID, statusID, orderedIDs)
}

func (s *DBStore) PromoteTask(id string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.PromoteTaskWithContext(ctx, id)
}

func (s *DBStore) DemoteTask(id string, parentID string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.DemoteTaskWithContext(ctx, id, parentID)
}

func (s *DBStore) MoveTaskToList(taskID string, targetTaskListID string) (*database.Task, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.MoveTaskToListWithContext(ctx, taskID, targetTaskListID)
}

func (s *DBStore) DeleteTask(id string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.DeleteTaskWithContext(ctx, id)
}

func (s *DBStore) GetSubtasks(parentID string) ([]database.Task, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetSubtasksWithContext(ctx, parentID)
}

// ── Task Note ─────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTaskNote(taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.CreateTaskNoteWithContext(ctx, taskID, noteType, content, authorName, authorID)
}

func (s *DBStore) UpsertTaskNoteByExternal(p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, false, err
	}
	return database.UpsertTaskNoteByExternalWithContext(ctx, p)
}

func (s *DBStore) GetTaskNotes(taskID string) ([]database.TaskNote, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTaskNotesWithContext(ctx, taskID)
}

func (s *DBStore) GetTaskNote(noteID string) (*database.TaskNote, error) {
	ctx, err := s.ctx()
	if err != nil {
		return nil, err
	}
	return database.GetTaskNoteWithContext(ctx, noteID)
}

func (s *DBStore) UpdateTaskNote(noteID string, content string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.UpdateTaskNoteWithContext(ctx, noteID, content)
}

func (s *DBStore) DeleteTaskNote(noteID string) error {
	ctx, err := s.ctx()
	if err != nil {
		return err
	}
	return database.DeleteTaskNoteWithContext(ctx, noteID)
}
