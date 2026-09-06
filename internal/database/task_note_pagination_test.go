package database

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTaskNotePaginationTestDB(t *testing.T) *gorm.DB {
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
	if err := testDB.AutoMigrate(&TaskList{}, &Task{}, &TaskNote{}); err != nil {
		t.Fatal(err)
	}
	SetDB(testDB)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		SetDB(previous)
	})
	return testDB
}

func seedTaskNotePage(t *testing.T, testDB *gorm.DB, userID, listID, taskID string, notes []TaskNote) {
	t.Helper()
	if err := testDB.Create(&TaskList{
		UUIDModel: UUIDModel{ID: listID},
		UserID:    userID,
		Title:     listID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&Task{
		UUIDModel:  UUIDModel{ID: taskID},
		TaskListID: listID,
		Title:      taskID,
		StatusID:   1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	for i := range notes {
		notes[i].UserID = userID
		notes[i].TaskID = taskID
		if err := testDB.Create(&notes[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func stringFilter(value string) NullableStringFilter {
	return NullableStringFilter{Set: true, Value: &value}
}

func TestListTaskNotesPageWithContext_FiltersAndStableKeyset(t *testing.T) {
	testDB := setupTaskNotePaginationTestDB(t)
	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	seedTaskNotePage(t, testDB, "user-a", "list-a", "task-a", []TaskNote{
		{UUIDModel: UUIDModel{ID: "note-a", CreatedAt: base}, Type: TaskNoteCustomer, Content: "A", ExternalSource: "jira", ExternalID: "a", ExternalParentID: "thread-1"},
		{UUIDModel: UUIDModel{ID: "note-b", CreatedAt: base}, Type: TaskNoteCustomer, Content: "B", ExternalSource: "jira", ExternalID: "b", ExternalParentID: "thread-1"},
		{UUIDModel: UUIDModel{ID: "note-c", CreatedAt: base.Add(time.Minute)}, Type: TaskNoteInternal, Content: "C"},
		{UUIDModel: UUIDModel{ID: "note-d", CreatedAt: base.Add(2 * time.Minute)}, Type: TaskNoteCustomer, Content: "D", ExternalSource: "jira", ExternalID: "d", ExternalParentID: "thread-1"},
	})
	noteType := TaskNoteCustomer
	query := TaskNotePageQuery{
		TaskID:           ptrString("task-a"),
		Source:           stringFilter("jira"),
		Type:             &noteType,
		ExternalParentID: stringFilter("thread-1"),
		Limit:            2,
		Sort:             TaskSortCreatedAtAsc,
	}
	first, err := ListTaskNotesPageWithContext(WithUserID(context.Background(), "user-a"), query)
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("expected continuation metadata, got %+v", first)
	}
	if got := []string{first.Notes[0].ID, first.Notes[1].ID}; got[0] != "note-a" || got[1] != "note-b" {
		t.Fatalf("unexpected stable order: %v", got)
	}
	query.Cursor = first.NextCursor
	second, err := ListTaskNotesPageWithContext(WithUserID(context.Background(), "user-a"), query)
	if err != nil {
		t.Fatal(err)
	}
	if second.HasMore || second.NextCursor != "" || len(second.Notes) != 1 || second.Notes[0].ID != "note-d" {
		t.Fatalf("unexpected second page: %+v", second)
	}
	exact, err := ListTaskNotesPageWithContext(WithUserID(context.Background(), "user-a"), TaskNotePageQuery{
		ExternalID: stringFilter("b"),
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.Notes) != 1 || exact.Notes[0].ID != "note-b" {
		t.Fatalf("external_id exact filter returned %+v", exact.Notes)
	}
}

func TestListTaskNotesPageWithContext_DescendingAndCursorBinding(t *testing.T) {
	testDB := setupTaskNotePaginationTestDB(t)
	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	seedTaskNotePage(t, testDB, "user-a", "list-a", "task-a", []TaskNote{
		{UUIDModel: UUIDModel{ID: "note-a", CreatedAt: base}, Type: TaskNoteInternal, Content: "A"},
		{UUIDModel: UUIDModel{ID: "note-b", CreatedAt: base.Add(time.Minute)}, Type: TaskNoteCustomer, Content: "B"},
	})
	query := TaskNotePageQuery{Limit: 1, Sort: TaskSortCreatedAtDesc}
	page, err := ListTaskNotesPageWithContext(WithUserID(context.Background(), "user-a"), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Notes) != 1 || page.Notes[0].ID != "note-b" {
		t.Fatalf("unexpected descending page: %+v", page)
	}
	query.Cursor = page.NextCursor
	query.Type = ptrTaskNoteType(TaskNoteCustomer)
	_, err = ListTaskNotesPageWithContext(WithUserID(context.Background(), "user-a"), query)
	if err == nil || !strings.Contains(err.Error(), "não corresponde") {
		t.Fatalf("expected cursor/filter mismatch, got %v", err)
	}
	if _, err := ListTaskNotesPageWithContext(WithUserID(context.Background(), "user-a"), TaskNotePageQuery{Cursor: "invalid", Limit: 10}); err == nil || !strings.Contains(err.Error(), "cursor inválido") {
		t.Fatalf("expected invalid cursor error, got %v", err)
	}
}

func TestListTaskNotesPageWithContext_NullFiltersAndUserScope(t *testing.T) {
	testDB := setupTaskNotePaginationTestDB(t)
	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	seedTaskNotePage(t, testDB, "user-a", "list-a", "task-a", []TaskNote{
		{UUIDModel: UUIDModel{ID: "note-local", CreatedAt: base}, Type: TaskNoteInternal, Content: "Local"},
		{UUIDModel: UUIDModel{ID: "note-external", CreatedAt: base.Add(time.Minute)}, Type: TaskNoteCustomer, Content: "External", ExternalSource: "jira", ExternalID: "c1"},
	})
	seedTaskNotePage(t, testDB, "user-b", "list-b", "task-b", []TaskNote{
		{UUIDModel: UUIDModel{ID: "note-private", CreatedAt: base}, Type: TaskNoteInternal, Content: "Private"},
	})
	page, err := ListTaskNotesPageWithContext(WithUserID(context.Background(), "user-a"), TaskNotePageQuery{
		Source:           NullableStringFilter{Set: true},
		ExternalID:       NullableStringFilter{Set: true},
		ExternalParentID: NullableStringFilter{Set: true},
		Limit:            10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Notes) != 1 || page.Notes[0].ID != "note-local" {
		t.Fatalf("null filters should select local top-level note, got %+v", page.Notes)
	}
	all, err := ListTaskNotesPageWithContext(WithUserID(context.Background(), "user-a"), TaskNotePageQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Notes) != 2 {
		t.Fatalf("cross-user note leaked or own note missing: %+v", all.Notes)
	}
}

func TestEnsureTaskNotePaginationIndexes(t *testing.T) {
	testDB := setupTaskNotePaginationTestDB(t)
	if err := ensureTaskNotePaginationIndexes(testDB); err != nil {
		t.Fatal(err)
	}
	if err := ensureTaskNotePaginationIndexes(testDB); err != nil {
		t.Fatalf("index creation must be idempotent: %v", err)
	}
	var indexes []struct{ Name string }
	if err := testDB.Raw(`PRAGMA index_list('task_notes')`).Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(indexes))
	for _, index := range indexes {
		got[index.Name] = true
	}
	for _, name := range []string{
		"idx_task_notes_created_id",
		"idx_task_notes_task_created_id",
		"idx_task_notes_source_created_id",
		"idx_task_notes_type_created_id",
		"idx_task_notes_external_id_created_id",
		"idx_task_notes_parent_created_id",
	} {
		if !got[name] {
			t.Fatalf("expected pagination index %q, got %v", name, got)
		}
	}
}

func ptrString(value string) *string {
	return &value
}

func ptrTaskNoteType(value TaskNoteType) *TaskNoteType {
	return &value
}
