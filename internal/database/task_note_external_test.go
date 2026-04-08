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

	tl, err := CreateTaskList("L", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTask(tl.ID, "T", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}

	typ := TaskNoteCustomer
	n1, created, err := UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
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

	n2, created2, err := UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Content:        "b",
		AuthorName:     "u2",
		ExternalSource: "jira",
		ExternalID:     "c1",
	})
	if err != nil || created2 || n2.ID != n1.ID || n2.Content != "b" {
		t.Fatalf("update: err=%v created=%v note=%+v", err, created2, n2)
	}

	notes, err := GetTaskNotes(task.ID)
	if err != nil || len(notes) != 1 {
		t.Fatalf("notes: %v %#v", err, notes)
	}
}

func TestUpsertTaskNoteByExternal_UniqueIndexRaceRetry(t *testing.T) {
	setupTaskNoteExternalTestDB(t)

	tl, err := CreateTaskList("L", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTask(tl.ID, "T", "", "", "", nil)
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

	n, created, err := UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
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

	tl, err := CreateTaskList("L", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	t1, _ := CreateTask(tl.ID, "A", "", "", "", nil)
	t2, _ := CreateTask(tl.ID, "B", "", "", "", nil)

	typ := TaskNoteSystem
	_, _, err = UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
		TaskID:         t1.ID,
		Type:           &typ,
		Content:        "x",
		ExternalSource: "jira",
		ExternalID:     "shared",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
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

	tl, _ := CreateTaskList("L", "", nil)
	task, _ := CreateTask(tl.ID, "T", "", "", "", nil)
	typ := TaskNoteInternal
	_, _, _ = UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Type:           &typ,
		Content:        "z",
		ExternalSource: "x",
		ExternalID:     "y",
	})

	got, err := FindTaskNoteByExternalRef("x", "y")
	if err != nil || got == nil || got.Content != "z" {
		t.Fatalf("find: err=%v got=%+v", err, got)
	}
	empty, err := FindTaskNoteByExternalRef("", "y")
	if err != nil || empty != nil {
		t.Fatalf("empty source: %v %#v", err, empty)
	}
}

func TestCreateTaskNote_ManualStillWorks(t *testing.T) {
	setupTaskNoteExternalTestDB(t)

	tl, _ := CreateTaskList("L", "", nil)
	task, _ := CreateTask(tl.ID, "T", "", "", "", nil)

	n, err := CreateTaskNote(task.ID, TaskNoteAgent, "hello", "", "")
	if err != nil || n.ExternalSource != "" || n.ExternalID != "" {
		t.Fatalf("manual note: err=%v n=%+v", err, n)
	}
}

func TestUpdateTaskNote_ByIDStillWorks(t *testing.T) {
	setupTaskNoteExternalTestDB(t)

	tl, _ := CreateTaskList("L", "", nil)
	task, _ := CreateTask(tl.ID, "T", "", "", "", nil)
	n, err := CreateTaskNote(task.ID, TaskNoteInternal, "a", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateTaskNote(n.ID, "b"); err != nil {
		t.Fatal(err)
	}
	got, _ := GetTaskNote(n.ID)
	if got.Content != "b" {
		t.Fatalf("content %q", got.Content)
	}
}

func TestUpsertTaskNoteByExternal_ExternalUpdatedAtOptional(t *testing.T) {
	setupTaskNoteExternalTestDB(t)

	tl, _ := CreateTaskList("L", "", nil)
	task, _ := CreateTask(tl.ID, "T", "", "", "", nil)
	typ := TaskNoteInternal
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_, _, err := UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
		TaskID:             task.ID,
		Type:               &typ,
		Content:            "t",
		ExternalSource:     "jira",
		ExternalID:         "e1",
		ExternalUpdatedAt:  &ts,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Content:        "t2",
		ExternalSource: "jira",
		ExternalID:     "e1",
	})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := FindTaskNoteByExternalRef("jira", "e1")
	if n == nil || n.ExternalUpdatedAt == nil || !n.ExternalUpdatedAt.Equal(ts) {
		// omitting external_updated_at on update must not clear prior value
		t.Fatalf("expected timestamp preserved, got %+v", n)
	}
}
