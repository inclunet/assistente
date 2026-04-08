package tasklist

import "assistente/internal/database"

// DBStore implementa TaskListRepository usando o banco de dados SQLite via GORM.
type DBStore struct{}

// NewDBStore cria um DBStore pronto para uso.
func NewDBStore() *DBStore { return &DBStore{} }

// ── Task List ──────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	return database.CreateTaskList(title, description, templateWorkflow, slug)
}

func (s *DBStore) GetTaskList(id uint) (*database.TaskList, error) {
	return database.GetTaskList(id)
}

func (s *DBStore) GetAllTaskLists() ([]database.TaskList, error) {
	return database.GetAllTaskLists()
}

func (s *DBStore) UpdateTaskList(id uint, title, description string) error {
	return database.UpdateTaskList(id, title, description)
}

func (s *DBStore) UpdateTaskListFull(id uint, title, description, preferredViewMode string, slug *string) error {
	return database.UpdateTaskListFull(id, title, description, preferredViewMode, slug)
}

func (s *DBStore) ResolveTaskListRef(taskListID *uint, taskListSlug string) (uint, error) {
	return database.ResolveTaskListID(taskListID, taskListSlug)
}

func (s *DBStore) SetTaskListValidationPolicy(taskListID uint, policyJSON string) error {
	return database.SetTaskListValidationPolicy(taskListID, policyJSON)
}

func (s *DBStore) SetTaskListViewMode(id uint, viewMode string) error {
	return database.SetTaskListViewMode(id, viewMode)
}

func (s *DBStore) CloneTaskList(id uint, newTitle string) (*database.TaskList, error) {
	return database.CloneTaskList(id, newTitle)
}

func (s *DBStore) ClearTaskList(id uint) error {
	return database.ClearTaskList(id)
}

func (s *DBStore) DeleteTaskList(id uint) error {
	return database.DeleteTaskList(id)
}

func (s *DBStore) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	return database.GetTaskListStats(taskListID)
}

func (s *DBStore) GetTaskListWithHierarchy(id uint) (*database.TaskList, error) {
	return database.GetTaskListWithHierarchy(id)
}

// ── Workflow ───────────────────────────────────────────────────────────────────

func (s *DBStore) GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error) {
	return database.GetWorkflow(taskListID)
}

func (s *DBStore) UpdateWorkflow(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions map[int][]int) error {
	return database.UpdateWorkflow(taskListID, statuses, transitions)
}

func (s *DBStore) UpdateWorkflowFull(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	return database.UpdateWorkflowFull(taskListID, statuses, transitions, initialStatusID, statusMigration)
}

func (s *DBStore) GetTaskCountsByStatus(taskListID uint) (map[int]int64, error) {
	return database.GetTaskCountsByStatus(taskListID)
}

func (s *DBStore) ReorderWorkflowStatuses(taskListID uint, statusOrder []int) error {
	return database.ReorderWorkflowStatuses(taskListID, statusOrder)
}

func (s *DBStore) ValidateStatusTransition(taskListID uint, fromStatusID, toStatusID int) error {
	return database.ValidateStatusTransition(taskListID, fromStatusID, toStatusID)
}

// ── Task ──────────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*database.Task, error) {
	return database.CreateTask(taskListID, title, description, code, link, parentID)
}

func (s *DBStore) CreateTaskFull(taskListID uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *uint) (*database.Task, error) {
	return database.CreateTaskFull(taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
}

func (s *DBStore) GetTask(id uint) (*database.Task, error) {
	return database.GetTask(id)
}

func (s *DBStore) GetTasksByTaskListID(taskListID uint) ([]database.Task, error) {
	return database.GetTasksByTaskListID(taskListID)
}

func (s *DBStore) GetTasksByStatus(taskListID uint, statusID int) ([]database.Task, error) {
	return database.GetTasksByStatus(taskListID, statusID)
}

func (s *DBStore) FindTaskByCode(taskListID uint, code string) (*database.Task, error) {
	return database.FindTaskByCode(taskListID, code)
}

func (s *DBStore) ResolveTaskRef(taskListID *uint, taskListSlug string, taskID *uint, code string) (uint, error) {
	return database.ResolveTaskID(taskListID, taskListSlug, taskID, code)
}

func (s *DBStore) UpdateTask(id uint, title, description, code, link string) error {
	return database.UpdateTask(id, title, description, code, link)
}

func (s *DBStore) UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	return database.UpdateTaskFull(id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
}

func (s *DBStore) UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error {
	return database.UpdateTaskAssignee(id, assigneeName, assigneeID)
}

func (s *DBStore) UpdateTaskStatus(id uint, newStatusID int) error {
	return database.UpdateTaskStatus(id, newStatusID)
}

func (s *DBStore) ReorderTasks(taskListID uint, statusID int, orderedIDs []uint) error {
	return database.ReorderTasks(taskListID, statusID, orderedIDs)
}

func (s *DBStore) PromoteTask(id uint) error {
	return database.PromoteTask(id)
}

func (s *DBStore) DemoteTask(id uint, parentID uint) error {
	return database.DemoteTask(id, parentID)
}

func (s *DBStore) MoveTaskToList(taskID uint, targetTaskListID uint) (*database.Task, error) {
	return database.MoveTaskToList(taskID, targetTaskListID)
}

func (s *DBStore) DeleteTask(id uint) error {
	return database.DeleteTask(id)
}

func (s *DBStore) GetSubtasks(parentID uint) ([]database.Task, error) {
	return database.GetSubtasks(parentID)
}

// ── Task Note ─────────────────────────────────────────────────────────────────

func (s *DBStore) CreateTaskNote(taskID uint, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	return database.CreateTaskNote(taskID, noteType, content, authorName, authorID)
}

func (s *DBStore) UpsertTaskNoteByExternal(p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	return database.UpsertTaskNoteByExternal(p)
}

func (s *DBStore) GetTaskNotes(taskID uint) ([]database.TaskNote, error) {
	return database.GetTaskNotes(taskID)
}

func (s *DBStore) GetTaskNote(noteID uint) (*database.TaskNote, error) {
	return database.GetTaskNote(noteID)
}

func (s *DBStore) UpdateTaskNote(noteID uint, content string) error {
	return database.UpdateTaskNote(noteID, content)
}

func (s *DBStore) DeleteTaskNote(noteID uint) error {
	return database.DeleteTaskNote(noteID)
}
