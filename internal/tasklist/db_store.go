package tasklist

import (
	"context"

	"assistente/internal/database"
)

// DBStore implementa TaskListRepository usando o banco de dados SQLite via GORM.
type DBStore struct{}

// NewDBStore cria um DBStore pronto para uso.
func NewDBStore() *DBStore { return &DBStore{} }

// ── Task List ──────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTaskList(ctx context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	return database.CreateTaskListWithContext(ctx, title, description, templateWorkflow, slug)
}

func (s *DBStore) GetTaskList(ctx context.Context, id string) (*database.TaskList, error) {
	return database.GetTaskListWithContext(ctx, id)
}

func (s *DBStore) GetAllTaskLists(ctx context.Context) ([]database.TaskList, error) {
	return database.GetAllTaskListsWithContext(ctx)
}

func (s *DBStore) UpdateTaskList(ctx context.Context, id string, title, description string) error {
	return database.UpdateTaskListWithContext(ctx, id, title, description)
}

func (s *DBStore) UpdateTaskListFull(ctx context.Context, id string, title, description, preferredViewMode string, slug *string) error {
	return database.UpdateTaskListFullWithContext(ctx, id, title, description, preferredViewMode, slug)
}

func (s *DBStore) ResolveTaskListRef(ctx context.Context, taskListID *string, taskListSlug string) (string, error) {
	return database.ResolveTaskListIDWithContext(ctx, taskListID, taskListSlug)
}

func (s *DBStore) SetTaskListValidationPolicy(ctx context.Context, taskListID string, policyJSON string) error {
	return database.SetTaskListValidationPolicyWithContext(ctx, taskListID, policyJSON)
}

func (s *DBStore) SetTaskListViewMode(ctx context.Context, id string, viewMode string) error {
	return database.SetTaskListViewModeWithContext(ctx, id, viewMode)
}

func (s *DBStore) CloneTaskList(ctx context.Context, id string, newTitle string) (*database.TaskList, error) {
	return database.CloneTaskListWithContext(ctx, id, newTitle)
}

func (s *DBStore) ClearTaskList(ctx context.Context, id string) error {
	return database.ClearTaskListWithContext(ctx, id)
}

func (s *DBStore) DeleteTaskList(ctx context.Context, id string) error {
	return database.DeleteTaskListWithContext(ctx, id)
}

func (s *DBStore) GetTaskListStats(ctx context.Context, taskListID string) (map[string]interface{}, error) {
	return database.GetTaskListStatsWithContext(ctx, taskListID)
}

func (s *DBStore) GetTaskListWithHierarchy(ctx context.Context, id string) (*database.TaskList, error) {
	return database.GetTaskListWithHierarchyWithContext(ctx, id)
}

// ── Workflow ───────────────────────────────────────────────────────────────────

func (s *DBStore) GetWorkflow(ctx context.Context, taskListID string) (*database.TaskListWorkflow, error) {
	return database.GetWorkflowWithContext(ctx, taskListID)
}

func (s *DBStore) UpdateWorkflow(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	return database.UpdateWorkflowWithContext(ctx, taskListID, statuses, transitions)
}

func (s *DBStore) UpdateWorkflowFull(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	return database.UpdateWorkflowFullWithContext(ctx, taskListID, statuses, transitions, initialStatusID, statusMigration)
}

func (s *DBStore) GetTaskCountsByStatus(ctx context.Context, taskListID string) (map[int]int64, error) {
	return database.GetTaskCountsByStatusWithContext(ctx, taskListID)
}

func (s *DBStore) ReorderWorkflowStatuses(ctx context.Context, taskListID string, statusOrder []int) error {
	return database.ReorderWorkflowStatusesWithContext(ctx, taskListID, statusOrder)
}

func (s *DBStore) ValidateStatusTransition(ctx context.Context, taskListID string, fromStatusID, toStatusID int) error {
	return database.ValidateStatusTransitionWithContext(ctx, taskListID, fromStatusID, toStatusID)
}

// ── Task ──────────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	return database.CreateTaskWithContext(ctx, taskListID, title, description, code, link, parentID)
}

func (s *DBStore) CreateTaskFull(ctx context.Context, taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error) {
	return database.CreateTaskFullWithContext(ctx, taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
}

func (s *DBStore) GetTask(ctx context.Context, id string) (*database.Task, error) {
	return database.GetTaskWithContext(ctx, id)
}

func (s *DBStore) GetTasksByTaskListID(ctx context.Context, taskListID string) ([]database.Task, error) {
	return database.GetTasksByTaskListIDWithContext(ctx, taskListID)
}

func (s *DBStore) GetTasksByStatus(ctx context.Context, taskListID string, statusID int) ([]database.Task, error) {
	return database.GetTasksByStatusWithContext(ctx, taskListID, statusID)
}

func (s *DBStore) FindTaskByCode(ctx context.Context, taskListID string, code string) (*database.Task, error) {
	return database.FindTaskByCodeWithContext(ctx, taskListID, code)
}

func (s *DBStore) ResolveTaskRef(ctx context.Context, taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	return database.ResolveTaskIDWithContext(ctx, taskListID, taskListSlug, taskID, code)
}

func (s *DBStore) ResolveTaskIDByTaskCode(ctx context.Context, taskListID *string, taskCode string) (string, error) {
	return database.ResolveTaskIDByTaskCodeWithContext(ctx, taskListID, taskCode)
}

func (s *DBStore) UpdateTask(ctx context.Context, id string, title, description, code, link string) error {
	return database.UpdateTaskWithContext(ctx, id, title, description, code, link)
}

func (s *DBStore) UpdateTaskFull(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return database.UpdateTaskFullWithContext(ctx, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}

func (s *DBStore) UpdateTaskAssignee(ctx context.Context, id string, assigneeName, assigneeID string) error {
	return database.UpdateTaskAssigneeWithContext(ctx, id, assigneeName, assigneeID)
}

func (s *DBStore) UpdateTaskStatus(ctx context.Context, id string, newStatusID int) error {
	return database.UpdateTaskStatusWithContext(ctx, id, newStatusID)
}

func (s *DBStore) ReorderTasks(ctx context.Context, taskListID string, statusID int, orderedIDs []string) error {
	return database.ReorderTasksWithContext(ctx, taskListID, statusID, orderedIDs)
}

func (s *DBStore) PromoteTask(ctx context.Context, id string) error {
	return database.PromoteTaskWithContext(ctx, id)
}

func (s *DBStore) DemoteTask(ctx context.Context, id string, parentID string) error {
	return database.DemoteTaskWithContext(ctx, id, parentID)
}

func (s *DBStore) MoveTaskToList(ctx context.Context, taskID string, targetTaskListID string) (*database.Task, error) {
	return database.MoveTaskToListWithContext(ctx, taskID, targetTaskListID)
}

func (s *DBStore) DeleteTask(ctx context.Context, id string) error {
	return database.DeleteTaskWithContext(ctx, id)
}

func (s *DBStore) GetSubtasks(ctx context.Context, parentID string) ([]database.Task, error) {
	return database.GetSubtasksWithContext(ctx, parentID)
}

// ── Task Note ─────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTaskNote(ctx context.Context, taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	return database.CreateTaskNoteWithContext(ctx, taskID, noteType, content, authorName, authorID)
}

func (s *DBStore) UpsertTaskNoteByExternal(ctx context.Context, p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	return database.UpsertTaskNoteByExternalWithContext(ctx, p)
}

func (s *DBStore) GetTaskNotes(ctx context.Context, taskID string) ([]database.TaskNote, error) {
	return database.GetTaskNotesWithContext(ctx, taskID)
}

func (s *DBStore) GetTaskNote(ctx context.Context, noteID string) (*database.TaskNote, error) {
	return database.GetTaskNoteWithContext(ctx, noteID)
}

func (s *DBStore) UpdateTaskNote(ctx context.Context, noteID string, content string) error {
	return database.UpdateTaskNoteWithContext(ctx, noteID, content)
}

func (s *DBStore) DeleteTaskNote(ctx context.Context, noteID string) error {
	return database.DeleteTaskNoteWithContext(ctx, noteID)
}
