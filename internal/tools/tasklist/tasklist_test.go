package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"assistente/internal/database"
)

// ==================== Fake Manager ====================

type fakeTaskListManager struct {
	taskLists      map[uint]*database.TaskList
	tasks          map[uint]*database.Task
	workflows      map[uint]*database.TaskListWorkflow
	notes          map[uint][]database.TaskNote
	nextListID     uint
	nextTaskID     uint
	nextNoteID     uint
	createListErr  error
	getListErr     error
	getAllErr      error
	statsErr       error
	createTaskErr  error
	getTaskErr     error
	updateTaskErr  error
	updateStatErr  error
	deleteTaskErr  error
	getWorkflowErr error
	createNoteErr  error
	getNotesErr    error
}

func newFakeManager() *fakeTaskListManager {
	return &fakeTaskListManager{
		taskLists:  make(map[uint]*database.TaskList),
		tasks:      make(map[uint]*database.Task),
		workflows:  make(map[uint]*database.TaskListWorkflow),
		notes:      make(map[uint][]database.TaskNote),
		nextListID: 1,
		nextTaskID: 1,
		nextNoteID: 1,
	}
}

func (f *fakeTaskListManager) addTaskList(title string, statuses []database.TaskListWorkflowStatus) *database.TaskList {
	id := f.nextListID
	f.nextListID++

	statusesJSON, _ := json.Marshal(statuses)
	transitions := database.TaskListWorkflowTransitions{1: {2, 3}, 2: {1, 3}, 3: {1, 2}}
	transitionsJSON, _ := json.Marshal(transitions)

	wf := &database.TaskListWorkflow{
		ID:                 id,
		TaskListID:         id,
		Statuses:           string(statusesJSON),
		AllowedTransitions: string(transitionsJSON),
		InitialStatusID:    1,
	}
	f.workflows[id] = wf

	tl := &database.TaskList{
		ID:       id,
		Title:    title,
		Workflow: wf,
	}
	f.taskLists[id] = tl
	return tl
}

func defaultStatuses() []database.TaskListWorkflowStatus {
	return []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "A Fazer", Color: "var(--color-warning)", Icon: "⌛"},
		{ID: 2, Order: 1, Label: "Em Progresso", Color: "var(--color-info)", Icon: "▶️"},
		{ID: 3, Order: 2, Label: "Concluído", Color: "var(--color-success)", Icon: "✅"},
	}
}

func (f *fakeTaskListManager) addTask(taskListID uint, title string, statusID int) *database.Task {
	id := f.nextTaskID
	f.nextTaskID++
	task := &database.Task{
		ID:         id,
		TaskListID: taskListID,
		Title:      title,
		StatusID:   statusID,
	}
	f.tasks[id] = task

	if tl, ok := f.taskLists[taskListID]; ok {
		tl.Tasks = append(tl.Tasks, *task)
	}
	return task
}

func (f *fakeTaskListManager) CreateTaskList(title, description string, templateWorkflow *database.TaskListWorkflow) (*database.TaskList, error) {
	if f.createListErr != nil {
		return nil, f.createListErr
	}
	tl := f.addTaskList(title, defaultStatuses())
	tl.Description = description
	return tl, nil
}

func (f *fakeTaskListManager) GetTaskList(id uint) (*database.TaskList, error) {
	if f.getListErr != nil {
		return nil, f.getListErr
	}
	tl, ok := f.taskLists[id]
	if !ok {
		return nil, fmt.Errorf("task list not found: %d", id)
	}
	return tl, nil
}

func (f *fakeTaskListManager) GetAllTaskLists() ([]database.TaskList, error) {
	if f.getAllErr != nil {
		return nil, f.getAllErr
	}
	result := make([]database.TaskList, 0, len(f.taskLists))
	for _, tl := range f.taskLists {
		result = append(result, *tl)
	}
	return result, nil
}

func (f *fakeTaskListManager) GetTaskListStats(taskListID uint) (map[string]interface{}, error) {
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	tl, ok := f.taskLists[taskListID]
	if !ok {
		return nil, fmt.Errorf("task list not found: %d", taskListID)
	}
	byStatus := make(map[string]int64)
	for _, task := range tl.Tasks {
		byStatus[fmt.Sprintf("%d", task.StatusID)]++
	}
	return map[string]interface{}{
		"total":    int64(len(tl.Tasks)),
		"byStatus": byStatus,
	}, nil
}

func (f *fakeTaskListManager) CreateTask(taskListID uint, title, description, code, link string, parentID *uint) (*database.Task, error) {
	if f.createTaskErr != nil {
		return nil, f.createTaskErr
	}
	if _, ok := f.taskLists[taskListID]; !ok {
		return nil, fmt.Errorf("task list not found: %d", taskListID)
	}
	wf := f.workflows[taskListID]
	task := f.addTask(taskListID, title, wf.InitialStatusID)
	task.Description = description
	task.Code = code
	task.Link = link
	task.ParentID = parentID
	return task, nil
}

func (f *fakeTaskListManager) CreateTaskFull(taskListID uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *uint) (*database.Task, error) {
	task, err := f.CreateTask(taskListID, title, description, code, link, parentID)
	if err != nil {
		return nil, err
	}
	task.AssigneeName = assigneeName
	task.AssigneeID = assigneeID
	task.CreatorName = creatorName
	task.CreatorID = creatorID
	return task, nil
}

func (f *fakeTaskListManager) GetTask(id uint) (*database.Task, error) {
	if f.getTaskErr != nil {
		return nil, f.getTaskErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task not found: %d", id)
	}
	return task, nil
}

func (f *fakeTaskListManager) FindTaskByCode(taskListID uint, code string) (*database.Task, error) {
	for _, task := range f.tasks {
		if task.TaskListID == taskListID && task.Code == code {
			return task, nil
		}
	}
	return nil, nil
}

func (f *fakeTaskListManager) UpdateTask(id uint, title, description, code, link string) error {
	if f.updateTaskErr != nil {
		return f.updateTaskErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %d", id)
	}
	task.Title = title
	task.Description = description
	task.Code = code
	task.Link = link
	return nil
}

func (f *fakeTaskListManager) UpdateTaskFull(id uint, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	if err := f.UpdateTask(id, title, description, code, link); err != nil {
		return err
	}
	task := f.tasks[id]
	task.AssigneeName = assigneeName
	task.AssigneeID = assigneeID
	task.CreatorName = creatorName
	task.CreatorID = creatorID
	return nil
}

func (f *fakeTaskListManager) UpdateTaskAssignee(id uint, assigneeName, assigneeID string) error {
	task, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %d", id)
	}
	task.AssigneeName = assigneeName
	task.AssigneeID = assigneeID
	return nil
}

func (f *fakeTaskListManager) UpdateTaskStatus(id uint, newStatusID int) error {
	if f.updateStatErr != nil {
		return f.updateStatErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %d", id)
	}
	if task.StatusID == newStatusID {
		return fmt.Errorf("transição de %d para %d não permitida", task.StatusID, newStatusID)
	}
	task.StatusID = newStatusID
	return nil
}

func (f *fakeTaskListManager) DeleteTask(id uint) error {
	if f.deleteTaskErr != nil {
		return f.deleteTaskErr
	}
	if _, ok := f.tasks[id]; !ok {
		return fmt.Errorf("task not found: %d", id)
	}
	delete(f.tasks, id)
	return nil
}

func (f *fakeTaskListManager) GetWorkflow(taskListID uint) (*database.TaskListWorkflow, error) {
	if f.getWorkflowErr != nil {
		return nil, f.getWorkflowErr
	}
	wf, ok := f.workflows[taskListID]
	if !ok {
		return nil, fmt.Errorf("workflow not found for task list: %d", taskListID)
	}
	return wf, nil
}

func (f *fakeTaskListManager) CreateTaskNote(taskID uint, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	if f.createNoteErr != nil {
		return nil, f.createNoteErr
	}
	if _, ok := f.tasks[taskID]; !ok {
		return nil, fmt.Errorf("task not found: %d", taskID)
	}
	id := f.nextNoteID
	f.nextNoteID++
	note := database.TaskNote{
		ID:         id,
		TaskID:     taskID,
		Type:       noteType,
		Content:    content,
		AuthorName: authorName,
		AuthorID:   authorID,
	}
	f.notes[taskID] = append(f.notes[taskID], note)
	return &note, nil
}

func (f *fakeTaskListManager) GetTaskNotes(taskID uint) ([]database.TaskNote, error) {
	if f.getNotesErr != nil {
		return nil, f.getNotesErr
	}
	return f.notes[taskID], nil
}

func (f *fakeTaskListManager) UpdateTaskListFull(id uint, title, description, preferredViewMode string) error {
	tl, ok := f.taskLists[id]
	if !ok {
		return fmt.Errorf("task list not found: %d", id)
	}
	tl.Title = title
	tl.Description = description
	if preferredViewMode == "list" || preferredViewMode == "kanban" {
		tl.PreferredViewMode = preferredViewMode
	}
	return nil
}

func (f *fakeTaskListManager) UpdateWorkflowFull(taskListID uint, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	wf, ok := f.workflows[taskListID]
	if !ok {
		return fmt.Errorf("workflow not found for task list: %d", taskListID)
	}

	statusIDs := make(map[int]bool, len(statuses))
	for _, s := range statuses {
		statusIDs[s.ID] = true
	}

	if !statusIDs[initialStatusID] {
		return fmt.Errorf("initial_status_id %d não existe nos statuses fornecidos", initialStatusID)
	}

	for fromID, toIDs := range transitions {
		if !statusIDs[fromID] {
			return fmt.Errorf("transição referencia status inexistente: %d", fromID)
		}
		for _, toID := range toIDs {
			if !statusIDs[toID] {
				return fmt.Errorf("transição de %d referencia status inexistente: %d", fromID, toID)
			}
		}
	}

	// Check tasks using removed statuses
	for _, task := range f.tasks {
		if task.TaskListID != taskListID {
			continue
		}
		if statusIDs[task.StatusID] {
			continue
		}
		if statusMigration != nil {
			if newID, ok := statusMigration[task.StatusID]; ok {
				if statusIDs[newID] {
					task.StatusID = newID
					continue
				}
			}
		}
		return fmt.Errorf("status_id %d está em uso e não existe nos novos statuses", task.StatusID)
	}

	statusesJSON, _ := json.Marshal(statuses)
	transitionsJSON, _ := json.Marshal(transitions)
	wf.Statuses = string(statusesJSON)
	wf.AllowedTransitions = string(transitionsJSON)
	wf.InitialStatusID = initialStatusID

	// Update tasklist workflow reference
	if tl, ok := f.taskLists[taskListID]; ok {
		tl.Workflow = wf
	}
	return nil
}

func (f *fakeTaskListManager) GetTaskCountsByStatus(taskListID uint) (map[int]int64, error) {
	counts := make(map[int]int64)
	for _, task := range f.tasks {
		if task.TaskListID == taskListID {
			counts[task.StatusID]++
		}
	}
	return counts, nil
}

// ==================== Helper ====================

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ==================== CreateTaskList Tests ====================

func TestCreateTaskList_Name(t *testing.T) {
	tool := NewCreateTaskList(nil)
	if tool.Name() != "create_task_list" {
		t.Fatalf("expected 'create_task_list', got '%s'", tool.Name())
	}
}

func TestCreateTaskList_Success(t *testing.T) {
	mgr := newFakeManager()
	tool := NewCreateTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"title": "My List"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "My List") {
		t.Fatalf("expected content to contain title, got: %s", result.Content)
	}
}

func TestCreateTaskList_EmptyTitle(t *testing.T) {
	mgr := newFakeManager()
	tool := NewCreateTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"title": "  "}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty title")
	}
}

func TestCreateTaskList_InvalidTemplate(t *testing.T) {
	mgr := newFakeManager()
	tool := NewCreateTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"title": "X", "workflow_template": "invalid"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid template")
	}
}

func TestCreateTaskList_SimpleTemplate(t *testing.T) {
	mgr := newFakeManager()
	tool := NewCreateTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"title": "Simple", "workflow_template": "simple"}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
}

func TestCreateTaskList_Error(t *testing.T) {
	mgr := newFakeManager()
	mgr.createListErr = fmt.Errorf("db error")
	tool := NewCreateTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"title": "Test"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error")
	}
}

// ==================== ListTaskLists Tests ====================

func TestListTaskLists_Name(t *testing.T) {
	tool := NewListTaskLists(nil)
	if tool.Name() != "list_task_lists" {
		t.Fatalf("expected 'list_task_lists', got '%s'", tool.Name())
	}
}

func TestListTaskLists_Empty(t *testing.T) {
	mgr := newFakeManager()
	tool := NewListTaskLists(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No task lists") {
		t.Fatalf("expected 'No task lists', got: %s", result.Content)
	}
}

func TestListTaskLists_WithItems(t *testing.T) {
	mgr := newFakeManager()
	mgr.addTaskList("List 1", defaultStatuses())
	mgr.addTaskList("List 2", defaultStatuses())
	tool := NewListTaskLists(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "2 task list") {
		t.Fatalf("expected 2 task lists, got: %s", result.Content)
	}
}

// ==================== GetTaskList Tests ====================

func TestGetTaskList_Name(t *testing.T) {
	tool := NewGetTaskList(nil)
	if tool.Name() != "get_task_list" {
		t.Fatalf("expected 'get_task_list', got '%s'", tool.Name())
	}
}

func TestGetTaskList_Success(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test List", defaultStatuses())
	mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewGetTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": tl.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Test List") {
		t.Fatalf("expected content to contain title, got: %s", result.Content)
	}
}

func TestGetTaskList_NotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewGetTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": 999}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task list")
	}
}

func TestGetTaskList_ZeroID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewGetTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": 0}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for zero ID")
	}
}

// ==================== GetTaskListStatus Tests ====================

func TestGetTaskListStatus_Name(t *testing.T) {
	tool := NewGetTaskListStatus(nil)
	if tool.Name() != "get_task_list_status" {
		t.Fatalf("expected 'get_task_list_status', got '%s'", tool.Name())
	}
}

func TestGetTaskListStatus_Success(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Status Test", defaultStatuses())
	mgr.addTask(tl.ID, "Task 1", 1)
	mgr.addTask(tl.ID, "Task 2", 2)
	tool := NewGetTaskListStatus(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": tl.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Status Test") {
		t.Fatalf("expected content to contain title, got: %s", result.Content)
	}
}

func TestGetTaskListStatus_NotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewGetTaskListStatus(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": 999}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error")
	}
}

// ==================== UpsertTask Tests ====================

func TestUpsertTask_Name(t *testing.T) {
	tool := NewUpsertTask(nil)
	if tool.Name() != "upsert_task" {
		t.Fatalf("expected 'upsert_task', got '%s'", tool.Name())
	}
}

func TestUpsertTask_Create(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "New Task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created' in content, got: %s", result.Content)
	}
}

func TestUpsertTask_Update(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Old Title", 1)
	tool := NewUpsertTask(mgr)

	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "New Title",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated' in content, got: %s", result.Content)
	}
}

func TestUpsertTask_InvalidStatus(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	statusID := 99
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Task",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid status_id")
	}
	if !strings.Contains(result.Content, "invalid status_id") {
		t.Fatalf("expected 'invalid status_id' in content, got: %s", result.Content)
	}
}

func TestUpsertTask_EmptyTitle(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty title")
	}
}

func TestUpsertTask_WithStatus(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "In Progress Task",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
}

// ==================== Dedup by Code Tests ====================

func TestUpsertTask_DedupByCode_CreatesWhenNew(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Dedup", defaultStatuses())
	tool := NewUpsertTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Ticket Alpha",
		"code":         "PROJ-100",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created', got: %s", result.Content)
	}
}

func TestUpsertTask_DedupByCode_UpdatesExisting(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Dedup", defaultStatuses())
	tool := NewUpsertTask(mgr)

	// First call: creates
	result1, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Original Title",
		"code":         "PROJ-200",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result1.Content, "created") {
		t.Fatalf("expected 'created', got: %s", result1.Content)
	}

	// Second call: same code, different title — should update
	result2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Updated Title",
		"code":         "PROJ-200",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result2.Content, "updated") {
		t.Fatalf("expected 'updated', got: %s", result2.Content)
	}

	// Verify only one task exists with that code
	count := 0
	for _, task := range mgr.tasks {
		if task.Code == "PROJ-200" {
			count++
			if task.Title != "Updated Title" {
				t.Errorf("expected title 'Updated Title', got '%s'", task.Title)
			}
		}
	}
	if count != 1 {
		t.Errorf("expected 1 task with code PROJ-200, got %d", count)
	}
}

func TestUpsertTask_DedupByCode_SameCodeDifferentLists(t *testing.T) {
	mgr := newFakeManager()
	tl1 := mgr.addTaskList("List A", defaultStatuses())
	tl2 := mgr.addTaskList("List B", defaultStatuses())
	tool := NewUpsertTask(mgr)

	// Create in list A
	r1, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl1.ID,
		"title":        "Task in A",
		"code":         "PROJ-300",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r1.Content, "created") {
		t.Fatalf("expected 'created' in list A, got: %s", r1.Content)
	}

	// Create in list B with same code — should create (not update A's task)
	r2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl2.ID,
		"title":        "Task in B",
		"code":         "PROJ-300",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(r2.Content, "created") {
		t.Fatalf("expected 'created' in list B, got: %s", r2.Content)
	}

	// Verify two tasks exist with code PROJ-300 in different lists
	count := 0
	for _, task := range mgr.tasks {
		if task.Code == "PROJ-300" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 tasks with code PROJ-300 in different lists, got %d", count)
	}
}

func TestUpsertTask_DedupByCode_EmptyCodeAlwaysCreates(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("No Code", defaultStatuses())
	tool := NewUpsertTask(mgr)

	// Two creates without code — both should create new tasks
	r1, _ := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Task A",
	}))
	r2, _ := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Task B",
	}))

	if !strings.Contains(r1.Content, "created") {
		t.Fatalf("expected first 'created', got: %s", r1.Content)
	}
	if !strings.Contains(r2.Content, "created") {
		t.Fatalf("expected second 'created', got: %s", r2.Content)
	}
}

func TestUpsertTask_DedupByCode_TaskIdTakesPrecedence(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Precedence", defaultStatuses())
	tool := NewUpsertTask(mgr)

	// Create two tasks with different codes
	tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "First",
		"code":         "CODE-A",
	}))
	tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Second",
		"code":         "CODE-B",
	}))

	// Find task with CODE-A
	var taskA *database.Task
	for _, task := range mgr.tasks {
		if task.Code == "CODE-A" {
			taskA = task
			break
		}
	}
	if taskA == nil {
		t.Fatal("task with CODE-A not found")
	}

	// Update by task_id with a different code — task_id should take precedence
	taskID := taskA.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "Updated by ID",
		"code":         "CODE-C",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated', got: %s", result.Content)
	}
	if taskA.Title != "Updated by ID" {
		t.Errorf("expected title 'Updated by ID', got '%s'", taskA.Title)
	}
	if taskA.Code != "CODE-C" {
		t.Errorf("expected code 'CODE-C', got '%s'", taskA.Code)
	}
}

func TestUpsertTask_DedupByCode_UpdatesStatusToo(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Status", defaultStatuses())
	tool := NewUpsertTask(mgr)

	// Create task
	tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Work Item",
		"code":         "WI-500",
	}))

	// Update via code with new status
	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Work Item Updated",
		"code":         "WI-500",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated', got: %s", result.Content)
	}

	// Verify status was updated
	for _, task := range mgr.tasks {
		if task.Code == "WI-500" {
			if task.StatusID != statusID {
				t.Errorf("expected status_id %d, got %d", statusID, task.StatusID)
			}
			break
		}
	}
}

// ==================== DeleteTask Tests ====================

func TestDeleteTask_Name(t *testing.T) {
	tool := NewDeleteTask(nil)
	if tool.Name() != "delete_task" {
		t.Fatalf("expected 'delete_task', got '%s'", tool.Name())
	}
}

func TestDeleteTask_Success(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Doomed Task", 1)
	tool := NewDeleteTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Doomed Task") {
		t.Fatalf("expected task title in content, got: %s", result.Content)
	}
}

func TestDeleteTask_NotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewDeleteTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": 999}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task")
	}
}

func TestDeleteTask_ZeroID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewDeleteTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": 0}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for zero ID")
	}
}

// ==================== AddTaskNote Tests ====================

func TestAddTaskNote_Name(t *testing.T) {
	tool := NewAddTaskNote(nil)
	if tool.Name() != "add_task_note" {
		t.Fatalf("expected 'add_task_note', got '%s'", tool.Name())
	}
}

func TestAddTaskNote_Success(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewAddTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":     task.ID,
		"type":        1,
		"content":     "This is an internal note",
		"author_name": "Test User",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "internal note") {
		t.Fatalf("expected 'internal note' in content, got: %s", result.Content)
	}
}

func TestAddTaskNote_CustomerType(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewAddTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"type":    2,
		"content": "Customer replied",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "customer response") {
		t.Fatalf("expected 'customer response' in content, got: %s", result.Content)
	}
}

func TestAddTaskNote_EmptyContent(t *testing.T) {
	mgr := newFakeManager()
	tool := NewAddTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": 1,
		"type":    1,
		"content": "  ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty content")
	}
}

func TestAddTaskNote_InvalidType(t *testing.T) {
	mgr := newFakeManager()
	tool := NewAddTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": 1,
		"type":    5,
		"content": "note",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid type")
	}
}

func TestAddTaskNote_ZeroTaskID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewAddTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": 0,
		"type":    1,
		"content": "note",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for zero task ID")
	}
}

func TestAddTaskNote_TaskNotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewAddTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": 999,
		"type":    1,
		"content": "note",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task")
	}
}

// ==================== GetTaskNotes Tests ====================

func TestGetTaskNotes_Name(t *testing.T) {
	tool := NewGetTaskNotes(nil)
	if tool.Name() != "get_task_notes" {
		t.Fatalf("expected 'get_task_notes', got '%s'", tool.Name())
	}
}

func TestGetTaskNotes_Empty(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewGetTaskNotes(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "No notes") {
		t.Fatalf("expected 'No notes' in content, got: %s", result.Content)
	}
}

func TestGetTaskNotes_WithNotes(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)

	mgr.CreateTaskNote(task.ID, 1, "First note", "Alice", "")
	mgr.CreateTaskNote(task.ID, 2, "Customer replied", "Bob", "")

	tool := NewGetTaskNotes(mgr)
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "2 note(s)") {
		t.Fatalf("expected '2 note(s)' in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "First note") {
		t.Fatalf("expected 'First note' in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Customer replied") {
		t.Fatalf("expected 'Customer replied' in content, got: %s", result.Content)
	}
}

func TestGetTaskNotes_TaskNotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewGetTaskNotes(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": 999}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task")
	}
}

func TestGetTaskNotes_ZeroID(t *testing.T) {
	mgr := newFakeManager()
	tool := NewGetTaskNotes(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": 0}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for zero ID")
	}
}

// ==================== UpsertTaskList Tests ====================

func TestUpsertTaskList_Name(t *testing.T) {
	tool := NewUpsertTaskList(nil)
	if tool.Name() != "upsert_task_list" {
		t.Fatalf("expected 'upsert_task_list', got '%s'", tool.Name())
	}
}

func TestUpsertTaskList_CreateSimple(t *testing.T) {
	mgr := newFakeManager()
	tool := NewUpsertTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title":       "My Project",
		"description": "A test project",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "My Project") {
		t.Fatalf("expected title in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created' in content, got: %s", result.Content)
	}
}

func TestUpsertTaskList_CreateWithCustomWorkflow(t *testing.T) {
	mgr := newFakeManager()
	tool := NewUpsertTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Jira Mirror",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "Aguardando Suporte", "color": "var(--color-warning)", "icon": "⏳"},
				{"id": 2, "label": "Aguardando Cliente", "color": "var(--color-info)", "icon": "👤"},
				{"id": 3, "label": "Resolvido", "color": "var(--color-success)", "icon": "✅"},
				{"id": 4, "label": "Concluído", "color": "var(--color-success)", "icon": "🏁"},
				{"id": 5, "label": "Cancelado", "color": "var(--color-danger)", "icon": "❌"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{2, 3, 5},
				"2": []int{1, 3, 5},
				"3": []int{4, 1},
				"4": []int{},
				"5": []int{},
			},
			"initial_status_id": 1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Jira Mirror") {
		t.Fatalf("expected title in content, got: %s", result.Content)
	}
}

func TestUpsertTaskList_CreateEmptyTitle(t *testing.T) {
	mgr := newFakeManager()
	tool := NewUpsertTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"title": "  "}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty title")
	}
}

func TestUpsertTaskList_CreateInvalidWorkflow_DuplicateIDs(t *testing.T) {
	mgr := newFakeManager()
	tool := NewUpsertTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Bad",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "A"},
				{"id": 1, "label": "B"},
			},
			"allowed_transitions": map[string]any{"1": []int{}},
			"initial_status_id":   1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for duplicate status IDs")
	}
}

func TestUpsertTaskList_CreateInvalidWorkflow_BadInitialStatus(t *testing.T) {
	mgr := newFakeManager()
	tool := NewUpsertTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Bad",
		"workflow": map[string]any{
			"statuses":            []map[string]any{{"id": 1, "label": "A"}},
			"allowed_transitions": map[string]any{"1": []int{}},
			"initial_status_id":   99,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid initial_status_id")
	}
}

func TestUpsertTaskList_UpdateMetadata(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Old Title", defaultStatuses())
	tool := NewUpsertTaskList(mgr)

	tlID := tl.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tlID,
		"title":        "New Title",
		"description":  "Updated description",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Fatalf("expected 'updated' in content, got: %s", result.Content)
	}
	if mgr.taskLists[tlID].Title != "New Title" {
		t.Errorf("expected title 'New Title', got '%s'", mgr.taskLists[tlID].Title)
	}
}

func TestUpsertTaskList_UpdateWorkflow(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("List", defaultStatuses())
	tool := NewUpsertTaskList(mgr)

	tlID := tl.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tlID,
		"title":        "List",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "Open"},
				{"id": 2, "label": "In Progress"},
				{"id": 3, "label": "Done"},
				{"id": 4, "label": "Cancelled"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{2, 4},
				"2": []int{3, 4},
				"3": []int{},
				"4": []int{},
			},
			"initial_status_id": 1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if mgr.workflows[tlID].InitialStatusID != 1 {
		t.Errorf("expected initial_status_id 1, got %d", mgr.workflows[tlID].InitialStatusID)
	}
}

func TestUpsertTaskList_UpdateWorkflow_RemoveStatusWithMigration(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("List", defaultStatuses())
	mgr.addTask(tl.ID, "Task in Progress", 2)
	tool := NewUpsertTaskList(mgr)

	tlID := tl.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tlID,
		"title":        "List",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "A Fazer"},
				{"id": 3, "label": "Concluído"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{3},
				"3": []int{1},
			},
			"initial_status_id":  1,
			"status_migration": map[string]any{"2": 1},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	for _, task := range mgr.tasks {
		if task.TaskListID == tlID && task.StatusID == 2 {
			t.Error("expected task to be migrated from status 2")
		}
	}
}

func TestUpsertTaskList_UpdateWorkflow_RemoveStatusWithoutMigration_Fails(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("List", defaultStatuses())
	mgr.addTask(tl.ID, "Task in Progress", 2)
	tool := NewUpsertTaskList(mgr)

	tlID := tl.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tlID,
		"title":        "List",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "A Fazer"},
				{"id": 3, "label": "Concluído"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{3},
				"3": []int{1},
			},
			"initial_status_id": 1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when removing status that has tasks without migration")
	}
}

func TestUpsertTaskList_UpdateNotFound(t *testing.T) {
	mgr := newFakeManager()
	tool := NewUpsertTaskList(mgr)

	var id uint = 999
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": id,
		"title":        "Ghost",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task list")
	}
}

func TestUpsertTaskList_CreateWithViewMode(t *testing.T) {
	mgr := newFakeManager()
	tool := NewUpsertTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title":               "Kanban List",
		"preferred_view_mode": "kanban",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created' in content, got: %s", result.Content)
	}
}

// ==================== Assignee Tests ====================

func TestUpsertTask_CreateWithAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	aName := "Alice"
	aID := "alice@example.com"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"title":         "Assigned Task",
		"assignee_name": aName,
		"assignee_id":   aID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Alice") {
		t.Fatalf("expected 'Alice' in result, got: %s", result.Content)
	}

	// Verify assignee was set
	for _, task := range mgr.tasks {
		if task.Title == "Assigned Task" {
			if task.AssigneeName != "Alice" {
				t.Errorf("expected assignee_name 'Alice', got '%s'", task.AssigneeName)
			}
			if task.AssigneeID != "alice@example.com" {
				t.Errorf("expected assignee_id 'alice@example.com', got '%s'", task.AssigneeID)
			}
			break
		}
	}
}

func TestUpsertTask_UpdateAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewUpsertTask(mgr)

	newName := "Bob"
	newID := "bob@example.com"
	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"task_id":       taskID,
		"title":         "Task",
		"assignee_name": newName,
		"assignee_id":   newID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.AssigneeName != "Bob" {
		t.Errorf("expected assignee_name 'Bob', got '%s'", updated.AssigneeName)
	}
	if updated.AssigneeID != "bob@example.com" {
		t.Errorf("expected assignee_id 'bob@example.com', got '%s'", updated.AssigneeID)
	}
}

func TestUpsertTask_ClearAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewUpsertTask(mgr)

	empty := ""
	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"task_id":       taskID,
		"title":         "Task",
		"assignee_name": empty,
		"assignee_id":   empty,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.AssigneeName != "" {
		t.Errorf("expected empty assignee_name, got '%s'", updated.AssigneeName)
	}
	if updated.AssigneeID != "" {
		t.Errorf("expected empty assignee_id, got '%s'", updated.AssigneeID)
	}
}

func TestUpsertTask_CreateWithoutAssignee_NoChange(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "No Assignee Task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	for _, task := range mgr.tasks {
		if task.Title == "No Assignee Task" {
			if task.AssigneeName != "" || task.AssigneeID != "" {
				t.Errorf("expected empty assignee fields for task without assignee params")
			}
			break
		}
	}
}

func TestUpsertTask_UpdatePreservesAssigneeWhenOmitted(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewUpsertTask(mgr)

	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "Updated Title",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.AssigneeName != "Alice" {
		t.Errorf("expected assignee preserved as 'Alice', got '%s'", updated.AssigneeName)
	}
}

func TestUpsertTask_DedupByCode_WithAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	aName := "Alice"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"title":         "Ticket",
		"code":          "FSD-100",
		"assignee_name": aName,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	// Update via code with new assignee
	newName := "Bob"
	result2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"title":         "Ticket Updated",
		"code":          "FSD-100",
		"assignee_name": newName,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result2.Content, "updated") {
		t.Fatalf("expected 'updated', got: %s", result2.Content)
	}

	for _, task := range mgr.tasks {
		if task.Code == "FSD-100" {
			if task.AssigneeName != "Bob" {
				t.Errorf("expected assignee_name 'Bob', got '%s'", task.AssigneeName)
			}
			break
		}
	}
}

// ==================== Creator Tests ====================

func TestUpsertTask_CreateWithCreator(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Ticket from external",
		"creator_name": "Fulano",
		"creator_id":   "user-123",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Fulano") {
		t.Errorf("expected 'Fulano' in result, got: %s", result.Content)
	}

	for _, task := range mgr.tasks {
		if task.Title == "Ticket from external" {
			if task.CreatorName != "Fulano" {
				t.Errorf("expected creator_name 'Fulano', got '%s'", task.CreatorName)
			}
			if task.CreatorID != "user-123" {
				t.Errorf("expected creator_id 'user-123', got '%s'", task.CreatorID)
			}
			break
		}
	}
}

func TestUpsertTask_CreateWithCreatorAndAssignee(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  tl.ID,
		"title":         "Full ticket",
		"creator_name":  "Fulano",
		"creator_id":    "user-123",
		"assignee_name": "Beltrano",
		"assignee_id":   "user-456",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	for _, task := range mgr.tasks {
		if task.Title == "Full ticket" {
			if task.CreatorName != "Fulano" {
				t.Errorf("expected creator_name 'Fulano', got '%s'", task.CreatorName)
			}
			if task.AssigneeName != "Beltrano" {
				t.Errorf("expected assignee_name 'Beltrano', got '%s'", task.AssigneeName)
			}
			break
		}
	}
}

func TestUpsertTask_UpdatePreservesCreatorWhenOmitted(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.CreatorName = "Original"
	task.CreatorID = "orig-id"
	tool := NewUpsertTask(mgr)

	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "Updated Title",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.CreatorName != "Original" {
		t.Errorf("expected creator preserved as 'Original', got '%s'", updated.CreatorName)
	}
}

func TestUpsertTask_ClearCreator(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.CreatorName = "Someone"
	task.CreatorID = "some-id"
	tool := NewUpsertTask(mgr)

	empty := ""
	taskID := task.ID
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      taskID,
		"title":        "Task",
		"creator_name": empty,
		"creator_id":   empty,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	updated := mgr.tasks[taskID]
	if updated.CreatorName != "" {
		t.Errorf("expected empty creator_name, got '%s'", updated.CreatorName)
	}
}

// ==================== TaskNote Author Tests ====================

func TestAddTaskNote_WithStructuredAuthor(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewAddTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":     task.ID,
		"type":        2,
		"content":     "Customer response here",
		"author_name": "Ciclano",
		"author_id":   "user-789",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	notes := mgr.notes[task.ID]
	if len(notes) == 0 {
		t.Fatal("expected at least one note")
	}
	note := notes[0]
	if note.AuthorName != "Ciclano" {
		t.Errorf("expected author_name 'Ciclano', got '%s'", note.AuthorName)
	}
	if note.AuthorID != "user-789" {
		t.Errorf("expected author_id 'user-789', got '%s'", note.AuthorID)
	}
}

func TestAddTaskNote_NoAuthor(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewAddTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"type":    4,
		"content": "System event without author",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}

	notes := mgr.notes[task.ID]
	note := notes[0]
	if note.AuthorName != "" || note.AuthorID != "" {
		t.Errorf("expected author fields empty, got author_name=%q author_id=%q",
			note.AuthorName, note.AuthorID)
	}
}

// ==================== Idempotent upsert_task Tests ====================

func TestUpsertTask_SameStatus_DifferentDescription(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 3
	task.Description = "old description"
	tool := NewUpsertTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"description":  "new description from ADF",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("should not fail when status unchanged but description changed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Errorf("expected action=updated, got: %s", result.Content)
	}
	updated := mgr.tasks[task.ID]
	if updated.Description != "new description from ADF" {
		t.Errorf("expected new description, got: %s", updated.Description)
	}
	if updated.StatusID != 3 {
		t.Errorf("status should remain 3, got: %d", updated.StatusID)
	}
}

func TestUpsertTask_SameStatus_SameFields_Noop(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 3
	task.Description = "same description"
	task.Code = "FSD-100"
	tool := NewUpsertTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"description":  "same description",
		"code":         "FSD-100",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("should not fail for noop: %s", result.Content)
	}
	if !strings.Contains(result.Content, "noop") {
		t.Errorf("expected action=noop, got: %s", result.Content)
	}
}

func TestUpsertTask_DifferentStatus_ValidTransition(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewUpsertTask(mgr)

	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("should succeed for valid status change: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Errorf("expected action=updated, got: %s", result.Content)
	}
	updated := mgr.tasks[task.ID]
	if updated.StatusID != 2 {
		t.Errorf("expected status 2, got: %d", updated.StatusID)
	}
}

func TestUpsertTask_DifferentStatus_InvalidTransition(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	mgr.updateStatErr = fmt.Errorf("transição de 1 para 3 não permitida")
	tool := NewUpsertTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("expected error for invalid transition, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "status change failed") {
		t.Errorf("expected status change failed message, got: %s", result.Content)
	}
}

func TestUpsertTask_Create_StillWorks(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Brand new task",
		"description":  "desc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("create should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Errorf("expected action=created, got: %s", result.Content)
	}
}

func TestUpsertTask_DedupByCode_StillWorks(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewUpsertTask(mgr)

	result1, _ := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "First",
		"code":         "EXT-1",
	}))
	if result1.IsError {
		t.Fatalf("first create should succeed: %s", result1.Content)
	}
	if !strings.Contains(result1.Content, "created") {
		t.Errorf("expected created, got: %s", result1.Content)
	}

	result2, _ := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Updated title",
		"code":         "EXT-1",
		"description":  "new desc",
	}))
	if result2.IsError {
		t.Fatalf("dedup update should succeed: %s", result2.Content)
	}
	if !strings.Contains(result2.Content, "updated") {
		t.Errorf("expected updated via dedup, got: %s", result2.Content)
	}
}

func TestUpsertTask_SameStatus_NoStatusID_UpdatesFields(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.Description = "old"
	tool := NewUpsertTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Ticket",
		"description":  "new description",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("should not fail: %s", result.Content)
	}
	if !strings.Contains(result.Content, "updated") {
		t.Errorf("expected updated, got: %s", result.Content)
	}
	if mgr.tasks[task.ID].Description != "new description" {
		t.Errorf("description not updated")
	}
}
