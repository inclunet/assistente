package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTaskPaginationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous := DB()
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := testDB.AutoMigrate(&TaskList{}, &TaskListWorkflow{}, &Task{}); err != nil {
		t.Fatal(err)
	}
	SetDB(testDB)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		SetDB(previous)
	})
	return testDB
}

func TestListTasksPageWithContext_KeysetStatusAndStableTieBreaker(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	tasks := []Task{
		{UUIDModel: UUIDModel{ID: "task-a", CreatedAt: base}, TaskListID: list.ID, Title: "A", StatusID: 1},
		{UUIDModel: UUIDModel{ID: "task-b", CreatedAt: base}, TaskListID: list.ID, Title: "B", StatusID: 1},
		{UUIDModel: UUIDModel{ID: "task-c", CreatedAt: base.Add(time.Minute)}, TaskListID: list.ID, Title: "C", StatusID: 2},
		{UUIDModel: UUIDModel{ID: "task-d", CreatedAt: base.Add(2 * time.Minute)}, TaskListID: list.ID, Title: "D", StatusID: 1},
	}
	for i := range tasks {
		if err := testDB.Create(&tasks[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	statusID := 1
	first, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		StatusID:   &statusID,
		Limit:      2,
		Sort:       TaskSortCreatedAtAsc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("expected first page metadata, got %+v", first)
	}
	if got := []string{first.Tasks[0].ID, first.Tasks[1].ID}; got[0] != "task-a" || got[1] != "task-b" {
		t.Fatalf("unexpected first page order: %v", got)
	}

	second, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		StatusID:   &statusID,
		Limit:      2,
		Sort:       TaskSortCreatedAtAsc,
		Cursor:     first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.HasMore || second.NextCursor != "" || len(second.Tasks) != 1 || second.Tasks[0].ID != "task-d" {
		t.Fatalf("unexpected second page: %+v", second)
	}
}

func TestListTasksPageWithContext_DescendingAndCursorBinding(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	for _, task := range []Task{
		{UUIDModel: UUIDModel{ID: "task-a", CreatedAt: base}, TaskListID: list.ID, Title: "A", StatusID: 1},
		{UUIDModel: UUIDModel{ID: "task-b", CreatedAt: base.Add(time.Minute)}, TaskListID: list.ID, Title: "B", StatusID: 1},
	} {
		if err := testDB.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	page, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		Limit:      1,
		Sort:       TaskSortCreatedAtDesc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tasks) != 1 || page.Tasks[0].ID != "task-b" {
		t.Fatalf("unexpected descending page: %+v", page)
	}
	_, err = ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		Limit:      1,
		Sort:       TaskSortCreatedAtAsc,
		Cursor:     page.NextCursor,
	})
	if err == nil || !strings.Contains(err.Error(), "não corresponde") {
		t.Fatalf("expected cursor binding error, got %v", err)
	}
}

func TestListTasksPageWithContext_EnforcesUserScope(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	otherList := TaskList{UUIDModel: UUIDModel{ID: "list-b"}, UserID: "user-b", Title: "Privada"}
	if err := testDB.Create(&otherList).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&Task{
		UUIDModel:  UUIDModel{ID: "task-private"},
		TaskListID: otherList.ID,
		Title:      "Segredo",
		StatusID:   1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := ListTasksPageWithContext(WithUserID(context.Background(), "user-a"), TaskPageQuery{
		TaskListID: otherList.ID,
		Limit:      10,
	})
	if err == nil {
		t.Fatal("expected cross-user query to fail")
	}
}

func TestEnsureTaskPaginationIndexes(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	if err := ensureTaskPaginationIndexes(testDB); err != nil {
		t.Fatal(err)
	}
	if err := ensureTaskPaginationIndexes(testDB); err != nil {
		t.Fatalf("index creation must be idempotent: %v", err)
	}
	var indexes []struct {
		Name string
	}
	if err := testDB.Raw(`PRAGMA index_list('tasks')`).Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(indexes))
	for _, index := range indexes {
		got[index.Name] = true
	}
	for _, name := range []string{"idx_tasks_list_created_id", "idx_tasks_list_status_created_id"} {
		if !got[name] {
			t.Fatalf("expected pagination index %q, got %v", name, got)
		}
	}
}
