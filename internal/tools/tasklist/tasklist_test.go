package tasklist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	tasklistsvc "assistente/internal/tasklist"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ==================== Real-backed Manager Stub ====================

type fakeTaskListManager struct {
	*realTaskListManager
	ctx       context.Context
	db        *gorm.DB
	taskLists map[string]*database.TaskList
	tasks     map[string]*database.Task
	workflows map[string]*database.TaskListWorkflow
	notes     map[string][]database.TaskNote

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

func newFakeManager(t testing.TB) *fakeTaskListManager {
	t.Helper()

	previous := database.DB()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open fake tasklist db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open fake tasklist sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	database.SetDB(db)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		database.SetDB(previous)
	})
	if err := db.AutoMigrate(
		&database.TaskListWorkflow{},
		&database.TaskList{},
		&database.Task{},
		&database.TaskNote{},
	); err != nil {
		t.Fatalf("migrate fake tasklist db: %v", err)
	}
	mgr := &fakeTaskListManager{
		ctx:       database.WithUserID(context.Background(), "tasklist-tool-fake-user"),
		db:        db,
		taskLists: make(map[string]*database.TaskList),
		tasks:     make(map[string]*database.Task),
		workflows: make(map[string]*database.TaskListWorkflow),
		notes:     make(map[string][]database.TaskNote),
	}
	mgr.realTaskListManager = &realTaskListManager{Service: tasklistsvc.NewService(tasklistsvc.ServiceConfig{
		Store:   tasklistsvc.NewDBStore(),
		Emitter: noopTaskListEmitter{},
	})}
	return mgr
}

func (f *fakeTaskListManager) syncDBFromSnapshots() {
	for _, tl := range f.taskLists {
		if tl == nil || tl.ID == "" {
			continue
		}
		_ = f.db.Model(&database.TaskList{}).
			Where("id = ?", tl.ID).
			Select("title", "slug", "description", "preferred_view_mode", "validation_policy", "custom_actions", "conversation_id").
			Updates(map[string]any{
				"title":               tl.Title,
				"slug":                tl.Slug,
				"description":         tl.Description,
				"preferred_view_mode": tl.PreferredViewMode,
				"validation_policy":   tl.ValidationPolicy,
				"custom_actions":      tl.CustomActions,
				"conversation_id":     tl.ConversationID,
			}).Error
	}
	for _, wf := range f.workflows {
		if wf == nil || wf.ID == "" {
			continue
		}
		_ = f.db.Model(&database.TaskListWorkflow{}).
			Where("id = ?", wf.ID).
			Select("statuses", "allowed_transitions", "initial_status_id").
			Updates(map[string]any{
				"statuses":            wf.Statuses,
				"allowed_transitions": wf.AllowedTransitions,
				"initial_status_id":   wf.InitialStatusID,
			}).Error
	}
	for _, task := range f.tasks {
		if task == nil || task.ID == "" {
			continue
		}
		_ = f.db.Model(&database.Task{}).
			Where("id = ?", task.ID).
			Select("task_list_id", "title", "description", "code", "link", "status_id", "parent_id", "assignee_name", "assignee_id", "creator_name", "creator_id", "conversation_id").
			Updates(map[string]any{
				"task_list_id":    task.TaskListID,
				"title":           task.Title,
				"description":     task.Description,
				"code":            task.Code,
				"link":            task.Link,
				"status_id":       task.StatusID,
				"parent_id":       task.ParentID,
				"assignee_name":   task.AssigneeName,
				"assignee_id":     task.AssigneeID,
				"creator_name":    task.CreatorName,
				"creator_id":      task.CreatorID,
				"conversation_id": task.ConversationID,
			}).Error
	}
}

func (f *fakeTaskListManager) refreshSnapshots() {
	var lists []database.TaskList
	_ = f.db.Preload("Workflow").Find(&lists).Error
	seenLists := make(map[string]bool, len(lists))
	for i := range lists {
		tl := lists[i]
		seenLists[tl.ID] = true
		if existing := f.taskLists[tl.ID]; existing != nil {
			*existing = tl
		} else {
			copy := tl
			f.taskLists[tl.ID] = &copy
		}
		if tl.Workflow != nil {
			wf := *tl.Workflow
			if existing := f.workflows[tl.ID]; existing != nil {
				*existing = wf
			} else {
				f.workflows[tl.ID] = &wf
			}
		}
	}
	for id := range f.taskLists {
		if !seenLists[id] {
			delete(f.taskLists, id)
			delete(f.workflows, id)
		}
	}

	var tasks []database.Task
	_ = f.db.Find(&tasks).Error
	seenTasks := make(map[string]bool, len(tasks))
	for i := range tasks {
		task := tasks[i]
		seenTasks[task.ID] = true
		if existing := f.tasks[task.ID]; existing != nil {
			*existing = task
		} else {
			copy := task
			f.tasks[task.ID] = &copy
		}
	}
	for id := range f.tasks {
		if !seenTasks[id] {
			delete(f.tasks, id)
		}
	}

	var notes []database.TaskNote
	_ = f.db.Find(&notes).Error
	f.notes = make(map[string][]database.TaskNote)
	for _, note := range notes {
		f.notes[note.TaskID] = append(f.notes[note.TaskID], note)
	}
}

func (f *fakeTaskListManager) withRealState(fn func() error) error {
	f.syncDBFromSnapshots()
	err := fn()
	f.refreshSnapshots()
	return err
}

func (f *fakeTaskListManager) effectiveCtx(ctx context.Context) context.Context {
	if _, err := database.RequireUserID(ctx); err != nil {
		return f.ctx
	}
	return ctx
}

func workflowTemplate(statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions) *database.TaskListWorkflow {
	statusesJSON, _ := json.Marshal(statuses)
	transitionsJSON, _ := json.Marshal(transitions)
	return &database.TaskListWorkflow{
		Statuses:           string(statusesJSON),
		AllowedTransitions: string(transitionsJSON),
		InitialStatusID:    1,
	}
}

func (f *fakeTaskListManager) addTaskList(title string, statuses []database.TaskListWorkflowStatus) *database.TaskList {
	transitions := database.TaskListWorkflowTransitions{1: {2, 3}, 2: {1, 3}, 3: {1, 2}}
	return f.addTaskListWithTransitions(title, statuses, transitions)
}

func defaultStatuses() []database.TaskListWorkflowStatus {
	return []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "A Fazer", Color: "var(--color-warning)", Icon: "⌛"},
		{ID: 2, Order: 1, Label: "Em Progresso", Color: "var(--color-info)", Icon: "▶️"},
		{ID: 3, Order: 2, Label: "Concluído", Color: "var(--color-success)", Icon: "✅"},
	}
}

func (f *fakeTaskListManager) addTaskListWithTransitions(title string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions) *database.TaskList {
	f.syncDBFromSnapshots()
	tl, err := f.realTaskListManager.CreateTaskList(f.ctx, title, "", workflowTemplate(statuses, transitions), "")
	if err != nil {
		panic(fmt.Sprintf("add task list: %v", err))
	}
	f.refreshSnapshots()
	return f.taskLists[tl.ID]
}

func (f *fakeTaskListManager) addTask(taskListID string, title string, statusID int) *database.Task {
	f.syncDBFromSnapshots()
	task, err := f.realTaskListManager.CreateTaskFull(f.ctx, taskListID, title, "", "", "", "", "", "", "", nil)
	if err != nil {
		panic(fmt.Sprintf("add task: %v", err))
	}
	if statusID != task.StatusID {
		if err := f.db.Model(&database.Task{}).Where("id = ?", task.ID).Update("status_id", statusID).Error; err != nil {
			panic(fmt.Sprintf("set task status fixture: %v", err))
		}
	}
	f.refreshSnapshots()
	return f.tasks[task.ID]
}

func (f *fakeTaskListManager) CreateTaskList(ctx context.Context, title, description string, templateWorkflow *database.TaskListWorkflow, slug string) (*database.TaskList, error) {
	ctx = f.effectiveCtx(ctx)
	if f.createListErr != nil {
		return nil, f.createListErr
	}
	var tl *database.TaskList
	err := f.withRealState(func() error {
		var err error
		tl, err = f.realTaskListManager.CreateTaskList(ctx, title, description, templateWorkflow, slug)
		return err
	})
	if tl != nil {
		return f.taskLists[tl.ID], err
	}
	return nil, err
}

func (f *fakeTaskListManager) GetTaskList(ctx context.Context, id string) (*database.TaskList, error) {
	ctx = f.effectiveCtx(ctx)
	if f.getListErr != nil {
		return nil, f.getListErr
	}
	f.syncDBFromSnapshots()
	tl, err := f.realTaskListManager.GetTaskList(ctx, id)
	f.refreshSnapshots()
	if tl != nil {
		return f.taskLists[tl.ID], err
	}
	return nil, err
}

func (f *fakeTaskListManager) GetAllTaskLists(ctx context.Context) ([]database.TaskList, error) {
	ctx = f.effectiveCtx(ctx)
	if f.getAllErr != nil {
		return nil, f.getAllErr
	}
	f.syncDBFromSnapshots()
	lists, err := f.realTaskListManager.GetAllTaskLists(ctx)
	f.refreshSnapshots()
	return lists, err
}

func (f *fakeTaskListManager) GetTaskListStats(ctx context.Context, taskListID string) (map[string]interface{}, error) {
	ctx = f.effectiveCtx(ctx)
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	f.syncDBFromSnapshots()
	stats, err := f.realTaskListManager.GetTaskListStats(ctx, taskListID)
	f.refreshSnapshots()
	return stats, err
}

func (f *fakeTaskListManager) CreateTask(ctx context.Context, taskListID string, title, description, code, link string, parentID *string) (*database.Task, error) {
	ctx = f.effectiveCtx(ctx)
	if f.createTaskErr != nil {
		return nil, f.createTaskErr
	}
	var task *database.Task
	err := f.withRealState(func() error {
		var err error
		task, err = f.realTaskListManager.CreateTask(ctx, taskListID, title, description, code, link, parentID)
		return err
	})
	if task != nil {
		return f.tasks[task.ID], err
	}
	return nil, err
}

func (f *fakeTaskListManager) CreateTaskFull(ctx context.Context, taskListID string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string, parentID *string) (*database.Task, error) {
	ctx = f.effectiveCtx(ctx)
	if f.createTaskErr != nil {
		return nil, f.createTaskErr
	}
	var task *database.Task
	err := f.withRealState(func() error {
		var err error
		task, err = f.realTaskListManager.CreateTaskFull(ctx, taskListID, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID, parentID)
		return err
	})
	if task != nil {
		return f.tasks[task.ID], err
	}
	return nil, err
}

func (f *fakeTaskListManager) GetTask(ctx context.Context, id string) (*database.Task, error) {
	ctx = f.effectiveCtx(ctx)
	if f.getTaskErr != nil {
		return nil, f.getTaskErr
	}
	f.syncDBFromSnapshots()
	task, err := f.realTaskListManager.GetTask(ctx, id)
	f.refreshSnapshots()
	if task != nil {
		return f.tasks[task.ID], err
	}
	return nil, err
}

func (f *fakeTaskListManager) FindTaskByCode(ctx context.Context, taskListID string, code string) (*database.Task, error) {
	ctx = f.effectiveCtx(ctx)
	f.syncDBFromSnapshots()
	task, err := f.realTaskListManager.FindTaskByCode(ctx, taskListID, code)
	f.refreshSnapshots()
	if task != nil {
		return f.tasks[task.ID], err
	}
	return nil, err
}

func (f *fakeTaskListManager) UpdateTask(ctx context.Context, id string, title, description, code, link string) error {
	ctx = f.effectiveCtx(ctx)
	if f.updateTaskErr != nil {
		return f.updateTaskErr
	}
	return f.withRealState(func() error {
		return f.realTaskListManager.UpdateTask(ctx, id, title, description, code, link)
	})
}

func (f *fakeTaskListManager) UpdateTaskFull(ctx context.Context, id string, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID string) error {
	ctx = f.effectiveCtx(ctx)
	if f.updateTaskErr != nil {
		return f.updateTaskErr
	}
	return f.withRealState(func() error {
		return f.realTaskListManager.UpdateTaskFull(ctx, id, title, description, code, link, assigneeName, assigneeID, creatorName, creatorID)
	})
}

func (f *fakeTaskListManager) UpdateTaskAssignee(ctx context.Context, id string, assigneeName, assigneeID string) error {
	ctx = f.effectiveCtx(ctx)
	return f.withRealState(func() error {
		return f.realTaskListManager.UpdateTaskAssignee(ctx, id, assigneeName, assigneeID)
	})
}

func (f *fakeTaskListManager) SetTaskConversation(ctx context.Context, id string, conversationID *string) error {
	ctx = f.effectiveCtx(ctx)
	return f.withRealState(func() error {
		return f.realTaskListManager.SetTaskConversation(ctx, id, conversationID)
	})
}

func (f *fakeTaskListManager) UpdateTaskStatus(ctx context.Context, id string, newStatusID int) error {
	ctx = f.effectiveCtx(ctx)
	if f.updateStatErr != nil {
		return f.updateStatErr
	}
	return f.withRealState(func() error {
		return f.realTaskListManager.UpdateTaskStatus(ctx, id, newStatusID)
	})
}

func (f *fakeTaskListManager) DeleteTask(ctx context.Context, id string) error {
	ctx = f.effectiveCtx(ctx)
	if f.deleteTaskErr != nil {
		return f.deleteTaskErr
	}
	return f.withRealState(func() error {
		return f.realTaskListManager.DeleteTask(ctx, id)
	})
}

func (f *fakeTaskListManager) MoveTaskToList(ctx context.Context, taskID string, targetTaskListID string) (*database.Task, error) {
	ctx = f.effectiveCtx(ctx)
	var task *database.Task
	err := f.withRealState(func() error {
		var err error
		task, err = f.realTaskListManager.MoveTaskToList(ctx, taskID, targetTaskListID)
		return err
	})
	if task != nil {
		return f.tasks[task.ID], err
	}
	return nil, err
}

func (f *fakeTaskListManager) GetWorkflow(ctx context.Context, taskListID string) (*database.TaskListWorkflow, error) {
	ctx = f.effectiveCtx(ctx)
	if f.getWorkflowErr != nil {
		return nil, f.getWorkflowErr
	}
	f.syncDBFromSnapshots()
	wf, err := f.realTaskListManager.GetWorkflow(ctx, taskListID)
	f.refreshSnapshots()
	if wf != nil {
		return f.workflows[taskListID], err
	}
	return nil, err
}

func (f *fakeTaskListManager) CreateTaskNote(ctx context.Context, taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	ctx = f.effectiveCtx(ctx)
	if f.createNoteErr != nil {
		return nil, f.createNoteErr
	}
	var note *database.TaskNote
	err := f.withRealState(func() error {
		var err error
		note, err = f.realTaskListManager.CreateTaskNote(ctx, taskID, noteType, content, authorName, authorID)
		return err
	})
	if note != nil {
		notes := f.notes[note.TaskID]
		for i := range notes {
			if notes[i].ID == note.ID {
				return &notes[i], err
			}
		}
	}
	return note, err
}

func (f *fakeTaskListManager) UpsertTaskNoteByExternal(ctx context.Context, p database.UpsertTaskNoteByExternalParams) (*database.TaskNote, bool, error) {
	ctx = f.effectiveCtx(ctx)
	if f.createNoteErr != nil {
		return nil, false, f.createNoteErr
	}
	var note *database.TaskNote
	var created bool
	err := f.withRealState(func() error {
		var err error
		note, created, err = f.realTaskListManager.UpsertTaskNoteByExternal(ctx, p)
		return err
	})
	if note != nil {
		notes := f.notes[note.TaskID]
		for i := range notes {
			if notes[i].ID == note.ID {
				return &notes[i], created, err
			}
		}
	}
	return note, created, err
}

func (f *fakeTaskListManager) UpdateTaskNote(ctx context.Context, noteID string, content string) error {
	ctx = f.effectiveCtx(ctx)
	return f.withRealState(func() error {
		return f.realTaskListManager.UpdateTaskNote(ctx, noteID, content)
	})
}

func (f *fakeTaskListManager) GetTaskNotes(ctx context.Context, taskID string) ([]database.TaskNote, error) {
	ctx = f.effectiveCtx(ctx)
	if f.getNotesErr != nil {
		return nil, f.getNotesErr
	}
	f.syncDBFromSnapshots()
	notes, err := f.realTaskListManager.GetTaskNotes(ctx, taskID)
	f.refreshSnapshots()
	return notes, err
}

func (f *fakeTaskListManager) GetTaskNote(ctx context.Context, noteID string) (*database.TaskNote, error) {
	ctx = f.effectiveCtx(ctx)
	f.syncDBFromSnapshots()
	note, err := f.realTaskListManager.GetTaskNote(ctx, noteID)
	f.refreshSnapshots()
	if note != nil {
		notes := f.notes[note.TaskID]
		for i := range notes {
			if notes[i].ID == note.ID {
				return &notes[i], err
			}
		}
	}
	return nil, err
}

func (f *fakeTaskListManager) UpdateTaskListFull(ctx context.Context, id string, title, description, preferredViewMode string, slug *string) error {
	ctx = f.effectiveCtx(ctx)
	return f.withRealState(func() error {
		return f.realTaskListManager.UpdateTaskListFull(ctx, id, title, description, preferredViewMode, slug)
	})
}

func (f *fakeTaskListManager) SetTaskListConversation(ctx context.Context, id string, conversationID *string) error {
	ctx = f.effectiveCtx(ctx)
	return f.withRealState(func() error {
		return f.realTaskListManager.SetTaskListConversation(ctx, id, conversationID)
	})
}

func (f *fakeTaskListManager) ResolveTaskListRef(ctx context.Context, taskListID *string, taskListSlug string) (string, error) {
	ctx = f.effectiveCtx(ctx)
	f.syncDBFromSnapshots()
	id, err := f.realTaskListManager.ResolveTaskListRef(ctx, taskListID, taskListSlug)
	f.refreshSnapshots()
	return id, err
}

func (f *fakeTaskListManager) ResolveTaskRef(ctx context.Context, taskListID *string, taskListSlug string, taskID *string, code string) (string, error) {
	ctx = f.effectiveCtx(ctx)
	f.syncDBFromSnapshots()
	id, err := f.realTaskListManager.ResolveTaskRef(ctx, taskListID, taskListSlug, taskID, code)
	f.refreshSnapshots()
	return id, err
}

func (f *fakeTaskListManager) ResolveTaskIDByTaskCode(ctx context.Context, taskListID *string, taskCode string) (string, error) {
	ctx = f.effectiveCtx(ctx)
	f.syncDBFromSnapshots()
	id, err := f.realTaskListManager.ResolveTaskIDByTaskCode(ctx, taskListID, taskCode)
	f.refreshSnapshots()
	return id, err
}

func (f *fakeTaskListManager) SetTaskListValidationPolicy(ctx context.Context, id string, policyJSON string) error {
	ctx = f.effectiveCtx(ctx)
	return f.withRealState(func() error {
		return f.realTaskListManager.SetTaskListValidationPolicy(ctx, id, policyJSON)
	})
}

func (f *fakeTaskListManager) GetTaskListCustomActions(ctx context.Context, id string) (*database.TaskListCustomActions, error) {
	ctx = f.effectiveCtx(ctx)
	f.syncDBFromSnapshots()
	actions, err := f.realTaskListManager.GetTaskListCustomActions(ctx, id)
	f.refreshSnapshots()
	return actions, err
}

func (f *fakeTaskListManager) SetTaskListCustomActions(ctx context.Context, id string, actionsJSON string) error {
	ctx = f.effectiveCtx(ctx)
	return f.withRealState(func() error {
		return f.realTaskListManager.SetTaskListCustomActions(ctx, id, actionsJSON)
	})
}

func (f *fakeTaskListManager) UpdateWorkflowFull(ctx context.Context, taskListID string, statuses []database.TaskListWorkflowStatus, transitions database.TaskListWorkflowTransitions, initialStatusID int, statusMigration map[int]int) error {
	ctx = f.effectiveCtx(ctx)
	return f.withRealState(func() error {
		return f.realTaskListManager.UpdateWorkflowFull(ctx, taskListID, statuses, transitions, initialStatusID, statusMigration)
	})
}

func (f *fakeTaskListManager) GetTaskCountsByStatus(ctx context.Context, taskListID string) (map[int]int64, error) {
	ctx = f.effectiveCtx(ctx)
	f.syncDBFromSnapshots()
	counts, err := f.realTaskListManager.GetTaskCountsByStatus(ctx, taskListID)
	f.refreshSnapshots()
	return counts, err
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

type noopTaskListEmitter struct{}

func (noopTaskListEmitter) Emit(string, any) {}

type realTaskListManager struct {
	*tasklistsvc.Service
}

func (m *realTaskListManager) CreateTaskNote(ctx context.Context, taskID string, noteType database.TaskNoteType, content, authorName, authorID string) (*database.TaskNote, error) {
	return m.Service.CreateTaskNote(ctx, taskID, int(noteType), content, authorName, authorID)
}

type realTaskListFixture struct {
	ctx  context.Context
	mgr  *realTaskListManager
	tool *TaskListTool
}

func newRealTaskListFixture(t *testing.T) realTaskListFixture {
	t.Helper()

	previous := database.DB()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open tasklist test db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("open tasklist sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	database.SetDB(db)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		database.SetDB(previous)
	})
	if err := db.AutoMigrate(
		&database.TaskListWorkflow{},
		&database.TaskList{},
		&database.Task{},
		&database.TaskNote{},
	); err != nil {
		t.Fatalf("migrate tasklist test db: %v", err)
	}

	mgr := &realTaskListManager{Service: tasklistsvc.NewService(tasklistsvc.ServiceConfig{
		Store:   tasklistsvc.NewDBStore(),
		Emitter: noopTaskListEmitter{},
	})}
	return realTaskListFixture{
		ctx:  database.WithUserID(context.Background(), "tasklist-tool-test-user"),
		mgr:  mgr,
		tool: NewTaskList(mgr),
	}
}

func TestTaskListManagerContract_ResolvesSlugAndTaskCode(t *testing.T) {
	defaultTransitions := database.TaskListWorkflowTransitions{1: {2, 3}, 2: {1, 3}, 3: {1, 2}}

	cases := []struct {
		name  string
		setup func(t *testing.T) (context.Context, TaskListManager)
	}{
		{
			name: "real service sqlite",
			setup: func(t *testing.T) (context.Context, TaskListManager) {
				t.Helper()
				fixture := newRealTaskListFixture(t)
				return fixture.ctx, fixture.mgr
			},
		},
		{
			name: "error-injection stub",
			setup: func(t *testing.T) (context.Context, TaskListManager) {
				t.Helper()
				mgr := newFakeManager(t)
				return mgr.ctx, mgr
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, mgr := tc.setup(t)
			workflow := workflowTemplate(defaultStatuses(), defaultTransitions)
			bugs, err := mgr.CreateTaskList(ctx, "Bugs", "", workflow, "bugs")
			if err != nil {
				t.Fatalf("CreateTaskList bugs: %v", err)
			}
			task, err := mgr.CreateTaskFull(ctx, bugs.ID, "Fix login", "", "FSD-99", "", "", "", "", "", nil)
			if err != nil {
				t.Fatalf("CreateTaskFull: %v", err)
			}

			gotListID, err := mgr.ResolveTaskListRef(ctx, nil, "bugs")
			if err != nil {
				t.Fatalf("ResolveTaskListRef by slug: %v", err)
			}
			if gotListID != bugs.ID {
				t.Fatalf("ResolveTaskListRef = %s, want %s", gotListID, bugs.ID)
			}

			gotTaskID, err := mgr.ResolveTaskRef(ctx, nil, "bugs", nil, "FSD-99")
			if err != nil {
				t.Fatalf("ResolveTaskRef by slug+code: %v", err)
			}
			if gotTaskID != task.ID {
				t.Fatalf("ResolveTaskRef = %s, want %s", gotTaskID, task.ID)
			}

			gotTaskID, err = mgr.ResolveTaskIDByTaskCode(ctx, nil, "FSD-99")
			if err != nil {
				t.Fatalf("ResolveTaskIDByTaskCode unique: %v", err)
			}
			if gotTaskID != task.ID {
				t.Fatalf("ResolveTaskIDByTaskCode = %s, want %s", gotTaskID, task.ID)
			}

			other, err := mgr.CreateTaskList(ctx, "Other", "", workflowTemplate(defaultStatuses(), defaultTransitions), "other")
			if err != nil {
				t.Fatalf("CreateTaskList other: %v", err)
			}
			if _, err := mgr.CreateTaskFull(ctx, other.ID, "Same external id", "", "FSD-99", "", "", "", "", "", nil); err != nil {
				t.Fatalf("CreateTaskFull duplicate code: %v", err)
			}
			if _, err := mgr.ResolveTaskIDByTaskCode(ctx, nil, "FSD-99"); err == nil {
				t.Fatal("ResolveTaskIDByTaskCode without list should reject ambiguous code")
			}
			gotTaskID, err = mgr.ResolveTaskIDByTaskCode(ctx, &bugs.ID, "FSD-99")
			if err != nil {
				t.Fatalf("ResolveTaskIDByTaskCode scoped: %v", err)
			}
			if gotTaskID != task.ID {
				t.Fatalf("scoped ResolveTaskIDByTaskCode = %s, want %s", gotTaskID, task.ID)
			}
		})
	}
}

// ==================== GetTaskList Tests (consolidated: list all, full details, summary) ====================

func TestGetTaskList_ParametersValidJSON(t *testing.T) {
	tool := NewTaskList(nil)
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() is not valid JSON: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["custom_actions"]; !ok {
		t.Fatalf("expected 'custom_actions' in tool parameters schema")
	}
}

func TestGetTaskList_Name(t *testing.T) {
	tool := NewTaskList(nil)
	if tool.Name() != "task_list" {
		t.Fatalf("expected 'task_list', got '%s'", tool.Name())
	}
}

func TestGetTaskList_ListAll_Empty(t *testing.T) {
	fixture := newRealTaskListFixture(t)

	result, err := fixture.tool.Execute(fixture.ctx, mustMarshal(t, map[string]any{}))
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

func TestGetTaskList_ListAll_WithItems(t *testing.T) {
	fixture := newRealTaskListFixture(t)
	if _, err := fixture.mgr.CreateTaskList(fixture.ctx, "List 1", "", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.mgr.CreateTaskList(fixture.ctx, "List 2", "", nil, ""); err != nil {
		t.Fatal(err)
	}

	result, err := fixture.tool.Execute(fixture.ctx, mustMarshal(t, map[string]any{}))
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

func TestGetTaskList_FullDetails(t *testing.T) {
	fixture := newRealTaskListFixture(t)
	tl, err := fixture.mgr.CreateTaskList(fixture.ctx, "Test List", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.mgr.CreateTask(fixture.ctx, tl.ID, "Task 1", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	if task, err := fixture.mgr.CreateTask(fixture.ctx, tl.ID, "Task 2", "", "", "", nil); err != nil {
		t.Fatal(err)
	} else if err := fixture.mgr.UpdateTaskStatus(fixture.ctx, task.ID, 2); err != nil {
		t.Fatal(err)
	}

	result, err := fixture.tool.Execute(fixture.ctx, mustMarshal(t, map[string]any{"task_list_id": tl.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Test List") {
		t.Fatalf("expected content to contain title, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "summary") {
		t.Fatalf("expected content to contain summary stats, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "\"total\"") {
		t.Fatalf("expected content to contain total count, got: %s", result.Content)
	}
}

func TestGetTaskList_NotFound(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": "999"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task list")
	}
}

func TestTaskList_ConversationLink(t *testing.T) {
	fixture := newRealTaskListFixture(t)

	// Cria lista já vinculada a uma conversa, com descrição.
	createRes, err := fixture.tool.Execute(fixture.ctx, mustMarshal(t, map[string]any{
		"title":           "Linked List",
		"description":     "important desc",
		"conversation_id": "conv-1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if createRes.IsError {
		t.Fatalf("unexpected error creating linked list: %s", createRes.Content)
	}
	if !strings.Contains(createRes.Content, "conv-1") {
		t.Fatalf("expected conversation_id in create output, got: %s", createRes.Content)
	}

	createdID, _ := createRes.Metadata["task_list_id"].(string)
	if createdID == "" {
		t.Fatalf("expected created task_list_id metadata, got: %#v", createRes.Metadata)
	}
	created, err := fixture.mgr.GetTaskList(fixture.ctx, createdID)
	if err != nil {
		t.Fatalf("created list not found in real manager: %v", err)
	}
	if created.ConversationID == nil || *created.ConversationID != "conv-1" {
		t.Fatalf("expected list linked to conv-1, got: %v", created.ConversationID)
	}

	// Update apenas-do-vínculo não pode sobrescrever title/description.
	updRes, err := fixture.tool.Execute(fixture.ctx, mustMarshal(t, map[string]any{
		"task_list_id":    createdID,
		"conversation_id": "conv-2",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updRes.IsError {
		t.Fatalf("unexpected error on conversation-only update: %s", updRes.Content)
	}
	created, err = fixture.mgr.GetTaskList(fixture.ctx, createdID)
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "Linked List" || created.Description != "important desc" {
		t.Fatalf("conversation-only update should preserve title/description, got: %q / %q", created.Title, created.Description)
	}
	if created.ConversationID == nil || *created.ConversationID != "conv-2" {
		t.Fatalf("expected list re-linked to conv-2, got: %v", created.ConversationID)
	}

	// Limpar o vínculo passando string vazia.
	clrRes, err := fixture.tool.Execute(fixture.ctx, mustMarshal(t, map[string]any{
		"task_list_id":    createdID,
		"conversation_id": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if clrRes.IsError {
		t.Fatalf("unexpected error clearing conversation link: %s", clrRes.Content)
	}
	created, err = fixture.mgr.GetTaskList(fixture.ctx, createdID)
	if err != nil {
		t.Fatal(err)
	}
	if created.ConversationID != nil {
		t.Fatalf("expected conversation link cleared, got: %v", *created.ConversationID)
	}
}

func TestTaskList_DuplicateConversationInheritance(t *testing.T) {
	fixture := newRealTaskListFixture(t)
	conv := "conv-src"
	src, err := fixture.mgr.CreateTaskList(fixture.ctx, "Source", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.mgr.SetTaskListConversation(fixture.ctx, src.ID, &conv); err != nil {
		t.Fatal(err)
	}

	// Duplicação sem conversation_id herda o vínculo da origem.
	inheritRes, err := fixture.tool.Execute(fixture.ctx, mustMarshal(t, map[string]any{
		"task_list_id": src.ID,
		"duplicate":    true,
		"title":        "Copy Inherits",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if inheritRes.IsError {
		t.Fatalf("unexpected duplicate error: %s", inheritRes.Content)
	}
	inheritedID, _ := inheritRes.Metadata["task_list_id"].(string)
	inherited, err := fixture.mgr.GetTaskList(fixture.ctx, inheritedID)
	if err != nil {
		t.Fatalf("inherited copy not found: %v", err)
	}
	if inherited.ConversationID == nil || *inherited.ConversationID != conv {
		t.Fatalf("expected duplicate to inherit conv-src, got: %v", inherited.ConversationID)
	}

	// Duplicação com conversation_id explícito sobrescreve a herança.
	overrideRes, err := fixture.tool.Execute(fixture.ctx, mustMarshal(t, map[string]any{
		"task_list_id":    src.ID,
		"duplicate":       true,
		"title":           "Copy Override",
		"conversation_id": "conv-other",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if overrideRes.IsError {
		t.Fatalf("unexpected duplicate error: %s", overrideRes.Content)
	}
	overrideID, _ := overrideRes.Metadata["task_list_id"].(string)
	override, err := fixture.mgr.GetTaskList(fixture.ctx, overrideID)
	if err != nil {
		t.Fatalf("override copy not found: %v", err)
	}
	if override.ConversationID == nil || *override.ConversationID != "conv-other" {
		t.Fatalf("expected duplicate to override with conv-other, got: %v", override.ConversationID)
	}
}

func TestGetTaskList_ZeroID(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_list_id": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty ID")
	}
}

func TestGetTaskList_SummaryOnly(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Status Test", defaultStatuses())
	mgr.addTask(tl.ID, "Task 1", 1)
	mgr.addTask(tl.ID, "Task 2", 2)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"summary_only": true,
	}))
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

func TestGetTaskList_SummaryOnly_NotFound(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": "999",
		"summary_only": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error")
	}
}

func TestGetTaskList_SummaryOnly_WithoutID_Error(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"summary_only": true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when summary_only without task_list_id")
	}
	if !strings.Contains(result.Content, "summary_only requires task_list_id or task_list_slug") {
		t.Errorf("expected summary_only ref error, got: %s", result.Content)
	}
}

func TestTask_ReadNoNotes(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Task 1") {
		t.Fatalf("expected task title in content, got: %s", result.Content)
	}
	if strings.Contains(result.Content, "notes") {
		t.Fatalf("expected no notes key in output when empty, got: %s", result.Content)
	}
}

func TestTask_ReadWithNotes(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)

	mgr.CreateTaskNote(context.Background(), task.ID, 1, "First note", "Alice", "")     //nolint:errcheck
	mgr.CreateTaskNote(context.Background(), task.ID, 2, "Customer replied", "Bob", "") //nolint:errcheck

	tool := NewTask(mgr)
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Task 1") {
		t.Fatalf("expected task title in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "First note") {
		t.Fatalf("expected 'First note' in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Customer replied") {
		t.Fatalf("expected 'Customer replied' in content, got: %s", result.Content)
	}
}

func TestTask_ReadIncludesFields(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Detailed Task", 1)
	task.Description = "Some description"
	task.Code = "FSD-123"
	task.Link = "https://example.com"
	task.AssigneeName = "Alice"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	for _, expected := range []string{"Detailed Task", "Some description", "FSD-123", "https://example.com", "Alice"} {
		if !strings.Contains(result.Content, expected) {
			t.Errorf("expected '%s' in content, got: %s", expected, result.Content)
		}
	}
}

func TestTask_ConversationLink(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

	// Cria task já vinculada a uma conversa.
	createRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":    tl.ID,
		"title":           "Linked task",
		"conversation_id": "conv-123",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if createRes.IsError {
		t.Fatalf("unexpected error creating linked task: %s", createRes.Content)
	}
	if !strings.Contains(createRes.Content, "conv-123") {
		t.Fatalf("expected conversation_id in create output, got: %s", createRes.Content)
	}

	var created *database.Task
	for _, tk := range mgr.tasks {
		if tk.Title == "Linked task" {
			created = tk
		}
	}
	if created == nil {
		t.Fatal("created task not found in fake manager")
	}
	if created.ConversationID == nil || *created.ConversationID != "conv-123" {
		t.Fatalf("expected task linked to conv-123, got: %v", created.ConversationID)
	}

	// Atualização somente do vínculo (sem title) deve preservar os demais campos.
	updRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":         created.ID,
		"conversation_id": "conv-456",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if updRes.IsError {
		t.Fatalf("unexpected error on conversation-only update: %s", updRes.Content)
	}
	if created.Title != "Linked task" {
		t.Fatalf("conversation-only update should preserve title, got: %q", created.Title)
	}
	if created.ConversationID == nil || *created.ConversationID != "conv-456" {
		t.Fatalf("expected task re-linked to conv-456, got: %v", created.ConversationID)
	}

	// Limpar o vínculo passando string vazia.
	clrRes, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":         created.ID,
		"conversation_id": "",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if clrRes.IsError {
		t.Fatalf("unexpected error clearing conversation link: %s", clrRes.Content)
	}
	if created.ConversationID != nil {
		t.Fatalf("expected conversation link cleared, got: %v", *created.ConversationID)
	}
}

func TestTask_ReadNotFound(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": "999"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task")
	}
}

func TestTask_ReadZeroID(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": ""}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty task ID")
	}
	if !strings.Contains(result.Content, "informe task_id ou code") {
		t.Errorf("expected task ref error, got: %s", result.Content)
	}
}

func TestTask_ReadByListSlugAndCode(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Bugs", defaultStatuses())
	tl.Slug = "bugs"
	task := mgr.addTask(tl.ID, "Fix it", 1)
	task.Code = "FSD-99"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_slug": "bugs",
		"code":           "FSD-99",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Fix it") {
		t.Errorf("expected title in response, got: %s", result.Content)
	}
}

// ==================== UpsertTask Tests ====================

func TestUpsertTask_Name(t *testing.T) {
	tool := NewTask(nil)
	if tool.Name() != "task" {
		t.Fatalf("expected 'upsert_task', got '%s'", tool.Name())
	}
}

func TestUpsertTask_Create(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Old Title", 1)
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Dedup", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Dedup", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl1 := mgr.addTaskList("List A", defaultStatuses())
	tl2 := mgr.addTaskList("List B", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("No Code", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Precedence", defaultStatuses())
	tool := NewTask(mgr)

	// Create two tasks with different codes
	_, _ = tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "First",
		"code":         "CODE-A",
	}))
	_, _ = tool.Execute(context.Background(), mustMarshal(t, map[string]any{
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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Status", defaultStatuses())
	tool := NewTask(mgr)

	// Create task
	_, _ = tool.Execute(context.Background(), mustMarshal(t, map[string]any{
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

func TestUpsertTask_DeleteSuccess(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Doomed Task", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"delete":       true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if !strings.Contains(result.Content, "deleted") {
		t.Fatalf("expected 'deleted' in content, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Doomed Task") {
		t.Fatalf("expected task title in content, got: %s", result.Content)
	}
}

func TestUpsertTask_DeleteNotFound(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": "1",
		"task_id":      "999",
		"delete":       true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent task")
	}
}

func TestUpsertTask_DeleteWithoutTaskID_Error(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": "1",
		"delete":       true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when delete without task_id")
	}
	if !strings.Contains(result.Content, "informe task_id ou code") {
		t.Errorf("expected task ref error, got: %s", result.Content)
	}
}

func TestUpsertTask_DeleteAndDuplicate_Error(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"delete":       true,
		"duplicate":    true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when delete and duplicate combined")
	}
	if !strings.Contains(result.Content, "cannot be used together") {
		t.Errorf("expected 'cannot be used together', got: %s", result.Content)
	}
}

// ==================== UpsertTaskNote Tests ====================

func TestUpsertTaskNote_Name(t *testing.T) {
	tool := NewTaskNote(nil)
	if tool.Name() != "task_note" {
		t.Fatalf("expected 'task_note', got '%s'", tool.Name())
	}
}

func TestUpsertTaskNote_CreateSuccess(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

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
	if !strings.Contains(result.Content, "created") {
		t.Fatalf("expected 'created' in content, got: %s", result.Content)
	}
}

func TestUpsertTaskNote_CreateWithListSlugAndCode(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Bugs", defaultStatuses())
	tl.Slug = "bugs"
	task := mgr.addTask(tl.ID, "Task", 1)
	task.Code = "J-1"
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_slug": "bugs",
		"code":           "J-1",
		"type":           1,
		"content":        "via slug+code",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 || notes[0].Content != "via slug+code" {
		t.Fatalf("expected one note on task, got %+v", notes)
	}
}

func TestUpsertTaskNote_CreateCustomerType(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

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

func TestUpsertTaskNote_UpdateSuccess(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)

	note, _ := mgr.CreateTaskNote(context.Background(), task.ID, 1, "Original content", "Alice", "")
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"note_id": note.ID,
		"content": "Updated content",
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
	updated, _ := mgr.GetTaskNote(context.Background(), note.ID)
	if updated.Content != "Updated content" {
		t.Errorf("expected content 'Updated content', got '%s'", updated.Content)
	}
}

func TestUpsertTaskNote_UpdateNotFound(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"note_id": "999",
		"content": "Updated content",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent note")
	}
}

func TestUpsertTaskNote_UpdateWrongTask(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task1 := mgr.addTask(tl.ID, "Task 1", 1)
	task2 := mgr.addTask(tl.ID, "Task 2", 1)

	note, _ := mgr.CreateTaskNote(context.Background(), task1.ID, 1, "Note on task 1", "Alice", "")
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task2.ID,
		"note_id": note.ID,
		"content": "Trying to update via wrong task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when note does not belong to task")
	}
	if !strings.Contains(result.Content, "does not match") {
		t.Errorf("expected mismatch error, got: %s", result.Content)
	}
}

func TestUpsertTaskNote_EmptyContent(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
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

func TestUpsertTaskNote_InvalidType(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
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

func TestUpsertTaskNote_ZeroTaskID(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": "",
		"type":    1,
		"content": "note",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty task ID")
	}
	if !strings.Contains(result.Content, "informe task_id ou code") {
		t.Errorf("expected task ref error, got: %s", result.Content)
	}
}

func TestUpsertTaskNote_TaskNotFound(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": "999",
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

func TestUpsertTaskNote_CreateWithoutType_Error(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task 1", 1)
	tool := NewTaskNote(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID,
		"content": "note without type",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when type not provided for create")
	}
	if !strings.Contains(result.Content, "type is required") {
		t.Errorf("expected 'type is required', got: %s", result.Content)
	}
}

func TestUpsertTaskNote_ExternalIdempotentTwice(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

	base := map[string]any{
		"task_id":             task.ID,
		"type":                2,
		"source":              "jira",
		"external_id":         "comment-98765",
		"external_parent_id":  "FSD-123",
		"author_name":         "Fulano",
		"author_id":           "abc",
		"content":             "Comentário vindo do Jira",
		"external_updated_at": "2026-04-08T12:00:00Z",
	}
	r1, err := tool.Execute(context.Background(), mustMarshal(t, base))
	if err != nil || r1.IsError {
		t.Fatalf("first upsert: %v %s", err, r1.Content)
	}
	id1 := metadataNoteID(t, r1.Metadata)
	if id1 == "" {
		t.Fatalf("expected note_id in metadata: %#v", r1.Metadata)
	}
	if r1.Metadata["action"] != "created" {
		t.Fatalf("expected created, got %#v", r1.Metadata)
	}

	base["content"] = "Comentário vindo do Jira (editado)"
	r2, err := tool.Execute(context.Background(), mustMarshal(t, base))
	if err != nil || r2.IsError {
		t.Fatalf("second upsert: %v %s", err, r2.Content)
	}
	id2 := metadataNoteID(t, r2.Metadata)
	if id1 != id2 {
		t.Fatalf("expected same note id, got %v then %v", id1, id2)
	}
	if r2.Metadata["action"] != "updated" {
		t.Fatalf("expected updated, got %#v", r2.Metadata)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Content != "Comentário vindo do Jira (editado)" {
		t.Fatalf("content: %q", notes[0].Content)
	}
}

func TestUpsertTaskNote_ExternalUpdateWithoutType(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

	_, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID, "type": 1, "content": "v1",
		"source": "jira", "external_id": "c1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID, "content": "v2",
		"source": "jira", "external_id": "c1",
	}))
	if err != nil || r2.IsError {
		t.Fatalf("update without type: %v %s", err, r2.Content)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 || notes[0].Content != "v2" {
		t.Fatalf("notes: %+v", notes)
	}
	if notes[0].Type != database.TaskNoteInternal {
		t.Fatalf("type should stay internal, got %d", notes[0].Type)
	}
}

func TestUpsertTaskNote_ExternalRequiresBothKeys(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task.ID, "type": 1, "content": "x",
		"source": "jira",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(r.Content, "both source and external_id") {
		t.Fatalf("expected both keys error, got: %s", r.Content)
	}
}

func TestUpsertTaskNote_ExternalConflictDifferentTask(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task1 := mgr.addTask(tl.ID, "T1", 1)
	task2 := mgr.addTask(tl.ID, "T2", 1)
	tool := NewTaskNote(mgr)

	_, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task1.ID, "type": 1, "content": "on t1",
		"source": "jira", "external_id": "same",
	}))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id": task2.ID, "type": 1, "content": "on t2",
		"source": "jira", "external_id": "same",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r2.IsError || !strings.Contains(r2.Content, "já existe") {
		t.Fatalf("expected conflict error, got: %s", r2.Content)
	}
}

func TestUpsertTaskNote_ByTaskCode_ManualCreate(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task, err := mgr.CreateTask(context.Background(), tl.ID, "Issue", "", "FSD-12345", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_code": "FSD-12345",
		"type":      2,
		"content":   "nota via code",
	}))
	if err != nil || r.IsError {
		t.Fatalf("execute: %v %s", err, r.Content)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 || notes[0].Content != "nota via code" {
		t.Fatalf("notes: %+v", notes)
	}
}

func TestUpsertTaskNote_ExternalByTaskCode_Idempotent(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task, err := mgr.CreateTask(context.Background(), tl.ID, "Issue", "", "FSD-12345", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tool := NewTaskNote(mgr)

	base := map[string]any{
		"task_code":           "FSD-12345",
		"type":                2,
		"source":              "jira",
		"external_id":         "10001",
		"external_parent_id":  "FSD-12345",
		"external_updated_at": "2026-04-08T19:00:00Z",
		"author_name":         "Jane Doe",
		"content":             "Comentário sincronizado",
	}
	r1, err := tool.Execute(context.Background(), mustMarshal(t, base))
	if err != nil || r1.IsError {
		t.Fatalf("first: %v %s", err, r1.Content)
	}
	id1 := metadataNoteID(t, r1.Metadata)
	base["content"] = "Comentário sincronizado (v2)"
	r2, err := tool.Execute(context.Background(), mustMarshal(t, base))
	if err != nil || r2.IsError {
		t.Fatalf("second: %v %s", err, r2.Content)
	}
	id2 := metadataNoteID(t, r2.Metadata)
	if id1 != id2 {
		t.Fatalf("expected same note id")
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), task.ID)
	if len(notes) != 1 || notes[0].Content != "Comentário sincronizado (v2)" {
		t.Fatalf("notes: %+v", notes)
	}
}

func TestUpsertTaskNote_TaskCodeNotFound(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	_, _ = mgr.CreateTask(context.Background(), tl.ID, "Issue", "", "OTHER", "", nil)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_code": "NOPE-1",
		"type":      1,
		"content":   "x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(r.Content, "nenhuma task com task_code") {
		t.Fatalf("expected not found: %s", r.Content)
	}
}

func TestUpsertTaskNote_TaskCodeAmbiguous(t *testing.T) {
	mgr := newFakeManager(t)
	tl1 := mgr.addTaskList("A", defaultStatuses())
	tl2 := mgr.addTaskList("B", defaultStatuses())
	_, _ = mgr.CreateTask(context.Background(), tl1.ID, "t", "", "SAME", "", nil)
	_, _ = mgr.CreateTask(context.Background(), tl2.ID, "t", "", "SAME", "", nil)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_code": "SAME",
		"type":      1,
		"content":   "x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(r.Content, "várias tasks com task_code") {
		t.Fatalf("expected ambiguous: %s", r.Content)
	}
}

func TestUpsertTaskNote_TaskCodeWithListSlug_Disambiguates(t *testing.T) {
	mgr := newFakeManager(t)
	tl1 := mgr.addTaskList("A", defaultStatuses())
	tl1.Slug = "lista-a"
	tl2 := mgr.addTaskList("B", defaultStatuses())
	tl2.Slug = "lista-b"
	taskB, _ := mgr.CreateTask(context.Background(), tl2.ID, "t", "", "KEY", "", nil)
	_, _ = mgr.CreateTask(context.Background(), tl1.ID, "t", "", "KEY", "", nil)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_slug": "lista-b",
		"task_code":      "KEY",
		"type":           1,
		"content":        "scoped",
	}))
	if err != nil || r.IsError {
		t.Fatalf("execute: %v %s", err, r.Content)
	}
	notes, _ := mgr.GetTaskNotes(context.Background(), taskB.ID)
	if len(notes) != 1 || notes[0].Content != "scoped" {
		t.Fatalf("expected note on task B, notes=%+v", notes)
	}
}

func TestUpsertTaskNote_TaskIDWithMismatchedTaskCode(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task, _ := mgr.CreateTask(context.Background(), tl.ID, "Issue", "", "FSD-1", "", nil)
	tool := NewTaskNote(mgr)

	r, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_id":   task.ID,
		"task_code": "OTHER",
		"type":      1,
		"content":   "x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !r.IsError || !strings.Contains(r.Content, "não coincide") {
		t.Fatalf("expected mismatch: %s", r.Content)
	}
}

func TestTask_ReadNotesIncludeExternalFields(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	ts := time.Date(2026, 4, 8, 15, 30, 0, 0, time.UTC)
	_, _, _ = mgr.UpsertTaskNoteByExternal(context.Background(), database.UpsertTaskNoteByExternalParams{
		TaskID:            task.ID,
		Type:              ptrTaskNoteType(database.TaskNoteCustomer),
		Content:           "synced",
		ExternalSource:    "jira",
		ExternalID:        "c-1",
		ExternalParentID:  "FSD-9",
		ExternalUpdatedAt: &ts,
	})

	tool := NewTask(mgr)
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{"task_id": task.ID}))
	if err != nil || result.IsError {
		t.Fatalf("read: %v %s", err, result.Content)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatal(err)
	}
	rawNotes, ok := payload["notes"].([]any)
	if !ok || len(rawNotes) != 1 {
		t.Fatalf("notes: %#v", payload["notes"])
	}
	n := rawNotes[0].(map[string]any)
	if n["source"] != "jira" || n["external_id"] != "c-1" || n["external_parent_id"] != "FSD-9" {
		t.Fatalf("unexpected note fields: %#v", n)
	}
	if n["external_updated_at"] != ts.Format(time.RFC3339) {
		t.Fatalf("external_updated_at: %#v", n["external_updated_at"])
	}
}

func ptrTaskNoteType(t database.TaskNoteType) *database.TaskNoteType {
	return &t
}

func metadataNoteID(t *testing.T, md map[string]any) string {
	t.Helper()
	v, ok := md["note_id"]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		t.Fatalf("unexpected note_id type %T in metadata", v)
		return ""
	}
}

// ==================== UpsertTaskList Tests ====================

func TestUpsertTaskList_Name(t *testing.T) {
	tool := NewTaskList(nil)
	if tool.Name() != "task_list" {
		t.Fatalf("expected 'upsert_task_list', got '%s'", tool.Name())
	}
}

func TestUpsertTaskList_CreateSimple(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

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

func TestUpsertTaskList_CreateWithCustomActions(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Suporte",
		"custom_actions": []map[string]any{
			{
				"id":       "refresh",
				"label":    "Atualizar",
				"icon":     "🔄",
				"surfaces": []string{"card_menu", "board_menu"},
				"event":    "tasklist.card.refresh",
			},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	id, _ := result.Metadata["task_list_id"].(string)
	if id == "" {
		t.Fatalf("expected task_list_id in metadata, got: %#v", result.Metadata)
	}
	tl := mgr.taskLists[id]
	ca, err := database.ParseTaskListCustomActionsJSON(tl.CustomActions)
	if err != nil {
		t.Fatalf("stored custom_actions invalid: %v (raw=%s)", err, tl.CustomActions)
	}
	if len(ca.Actions) != 1 || ca.Actions[0].ID != "refresh" {
		t.Fatalf("expected one action 'refresh', got: %#v", ca.Actions)
	}
	if !strings.Contains(result.Content, "custom_actions") {
		t.Fatalf("expected custom_actions echoed in content, got: %s", result.Content)
	}
}

func TestUpsertTaskList_UpdateCustomActionsClear(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Suporte", defaultStatuses())
	tl.CustomActions = `{"actions":[{"id":"refresh","label":"Atualizar","event":"tasklist.card.refresh"}]}`
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":   tl.ID,
		"custom_actions": []map[string]any{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if mgr.taskLists[tl.ID].CustomActions != "" {
		t.Fatalf("expected custom_actions cleared, got: %q", mgr.taskLists[tl.ID].CustomActions)
	}
}

func TestUpsertTaskList_CustomActionsInvalid(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title": "Suporte",
		"custom_actions": []map[string]any{
			{"id": "bad id", "label": "X", "event": "tasklist.card.refresh"},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("expected error for invalid action id, got: %s", result.Content)
	}
}

// TestUpsertTaskList_CustomActionsObjectRejected garante que custom_actions
// como objeto ({}) é rejeitado com erro, em vez de ser tratado como "limpar"
// (que mascararia um tipo errado como data-loss). Só [] limpa.
func TestUpsertTaskList_CustomActionsObjectRejected(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Suporte", defaultStatuses())
	tl.CustomActions = `{"actions":[{"id":"refresh","label":"Atualizar","event":"tasklist.card.refresh"}]}`
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":   tl.ID,
		"custom_actions": map[string]any{},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("expected error for custom_actions as object {}, got: %s", result.Content)
	}
	if mgr.taskLists[tl.ID].CustomActions == "" {
		t.Fatalf("custom_actions should NOT be cleared by an invalid object {}")
	}
}

// TestGetTaskList_BySlugDoesNotMutate garante que uma chamada de leitura apenas
// com task_list_slug retorna os detalhes da lista SEM cair no caminho de
// escrita (que sobrescreveria description/view_mode com vazio). Regressão do
// bug em que task_list_slug sozinho era tratado como write.
func TestGetTaskList_BySlugDoesNotMutate(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Suporte", defaultStatuses())
	tl.Slug = "bugs"
	tl.Description = "descrição importante"
	tl.PreferredViewMode = "kanban"
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_slug": "bugs",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content)
	}
	if action, ok := result.Metadata["action"]; ok {
		t.Fatalf("read-by-slug should not be a write, got action=%v", action)
	}
	got := mgr.taskLists[tl.ID]
	if got.Description != "descrição importante" {
		t.Fatalf("description was mutated by read-by-slug: %q", got.Description)
	}
	if got.PreferredViewMode != "kanban" {
		t.Fatalf("preferred_view_mode was mutated by read-by-slug: %q", got.PreferredViewMode)
	}
	if id, _ := result.Metadata["task_list_id"].(string); id != tl.ID {
		t.Fatalf("expected full details for %s, got metadata: %#v", tl.ID, result.Metadata)
	}
}

// TestCustomActionsToList_InvalidSurfacesError garante que JSON corrompido em
// custom_actions não some silenciosamente do echo: expõe um marcador de erro,
// coerente com validationPolicyToMap.
func TestCustomActionsToList_InvalidSurfacesError(t *testing.T) {
	// vazio -> omitido (nil)
	if out := customActionsToList("   "); out != nil {
		t.Fatalf("empty should be nil, got: %#v", out)
	}
	// lista vazia válida -> omitido (nil)
	if out := customActionsToList(`{"actions":[]}`); out != nil {
		t.Fatalf("empty actions should be nil, got: %#v", out)
	}
	// JSON inválido -> marcador de erro
	out := customActionsToList(`{"actions":[{"id":"x"`)
	if len(out) != 1 {
		t.Fatalf("invalid JSON should yield one marker entry, got: %#v", out)
	}
	if pe, _ := out[0]["_parse_error"].(bool); !pe {
		t.Fatalf("expected _parse_error marker, got: %#v", out[0])
	}
	if _, ok := out[0]["raw"]; !ok {
		t.Fatalf("expected raw in parse error marker, got: %#v", out[0])
	}
	// campo desconhecido (parser estrito) -> também é marcador de erro
	out2 := customActionsToList(`{"actions":[{"id":"x","label":"X","emits_event":"e"}]}`)
	if len(out2) != 1 {
		t.Fatalf("unknown field should yield marker, got: %#v", out2)
	}
	if pe, _ := out2[0]["_parse_error"].(bool); !pe {
		t.Fatalf("expected _parse_error marker for unknown field, got: %#v", out2[0])
	}
}

func TestUpsertTaskList_CreateWithCustomWorkflow(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

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
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"title":       "  ",
		"description": "forces write mode",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty title")
	}
}

func TestUpsertTaskList_CreateInvalidWorkflow_DuplicateIDs(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

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
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Old Title", defaultStatuses())
	tool := NewTaskList(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("List", defaultStatuses())
	tool := NewTaskList(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("List", defaultStatuses())
	mgr.addTask(tl.ID, "Task in Progress", 2)
	tool := NewTaskList(mgr)

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
			"status_migration":  map[string]any{"2": 1},
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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("List", defaultStatuses())
	mgr.addTask(tl.ID, "Task in Progress", 2)
	tool := NewTaskList(mgr)

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
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	id := "999"
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
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

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

// ==================== Duplicate TaskList Tests ====================

func TestUpsertTaskList_DuplicateBasic(t *testing.T) {
	mgr := newFakeManager(t)
	source := mgr.addTaskList("Source List", defaultStatuses())
	mgr.taskLists[source.ID].Description = "source description"
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Copy of Source",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "duplicated") {
		t.Errorf("expected action duplicated, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Copy of Source") {
		t.Errorf("expected title in result, got: %s", result.Content)
	}

	// Verify a new list was created (not the source)
	found := false
	for _, tl := range mgr.taskLists {
		if tl.ID != source.ID && tl.Title == "Copy of Source" {
			found = true
			if tl.Description != "source description" {
				t.Errorf("expected description inherited, got %q", tl.Description)
			}
			break
		}
	}
	if !found {
		t.Fatal("duplicate task list not found")
	}
}

func TestUpsertTaskList_DuplicateInheritsWorkflow(t *testing.T) {
	mgr := newFakeManager(t)
	customStatuses := []database.TaskListWorkflowStatus{
		{ID: 10, Order: 0, Label: "Novo"},
		{ID: 20, Order: 1, Label: "Andamento"},
		{ID: 30, Order: 2, Label: "Feito"},
	}
	source := mgr.addTaskList("Custom WF", customStatuses)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Copy WF",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}

	// Find the new list and verify its workflow matches source
	for _, tl := range mgr.taskLists {
		if tl.ID != source.ID && tl.Title == "Copy WF" {
			if tl.Workflow == nil {
				t.Fatal("expected workflow on duplicated list")
			}
			if tl.Workflow.InitialStatusID != source.Workflow.InitialStatusID {
				t.Errorf("expected initial_status_id=%d, got %d", source.Workflow.InitialStatusID, tl.Workflow.InitialStatusID)
			}
			return
		}
	}
	t.Fatal("duplicate task list not found")
}

func TestUpsertTaskList_DuplicateOverridesDescription(t *testing.T) {
	mgr := newFakeManager(t)
	source := mgr.addTaskList("Source", defaultStatuses())
	mgr.taskLists[source.ID].Description = "original desc"
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Copy",
		"description":  "overridden desc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}

	for _, tl := range mgr.taskLists {
		if tl.ID != source.ID && tl.Title == "Copy" {
			if tl.Description != "overridden desc" {
				t.Errorf("expected overridden description, got %q", tl.Description)
			}
			return
		}
	}
	t.Fatal("duplicate task list not found")
}

func TestUpsertTaskList_DuplicateOverridesWorkflow(t *testing.T) {
	mgr := newFakeManager(t)
	source := mgr.addTaskList("Source", defaultStatuses())
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Copy Custom WF",
		"workflow": map[string]any{
			"statuses": []map[string]any{
				{"id": 1, "label": "Open"},
				{"id": 2, "label": "Closed"},
			},
			"allowed_transitions": map[string]any{
				"1": []int{2},
				"2": []int{1},
			},
			"initial_status_id": 1,
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate with workflow override should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "duplicated") {
		t.Errorf("expected duplicated, got: %s", result.Content)
	}
}

func TestUpsertTaskList_DuplicateSourceNotFound(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	ghostID := "9999"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": ghostID,
		"duplicate":    true,
		"title":        "Ghost copy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent source")
	}
	if !strings.Contains(result.Content, "não encontrado") {
		t.Errorf("expected not-found error, got: %s", result.Content)
	}
}

func TestUpsertTaskList_DuplicateWithoutTaskListID_Error(t *testing.T) {
	mgr := newFakeManager(t)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"duplicate": true,
		"title":     "No source",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when duplicate is true without task_list_id")
	}
	if !strings.Contains(result.Content, "duplicate requires task_list_id or task_list_slug") {
		t.Errorf("expected duplicate ref error, got: %s", result.Content)
	}
}

func TestUpsertTaskList_DuplicateDoesNotCopyTasks(t *testing.T) {
	mgr := newFakeManager(t)
	source := mgr.addTaskList("Source", defaultStatuses())
	mgr.addTask(source.ID, "Task 1", 1)
	mgr.addTask(source.ID, "Task 2", 2)
	tool := NewTaskList(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": source.ID,
		"duplicate":    true,
		"title":        "Empty Copy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}

	// Find the new list and verify it has no tasks
	for _, tl := range mgr.taskLists {
		if tl.ID != source.ID && tl.Title == "Empty Copy" {
			taskCount := 0
			for _, task := range mgr.tasks {
				if task.TaskListID == tl.ID {
					taskCount++
				}
			}
			if taskCount != 0 {
				t.Errorf("expected 0 tasks in duplicated list, got %d", taskCount)
			}
			return
		}
	}
	t.Fatal("duplicate task list not found")
}

// ==================== Assignee Tests ====================

func TestUpsertTask_CreateWithAssignee(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.AssigneeName = "Alice"
	task.AssigneeID = "alice@example.com"
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.CreatorName = "Original"
	task.CreatorID = "orig-id"
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	task.CreatorName = "Someone"
	task.CreatorID = "some-id"
	tool := NewTask(mgr)

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

// ==================== UpsertTaskNote Author Tests ====================

func TestUpsertTaskNote_CreateWithStructuredAuthor(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

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

func TestUpsertTaskNote_CreateNoAuthor(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Task", 1)
	tool := NewTaskNote(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 3
	task.Description = "old description"
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 3
	task.Description = "same description"
	task.Code = "FSD-100"
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	statuses := defaultStatuses()
	transitions := database.TaskListWorkflowTransitions{
		1: {2},
		2: {1, 3},
	}
	tl := mgr.addTaskListWithTransitions("Test", statuses, transitions)
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	tool := NewTask(mgr)

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
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.Description = "old"
	tool := NewTask(mgr)

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

// ==================== Workflow Transition Tests ====================

func TestUpsertTask_TransitionToTerminalStatus(t *testing.T) {
	mgr := newFakeManager(t)
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
		{ID: 4, Order: 3, Label: "Cancelado"},
		{ID: 5, Order: 4, Label: "Abandonado"},
	}
	transitions := database.TaskListWorkflowTransitions{
		1: {2, 4, 5},
		2: {1, 3, 4, 5},
	}
	tl := mgr.addTaskListWithTransitions("Jira-like", statuses, transitions)
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 2
	tool := NewTask(mgr)

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
	if result.IsError {
		t.Fatalf("transition to terminal status should succeed: %s", result.Content)
	}
	if mgr.tasks[task.ID].StatusID != 3 {
		t.Errorf("expected status 3, got %d", mgr.tasks[task.ID].StatusID)
	}
}

func TestUpsertTask_TransitionFromZeroStatus(t *testing.T) {
	mgr := newFakeManager(t)
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
	}
	transitions := database.TaskListWorkflowTransitions{
		1: {2, 3},
		2: {1, 3},
	}
	tl := mgr.addTaskListWithTransitions("Test", statuses, transitions)
	task := mgr.addTask(tl.ID, "Orphan task", 0)
	task.StatusID = 0
	tool := NewTask(mgr)

	statusID := 2
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"task_id":      task.ID,
		"title":        "Orphan task",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("transition from status 0 (invalid) to valid status should succeed: %s", result.Content)
	}
	if mgr.tasks[task.ID].StatusID != 2 {
		t.Errorf("expected status 2, got %d", mgr.tasks[task.ID].StatusID)
	}
}

func TestUpsertTask_SameStatusNoop_NoUpdateTaskStatusCall(t *testing.T) {
	mgr := newFakeManager(t)
	tl := mgr.addTaskList("Test", defaultStatuses())
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 2
	tool := NewTask(mgr)

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
		t.Fatalf("same status should not fail: %s", result.Content)
	}
	if !strings.Contains(result.Content, "noop") {
		t.Errorf("expected noop for same status and same fields, got: %s", result.Content)
	}
}

func TestUpsertTask_InvalidDestinationStatus(t *testing.T) {
	mgr := newFakeManager(t)
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Done"},
	}
	transitions := database.TaskListWorkflowTransitions{1: {2}}
	tl := mgr.addTaskListWithTransitions("Test", statuses, transitions)
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewTask(mgr)

	statusID := 99
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
		t.Fatal("expected error for non-existent destination status")
	}
	if !strings.Contains(result.Content, "invalid status_id") {
		t.Errorf("expected 'invalid status_id' message, got: %s", result.Content)
	}
}

func TestUpsertTask_TransitionNotAllowedByWorkflow(t *testing.T) {
	mgr := newFakeManager(t)
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
	}
	transitions := database.TaskListWorkflowTransitions{
		1: {2},
		2: {3},
	}
	tl := mgr.addTaskListWithTransitions("Strict", statuses, transitions)
	task := mgr.addTask(tl.ID, "Ticket", 1)
	task.StatusID = 1
	tool := NewTask(mgr)

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
		t.Fatalf("expected error: 1->3 is not allowed, only 1->2, got: %s", result.Content)
	}
	if !strings.Contains(result.Content, "status change failed") {
		t.Errorf("expected 'status change failed', got: %s", result.Content)
	}
}

func TestUpsertTask_CreateWithTerminalStatus(t *testing.T) {
	mgr := newFakeManager(t)
	statuses := []database.TaskListWorkflowStatus{
		{ID: 1, Order: 0, Label: "Backlog"},
		{ID: 2, Order: 1, Label: "Em Progresso"},
		{ID: 3, Order: 2, Label: "Concluído"},
	}
	transitions := database.TaskListWorkflowTransitions{
		1: {2, 3},
		2: {3},
	}
	tl := mgr.addTaskListWithTransitions("Test", statuses, transitions)
	tool := NewTask(mgr)

	statusID := 3
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": tl.ID,
		"title":        "Already done",
		"status_id":    statusID,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("creating task with terminal status should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "created") {
		t.Errorf("expected created, got: %s", result.Content)
	}
}

// ==================== Move Task Tests ====================

func TestUpsertTask_MoveToAnotherList(t *testing.T) {
	mgr := newFakeManager(t)
	listA := mgr.addTaskList("List A", defaultStatuses())
	listB := mgr.addTaskList("List B", defaultStatuses())
	task := mgr.addTask(listA.ID, "My task", 1)
	task.Description = "original desc"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listB.ID,
		"task_id":      task.ID,
		"title":        "My task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("move should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "moved") {
		t.Errorf("expected action moved, got: %s", result.Content)
	}
	if mgr.tasks[task.ID].TaskListID != listB.ID {
		t.Errorf("expected task_list_id=%s, got %s", listB.ID, mgr.tasks[task.ID].TaskListID)
	}
}

func TestUpsertTask_MoveSameList_Noop(t *testing.T) {
	mgr := newFakeManager(t)
	listA := mgr.addTaskList("List A", defaultStatuses())
	task := mgr.addTask(listA.ID, "My task", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listA.ID,
		"task_id":      task.ID,
		"title":        "My task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("same-list update should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "noop") {
		t.Errorf("expected noop (no move, same fields), got: %s", result.Content)
	}
}

func TestUpsertTask_MoveAndUpdateFields(t *testing.T) {
	mgr := newFakeManager(t)
	listA := mgr.addTaskList("List A", defaultStatuses())
	listB := mgr.addTaskList("List B", defaultStatuses())
	task := mgr.addTask(listA.ID, "Old title", 1)
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listB.ID,
		"task_id":      task.ID,
		"title":        "New title",
		"description":  "new desc",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("move+update should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "moved") {
		t.Errorf("expected action moved, got: %s", result.Content)
	}
	if mgr.tasks[task.ID].Title != "New title" {
		t.Errorf("title not updated")
	}
	if mgr.tasks[task.ID].Description != "new desc" {
		t.Errorf("description not updated")
	}
}

func TestUpsertTask_MoveResetsStatus(t *testing.T) {
	mgr := newFakeManager(t)
	listA := mgr.addTaskList("List A", defaultStatuses())
	listB := mgr.addTaskList("List B", defaultStatuses())
	task := mgr.addTask(listA.ID, "Task", 1)
	task.StatusID = 2 // Em Progresso
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listB.ID,
		"task_id":      task.ID,
		"title":        "Task",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("move should succeed: %s", result.Content)
	}
	// MoveTaskToList resets status to initial (1)
	if mgr.tasks[task.ID].StatusID != 1 {
		t.Errorf("expected status reset to 1 after move, got %d", mgr.tasks[task.ID].StatusID)
	}
}

// ==================== Duplicate Task Tests ====================

func TestUpsertTask_DuplicateSameList(t *testing.T) {
	mgr := newFakeManager(t)
	list := mgr.addTaskList("List", defaultStatuses())
	source := mgr.addTask(list.ID, "Source task", 1)
	source.Description = "source desc"
	source.Link = "https://example.com"
	source.AssigneeName = "Alice"
	source.AssigneeID = "alice@example.com"
	source.CreatorName = "Bob"
	source.CreatorID = "bob@example.com"
	source.Code = "JIRA-123"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": list.ID,
		"task_id":      source.ID,
		"duplicate":    true,
		"title":        "Copy of Source",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate should succeed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "duplicated") {
		t.Errorf("expected action duplicated, got: %s", result.Content)
	}

	// Find the new task (not source)
	var newTask *database.Task
	for _, tk := range mgr.tasks {
		if tk.ID != source.ID && tk.Title == "Copy of Source" {
			newTask = tk
			break
		}
	}
	if newTask == nil {
		t.Fatal("duplicate task not found")
	}
	if newTask.TaskListID != list.ID {
		t.Errorf("expected task_list_id=%s, got %s", list.ID, newTask.TaskListID)
	}
	if newTask.Description != "source desc" {
		t.Errorf("expected description inherited, got %q", newTask.Description)
	}
	if newTask.Link != "https://example.com" {
		t.Errorf("expected link inherited, got %q", newTask.Link)
	}
	if newTask.AssigneeName != "Alice" {
		t.Errorf("expected assignee inherited, got %q", newTask.AssigneeName)
	}
	if newTask.CreatorName != "Bob" {
		t.Errorf("expected creator inherited, got %q", newTask.CreatorName)
	}
	// Code should NOT be inherited
	if newTask.Code != "" {
		t.Errorf("expected code NOT inherited, got %q", newTask.Code)
	}
}

func TestUpsertTask_DuplicateToAnotherList(t *testing.T) {
	mgr := newFakeManager(t)
	listA := mgr.addTaskList("List A", defaultStatuses())
	listB := mgr.addTaskList("List B", defaultStatuses())
	source := mgr.addTask(listA.ID, "Source", 1)
	source.Description = "original"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": listB.ID,
		"task_id":      source.ID,
		"duplicate":    true,
		"title":        "Copy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("cross-list duplicate should succeed: %s", result.Content)
	}

	var newTask *database.Task
	for _, tk := range mgr.tasks {
		if tk.ID != source.ID && tk.Title == "Copy" {
			newTask = tk
			break
		}
	}
	if newTask == nil {
		t.Fatal("duplicate task not found")
	}
	if newTask.TaskListID != listB.ID {
		t.Errorf("expected task_list_id=%s, got %s", listB.ID, newTask.TaskListID)
	}
	if newTask.Description != "original" {
		t.Errorf("expected description inherited, got %q", newTask.Description)
	}
}

func TestUpsertTask_DuplicateOverridesFields(t *testing.T) {
	mgr := newFakeManager(t)
	list := mgr.addTaskList("List", defaultStatuses())
	source := mgr.addTask(list.ID, "Source", 1)
	source.Description = "source desc"
	source.Link = "https://old.com"
	source.AssigneeName = "Alice"
	tool := NewTask(mgr)

	newAssignee := "Charlie"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id":  list.ID,
		"task_id":       source.ID,
		"duplicate":     true,
		"title":         "Override copy",
		"description":   "new desc",
		"link":          "https://new.com",
		"assignee_name": newAssignee,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate with overrides should succeed: %s", result.Content)
	}

	var newTask *database.Task
	for _, tk := range mgr.tasks {
		if tk.ID != source.ID && tk.Title == "Override copy" {
			newTask = tk
			break
		}
	}
	if newTask == nil {
		t.Fatal("duplicate task not found")
	}
	if newTask.Description != "new desc" {
		t.Errorf("expected overridden description, got %q", newTask.Description)
	}
	if newTask.Link != "https://new.com" {
		t.Errorf("expected overridden link, got %q", newTask.Link)
	}
	if newTask.AssigneeName != "Charlie" {
		t.Errorf("expected overridden assignee, got %q", newTask.AssigneeName)
	}
}

func TestUpsertTask_DuplicateWithCode(t *testing.T) {
	mgr := newFakeManager(t)
	list := mgr.addTaskList("List", defaultStatuses())
	source := mgr.addTask(list.ID, "Source", 1)
	source.Code = "JIRA-100"
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": list.ID,
		"task_id":      source.ID,
		"duplicate":    true,
		"title":        "Copy with code",
		"code":         "JIRA-200",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("duplicate with explicit code should succeed: %s", result.Content)
	}

	var newTask *database.Task
	for _, tk := range mgr.tasks {
		if tk.ID != source.ID && tk.Title == "Copy with code" {
			newTask = tk
			break
		}
	}
	if newTask == nil {
		t.Fatal("duplicate task not found")
	}
	if newTask.Code != "JIRA-200" {
		t.Errorf("expected explicit code JIRA-200, got %q", newTask.Code)
	}
}

func TestUpsertTask_DuplicateSourceNotFound(t *testing.T) {
	mgr := newFakeManager(t)
	list := mgr.addTaskList("List", defaultStatuses())
	tool := NewTask(mgr)

	ghostID := "9999"
	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": list.ID,
		"task_id":      ghostID,
		"duplicate":    true,
		"title":        "Ghost copy",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent source task")
	}
	if !strings.Contains(result.Content, "não encontrado") {
		t.Errorf("expected not-found error, got: %s", result.Content)
	}
}

func TestUpsertTask_DuplicateWithoutTaskID_Error(t *testing.T) {
	mgr := newFakeManager(t)
	list := mgr.addTaskList("List", defaultStatuses())
	tool := NewTask(mgr)

	result, err := tool.Execute(context.Background(), mustMarshal(t, map[string]any{
		"task_list_id": list.ID,
		"duplicate":    true,
		"title":        "No source",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when duplicate is true without task_id")
	}
	if !strings.Contains(result.Content, "informe task_id ou code") {
		t.Errorf("expected task ref error, got: %s", result.Content)
	}
}
