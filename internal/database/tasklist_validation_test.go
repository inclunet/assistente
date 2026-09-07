package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupValidationPolicyTestDB(t *testing.T) {
	t.Helper()
	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&TaskListWorkflow{}, &TaskList{}, &Task{}, &TaskNote{}); err != nil {
		t.Fatal(err)
	}
	if err := ensureTaskNoteExternalUniqueIndex(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

func TestValidateTaskCodeForTaskList_Regex(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	_ = SetTaskListValidationPolicyWithContext(ctx, tl.ID, `{"task_code_regex":"^FSD-\\d+$"}`)

	_, err := CreateTaskWithContext(ctx, tl.ID, "x", "", "FSD-99", "", nil)
	if err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	_, err = CreateTaskWithContext(ctx, tl.ID, "y", "", "BAD", "", nil)
	if err == nil {
		t.Fatal("expected error for code BAD")
	}
}

func TestValidateExternalNoteForTaskList_AllowedSources(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	_ = SetTaskListValidationPolicyWithContext(ctx, tl.ID, `{"allowed_note_sources":["jira","linear"]}`)
	task, _ := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	typ := TaskNoteInternal

	_, _, err := UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Type:           &typ,
		Content:        "a",
		ExternalSource: "jira",
		ExternalID:     "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = UpsertTaskNoteByExternalWithContext(ctx, UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Type:           &typ,
		Content:        "b",
		ExternalSource: "slack",
		ExternalID:     "2",
	})
	if err == nil {
		t.Fatal("expected error for disallowed source")
	}
}

func TestMoveTaskToList_RespectsTargetCodePolicy(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl1, _ := CreateTaskListWithContext(ctx, "A", "", nil, "")
	tl2, _ := CreateTaskListWithContext(ctx, "B", "", nil, "")
	_ = SetTaskListValidationPolicyWithContext(ctx, tl2.ID, `{"task_code_regex":"^X-\\d+$"}`)

	task, _ := CreateTaskWithContext(ctx, tl1.ID, "t", "", "FSD-1", "", nil)
	_, err := MoveTaskToListWithContext(ctx, task.ID, tl2.ID)
	if err == nil {
		t.Fatal("expected move rejected: code does not match target list regex")
	}
}

func TestSetTaskListValidationPolicy_InvalidJSON(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	err := SetTaskListValidationPolicyWithContext(ctx, tl.ID, `{not json`)
	if err == nil {
		t.Fatal("expected error")
	}
}
