package database

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestCreateTaskListInvalidSlugDoesNotLeavePartialList(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()

	if _, err := CreateTaskListWithContext(ctx, "Plano", "", nil, "slug inválido"); err == nil {
		t.Fatal("esperava erro para slug inválido")
	}

	var lists int64
	if err := db.Model(&TaskList{}).Count(&lists).Error; err != nil {
		t.Fatal(err)
	}
	var workflows int64
	if err := db.Model(&TaskListWorkflow{}).Count(&workflows).Error; err != nil {
		t.Fatal(err)
	}
	if lists != 0 || workflows != 0 {
		t.Fatalf("criação inválida deixou dados parciais: lists=%d workflows=%d", lists, workflows)
	}
}

func TestCreateTaskListRollsBackWhenWorkflowCreationFails(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	const callback = "test:fail_tasklist_workflow"
	if err := db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "TaskListWorkflow" {
			_ = tx.AddError(errors.New("falha injetada no workflow"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Callback().Create().Remove(callback)
	})

	if _, err := CreateTaskListWithContext(ctx, "Plano", "", nil, "rollback-plan"); err == nil {
		t.Fatal("esperava falha injetada ao criar workflow")
	}

	var lists int64
	if err := db.Model(&TaskList{}).Count(&lists).Error; err != nil {
		t.Fatal(err)
	}
	var workflows int64
	if err := db.Model(&TaskListWorkflow{}).Count(&workflows).Error; err != nil {
		t.Fatal(err)
	}
	if lists != 0 || workflows != 0 {
		t.Fatalf("transação não reverteu criação parcial: lists=%d workflows=%d", lists, workflows)
	}
}

func TestUpdateTaskStatusClearsAndRestoresCompletedAt(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()

	list, err := CreateTaskListWithContext(ctx, "Plano", "", nil, "plan-status")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, list.ID, "Etapa", "", "plan:step", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateTaskStatusWithContext(ctx, task.ID, 3); err != nil {
		t.Fatal(err)
	}
	completed, err := GetTaskWithContext(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.CompletedAt == nil {
		t.Fatal("status final deveria preencher completed_at")
	}

	if err := UpdateTaskStatusWithContext(ctx, task.ID, 1); err != nil {
		t.Fatal(err)
	}
	reopened, err := GetTaskWithContext(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.CompletedAt != nil {
		t.Fatalf("reabertura deveria limpar completed_at: %v", reopened.CompletedAt)
	}

	if err := UpdateTaskStatusWithContext(ctx, task.ID, 3); err != nil {
		t.Fatal(err)
	}
	recompleted, err := GetTaskWithContext(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recompleted.CompletedAt == nil {
		t.Fatal("nova conclusão deveria restaurar completed_at")
	}
}
