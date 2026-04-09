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
	ensureTaskNoteExternalUniqueIndex()
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
	tl, _ := CreateTaskList("L", "", nil, "")
	_ = SetTaskListValidationPolicy(tl.ID, `{"task_code_regex":"^FSD-\\d+$"}`)

	_, err := CreateTask(tl.ID, "x", "", "FSD-99", "", nil)
	if err != nil {
		t.Fatalf("expected ok: %v", err)
	}
	_, err = CreateTask(tl.ID, "y", "", "BAD", "", nil)
	if err == nil {
		t.Fatal("expected error for code BAD")
	}
}

func TestValidateExternalNoteForTaskList_AllowedSources(t *testing.T) {
	setupValidationPolicyTestDB(t)
	tl, _ := CreateTaskList("L", "", nil, "")
	_ = SetTaskListValidationPolicy(tl.ID, `{"allowed_note_sources":["jira","linear"]}`)
	task, _ := CreateTask(tl.ID, "T", "", "", "", nil)
	typ := TaskNoteInternal

	_, _, err := UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
		TaskID:         task.ID,
		Type:           &typ,
		Content:        "a",
		ExternalSource: "jira",
		ExternalID:     "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = UpsertTaskNoteByExternal(UpsertTaskNoteByExternalParams{
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
	tl1, _ := CreateTaskList("A", "", nil, "")
	tl2, _ := CreateTaskList("B", "", nil, "")
	_ = SetTaskListValidationPolicy(tl2.ID, `{"task_code_regex":"^X-\\d+$"}`)

	task, _ := CreateTask(tl1.ID, "t", "", "FSD-1", "", nil)
	_, err := MoveTaskToList(task.ID, tl2.ID)
	if err == nil {
		t.Fatal("expected move rejected: code does not match target list regex")
	}
}

func TestSetTaskListValidationPolicy_InvalidJSON(t *testing.T) {
	setupValidationPolicyTestDB(t)
	tl, _ := CreateTaskList("L", "", nil, "")
	err := SetTaskListValidationPolicy(tl.ID, `{not json`)
	if err == nil {
		t.Fatal("expected error")
	}
}
