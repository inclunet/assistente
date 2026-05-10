package database

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTaskNoteExternalTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&TaskListWorkflow{},
		&TaskList{},
		&Task{},
		&TaskNote{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ensureTaskNoteExternalUniqueIndex()
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

func TestUpsertTaskNoteByExternal_CreateAndUpdate(t *testing.T) {
	setupTaskNoteExternalTestDB(t)
	ctx := testCtx()

	tl, err := CreateTaskListWithContext(ctx, "L", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	typ := TaskNoteCustomer
	n1, created, err := UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:           task.ID,
		Type:             &typ,
		Content:          "a",
		AuthorName:       "u",
		ExternalSource:   "jira",
		ExternalID:       "c1",
		ExternalParentID: "K-1",
	})
	if err != nil || !created || n1.Content != "a" {
		t.Fatalf("create: err=%v created=%v note=%+v", err, created, n1)
	}

	n2, created2, err := UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Content:        "b",
		AuthorName:     "u2",
		ExternalSource: "jira",
		ExternalID:     "c1",
	})
	if err != nil || created2 || n2.ID != n1.ID || n2.Content != "b" {
		t.Fatalf("update: err=%v created=%v note=%+v", err, created2, n2)
	}

	notes, err := GetTaskNotesWithContext(ctx, task.ID)
	if err != nil || len(notes) != 1 {
		t.Fatalf("notes: %v %#v", err, notes)
	}
}

func TestUpsertTaskNoteByExternal_UniqueIndexRaceRetry(t *testing.T) {
	setupTaskNoteExternalTestDB(t)
	ctx := testCtx()

	tl, err := CreateTaskListWithContext(ctx, "L", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	typ := TaskNoteInternal
	// Inserção direta simula linha criada por outra transação antes do upsert.
	direct := &TaskNote{
		TaskID:         task.ID,
		Type:           typ,
		Content:        "race",
		ExternalSource: "jira",
		ExternalID:     "race-id",
	}
	if err := db.Create(direct).Error; err != nil {
		t.Fatal(err)
	}

	n, created, err := UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Type:           &typ,
		Content:        "after",
		ExternalSource: "jira",
		ExternalID:     "race-id",
	})
	if err != nil || created || n.Content != "after" {
		t.Fatalf("after race path: err=%v created=%v n=%+v", err, created, n)
	}
}

func TestUpsertTaskNoteByExternal_WrongTaskError(t *testing.T) {
	setupTaskNoteExternalTestDB(t)
	ctx := testCtx()

	tl, err := CreateTaskListWithContext(ctx, "L", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	t1, _ := CreateTaskWithContext(ctx, tl.ID, "A", "", "", "", nil)
	t2, _ := CreateTaskWithContext(ctx, tl.ID, "B", "", "", "", nil)

	typ := TaskNoteSystem
	_, _, err = UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:         t1.ID,
		Type:           &typ,
		Content:        "x",
		ExternalSource: "jira",
		ExternalID:     "shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:         t2.ID,
		Type:           &typ,
		Content:        "y",
		ExternalSource: "jira",
		ExternalID:     "shared",
	})
	if err == nil {
		t.Fatal("expected error when external ref already on another task")
	}
}

func TestFindTaskNoteByExternalRef(t *testing.T) {
	setupTaskNoteExternalTestDB(t)
	ctx := testCtx()

	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	task, _ := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	typ := TaskNoteInternal
	_, _, _ = UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Type:           &typ,
		Content:        "z",
		ExternalSource: "x",
		ExternalID:     "y",
	})

	got, err := FindTaskNoteByExternalRefWithContext(ctx, "x", "y")
	if err != nil || got == nil || got.Content != "z" {
		t.Fatalf("find: err=%v got=%+v", err, got)
	}
	empty, err := FindTaskNoteByExternalRefWithContext(ctx, "", "y")
	if err != nil || empty != nil {
		t.Fatalf("empty source: %v %#v", err, empty)
	}
}

func TestCreateTaskNote_ManualStillWorks(t *testing.T) {
	setupTaskNoteExternalTestDB(t)
	ctx := testCtx()

	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	task, _ := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)

	n, err := CreateTaskNoteWithContext(ctx, task.ID, TaskNoteAgent, "hello", "", "")
	if err != nil || n.ExternalSource != "" || n.ExternalID != "" {
		t.Fatalf("manual note: err=%v n=%+v", err, n)
	}
}

func TestUpdateTaskNote_ByIDStillWorks(t *testing.T) {
	setupTaskNoteExternalTestDB(t)
	ctx := testCtx()

	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	task, _ := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	n, err := CreateTaskNoteWithContext(ctx, task.ID, TaskNoteInternal, "a", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateTaskNoteWithContext(ctx, n.ID, "b"); err != nil {
		t.Fatal(err)
	}
	got, _ := GetTaskNoteWithContext(ctx, n.ID)
	if got.Content != "b" {
		t.Fatalf("content %q", got.Content)
	}
}

func TestUpsertTaskNoteByExternal_ExternalUpdatedAtOptional(t *testing.T) {
	setupTaskNoteExternalTestDB(t)
	ctx := testCtx()

	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	task, _ := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	typ := TaskNoteInternal
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_, _, err := UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:            task.ID,
		Type:              &typ,
		Content:           "t",
		ExternalSource:    "jira",
		ExternalID:        "e1",
		ExternalUpdatedAt: &ts,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Content:        "t2",
		ExternalSource: "jira",
		ExternalID:     "e1",
	})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := FindTaskNoteByExternalRefWithContext(ctx, "jira", "e1")
	if n == nil || n.ExternalUpdatedAt == nil || !n.ExternalUpdatedAt.Equal(ts) {
		// omitting external_updated_at on update must not clear prior value
		t.Fatalf("expected timestamp preserved, got %+v", n)
	}
}
