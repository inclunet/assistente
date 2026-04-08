package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTaskResolveTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&TaskListWorkflow{}, &TaskList{}, &Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ensureTaskListSlugUniqueIndex()
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

func TestResolveTaskIDByIDAndCode(t *testing.T) {
	setupTaskResolveTestDB(t)
	list, err := CreateTaskList("L", "", nil, "my-list")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTask(list.ID, "T", "", "ABC-1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	id := task.ID
	idPtr := id

	got, err := ResolveTaskID(nil, "", &idPtr, "")
	if err != nil || got != id {
		t.Fatalf("by id only: got %d err %v", got, err)
	}

	got, err = ResolveTaskID(nil, "", &idPtr, "ABC-1")
	if err != nil || got != id {
		t.Fatalf("by id+code: got %d err %v", got, err)
	}

	_, err = ResolveTaskID(nil, "", &idPtr, "WRONG")
	if err == nil {
		t.Fatal("expected code mismatch")
	}

	lid := list.ID
	got, err = ResolveTaskID(&lid, "my-list", &idPtr, "ABC-1")
	if err != nil || got != id {
		t.Fatalf("by id+code+list: got %d err %v", got, err)
	}

	got, err = ResolveTaskID(nil, "my-list", nil, "ABC-1")
	if err != nil || got != id {
		t.Fatalf("by slug+code: got %d err %v", got, err)
	}

	_, err = ResolveTaskID(nil, "", nil, "ABC-1")
	if err == nil {
		t.Fatal("expected error without list for code-only")
	}
}

func TestResolveTaskIDListMismatchWithCode(t *testing.T) {
	setupTaskResolveTestDB(t)
	a, _ := CreateTaskList("A", "", nil, "list-a")
	b, _ := CreateTaskList("B", "", nil, "list-b")
	task, _ := CreateTask(a.ID, "T", "", "X", "", nil)
	idPtr := task.ID
	bid := b.ID
	_, err := ResolveTaskID(&bid, "", &idPtr, "X")
	if err == nil {
		t.Fatal("expected list mismatch when id+code+wrong list")
	}
}
