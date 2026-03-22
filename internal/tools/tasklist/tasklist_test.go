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
	nextListID     uint
	nextTaskID     uint
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
}

func newFakeManager() *fakeTaskListManager {
	return &fakeTaskListManager{
		taskLists:  make(map[uint]*database.TaskList),
		tasks:      make(map[uint]*database.Task),
		workflows:  make(map[uint]*database.TaskListWorkflow),
		nextListID: 1,
		nextTaskID: 1,
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

func (f *fakeTaskListManager) CreateTask(taskListID uint, title, description string, parentID *uint) (*database.Task, error) {
	if f.createTaskErr != nil {
		return nil, f.createTaskErr
	}
	if _, ok := f.taskLists[taskListID]; !ok {
		return nil, fmt.Errorf("task list not found: %d", taskListID)
	}
	wf := f.workflows[taskListID]
	task := f.addTask(taskListID, title, wf.InitialStatusID)
	task.Description = description
	task.ParentID = parentID
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

func (f *fakeTaskListManager) UpdateTask(id uint, title, description string) error {
	if f.updateTaskErr != nil {
		return f.updateTaskErr
	}
	task, ok := f.tasks[id]
	if !ok {
		return fmt.Errorf("task not found: %d", id)
	}
	task.Title = title
	task.Description = description
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

// ==================== BulkUpsertTasks Tests ====================

func TestBulkUpsertTasks_Name(t *testing.T) {
	tool := NewBulkUpsertTasks(nil)
	if tool.Name() != "bulk_upsert_tasks" {
		t.Fatalf("expected 'bulk_upsert_tasks', got '%s'", tool.Name())
	}
}

func TestBulkUpsertTasks_Success(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Bulk Test", defaultStatuses())
	tool := NewBulkUpsertTasks(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"tasks": []map[string]any{
			{"task_list_id": tl.ID, "title": "Task 1"},
			{"task_list_id": tl.ID, "title": "Task 2"},
			{"task_list_id": tl.ID, "title": "Task 3"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "3 succeeded") {
		t.Fatalf("expected '3 succeeded', got: %s", result.Content)
	}
}

func TestBulkUpsertTasks_Empty(t *testing.T) {
	mgr := newFakeManager()
	tool := NewBulkUpsertTasks(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"tasks": []map[string]any{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty tasks array")
	}
}

func TestBulkUpsertTasks_PartialFailure(t *testing.T) {
	mgr := newFakeManager()
	tl := mgr.addTaskList("Bulk", defaultStatuses())
	tool := NewBulkUpsertTasks(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"tasks": []map[string]any{
			{"task_list_id": tl.ID, "title": "Good"},
			{"task_list_id": 999, "title": "Bad"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "1 succeeded") {
		t.Fatalf("expected '1 succeeded', got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "1 failed") {
		t.Fatalf("expected '1 failed', got: %s", result.Content)
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
