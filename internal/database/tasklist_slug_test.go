package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTaskListSlugTestDB(t *testing.T) {
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

func TestResolveTaskListID(t *testing.T) {
	setupTaskListSlugTestDB(t)
	ctx := testCtx()
	a, err := CreateTaskListWithContext(ctx, "A", "", nil, "my-alpha")
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateTaskListWithContext(ctx, "B", "", nil, "beta")
	if err != nil {
		t.Fatal(err)
	}
	_ = b

	id := a.ID
	idPtr := id

	got, err := ResolveTaskListIDWithContext(ctx, &idPtr, "")
	if err != nil || got != id {
		t.Fatalf("by id: got %s err %v", got, err)
	}

	got, err = ResolveTaskListIDWithContext(ctx, nil, "MY-ALPHA")
	if err != nil || got != id {
		t.Fatalf("by slug: got %s err %v", got, err)
	}

	got, err = ResolveTaskListIDWithContext(ctx, &idPtr, "my-alpha")
	if err != nil || got != id {
		t.Fatalf("both consistent: got %s err %v", got, err)
	}

	_, err = ResolveTaskListIDWithContext(ctx, &idPtr, "beta")
	if err == nil {
		t.Fatal("expected mismatch id vs slug")
	}

	_, err = ResolveTaskListIDWithContext(ctx, nil, "")
	if err == nil {
		t.Fatal("expected error when no id and no slug")
	}
}

func TestCreateTaskListDuplicateSlug(t *testing.T) {
	setupTaskListSlugTestDB(t)
	ctx := testCtx()
	if _, err := CreateTaskListWithContext(ctx, "A", "", nil, "dup"); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTaskListWithContext(ctx, "B", "", nil, "dup"); err == nil {
		t.Fatal("expected duplicate slug error")
	}
}
