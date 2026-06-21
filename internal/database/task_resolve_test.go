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
	if err := ensureTaskListSlugUniqueIndex(); err != nil {
		t.Fatalf("ensureTaskListSlugUniqueIndex: %v", err)
	}
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
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "L", "", nil, "my-list")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, list.ID, "T", "", "ABC-1", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	id := task.ID
	idPtr := id

	got, err := ResolveTaskIDWithContext(ctx, nil, "", &idPtr, "")
	if err != nil || got != id {
		t.Fatalf("by id only: got %s err %v", got, err)
	}

	got, err = ResolveTaskIDWithContext(ctx, nil, "", &idPtr, "ABC-1")
	if err != nil || got != id {
		t.Fatalf("by id+code: got %s err %v", got, err)
	}

	_, err = ResolveTaskIDWithContext(ctx, nil, "", &idPtr, "WRONG")
	if err == nil {
		t.Fatal("expected code mismatch")
	}

	lid := list.ID
	got, err = ResolveTaskIDWithContext(ctx, &lid, "my-list", &idPtr, "ABC-1")
	if err != nil || got != id {
		t.Fatalf("by id+code+list: got %s err %v", got, err)
	}

	got, err = ResolveTaskIDWithContext(ctx, nil, "my-list", nil, "ABC-1")
	if err != nil || got != id {
		t.Fatalf("by slug+code: got %s err %v", got, err)
	}

	_, err = ResolveTaskIDWithContext(ctx, nil, "", nil, "ABC-1")
	if err == nil {
		t.Fatal("expected error without list for code-only")
	}
}

func TestResolveTaskIDListMismatchWithCode(t *testing.T) {
	setupTaskResolveTestDB(t)
	ctx := testCtx()
	a, _ := CreateTaskListWithContext(ctx, "A", "", nil, "list-a")
	b, _ := CreateTaskListWithContext(ctx, "B", "", nil, "list-b")
	task, _ := CreateTaskWithContext(ctx, a.ID, "T", "", "X", "", nil)
	idPtr := task.ID
	bid := b.ID
	_, err := ResolveTaskIDWithContext(ctx, &bid, "", &idPtr, "X")
	if err == nil {
		t.Fatal("expected list mismatch when id+code+wrong list")
	}
}

func TestResolveTaskIDByTaskCode_GlobalUnique(t *testing.T) {
	setupTaskResolveTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "L", "", nil, "my-list")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, list.ID, "T", "", "FSD-99", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ResolveTaskIDByTaskCodeWithContext(ctx, nil, "FSD-99")
	if err != nil || got != task.ID {
		t.Fatalf("got %s err %v", got, err)
	}
}

func TestResolveTaskIDByTaskCode_NotFound(t *testing.T) {
	setupTaskResolveTestDB(t)
	ctx := testCtx()
	list, _ := CreateTaskListWithContext(ctx, "L", "", nil, "my-list")
	_, _ = CreateTaskWithContext(ctx, list.ID, "T", "", "ONLY", "", nil)
	_, err := ResolveTaskIDByTaskCodeWithContext(ctx, nil, "MISSING")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveTaskIDByTaskCode_Ambiguous(t *testing.T) {
	setupTaskResolveTestDB(t)
	ctx := testCtx()
	a, _ := CreateTaskListWithContext(ctx, "A", "", nil, "la")
	b, _ := CreateTaskListWithContext(ctx, "B", "", nil, "lb")
	ta, _ := CreateTaskWithContext(ctx, a.ID, "T", "", "DUP", "", nil)
	_, _ = CreateTaskWithContext(ctx, b.ID, "T", "", "DUP", "", nil)
	_, err := ResolveTaskIDByTaskCodeWithContext(ctx, nil, "DUP")
	if err == nil {
		t.Fatal("expected ambiguous error")
	}
	lid := a.ID
	got, err := ResolveTaskIDByTaskCodeWithContext(ctx, &lid, "DUP")
	if err != nil || got != ta.ID {
		t.Fatalf("got %s err %v want %s", got, err, ta.ID)
	}
}

func TestResolveTaskIDByTaskCode_ScopedNotFound(t *testing.T) {
	setupTaskResolveTestDB(t)
	ctx := testCtx()
	a, _ := CreateTaskListWithContext(ctx, "A", "", nil, "la")
	b, _ := CreateTaskListWithContext(ctx, "B", "", nil, "lb")
	_, _ = CreateTaskWithContext(ctx, a.ID, "T", "", "X1", "", nil)
	lid := b.ID
	_, err := ResolveTaskIDByTaskCodeWithContext(ctx, &lid, "X1")
	if err == nil {
		t.Fatal("expected not found in list B")
	}
}
