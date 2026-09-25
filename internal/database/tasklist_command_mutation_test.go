package database

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"
)

func TestReadTaskListCommandTargetUsesOneAuthoritativeSnapshot(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "Origem", "descrição", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTaskWithContext(ctx, list.ID, "Card", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	target, fingerprint, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if target.Title != "Origem" || target.Description != "descrição" || target.Workflow == nil || fingerprint == "" {
		t.Fatalf("snapshot incompleto: target=%+v fingerprint=%q", target, fingerprint)
	}
	if target.TaskCount != 1 {
		t.Fatalf("payload não refletiu o mesmo snapshot: task_count=%d", target.TaskCount)
	}
}

func TestTaskListCommandUpdateRejectsConcurrentABA(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "A", "descrição", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	_, before, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateTaskListWithContext(ctx, list.ID, "B", "descrição"); err != nil {
		t.Fatal(err)
	}
	if err := UpdateTaskListWithContext(ctx, list.ID, "A", "descrição"); err != nil {
		t.Fatal(err)
	}
	_, err = CommitTaskListCommandMutationWithContext(ctx, TaskListCommandMutationRequest{
		Operation:           "update",
		ID:                  list.ID,
		Title:               "novo",
		Description:         "descrição",
		ExpectedFingerprint: before,
	})
	if !errors.Is(err, ErrTaskListCommandStale) {
		t.Fatalf("ABA aceita ou erro errado: %v", err)
	}
	current, err := GetTaskListMetadataWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Title != "A" {
		t.Fatalf("mutação stale vazou: %q", current.Title)
	}
}

func TestTaskListCommandClearPreservesListAndWorkflowAndRemovesContent(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "Limpar", "descrição", nil, "limpar")
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := GetWorkflowWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := CreateTaskWithContext(ctx, list.ID, "Pai", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := CreateTaskWithContext(ctx, list.ID, "Filho", "", "", "", &parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{parent.ID, child.ID} {
		if _, err := CreateTaskNoteWithContext(ctx, taskID, TaskNoteInternal, "nota", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	_, fingerprint, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}

	result, err := CommitTaskListCommandMutationWithContext(ctx, TaskListCommandMutationRequest{
		Operation:           "clear",
		ID:                  list.ID,
		ExpectedFingerprint: fingerprint,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != list.ID {
		t.Fatalf("clear retornou lista errada: %q", result.ID)
	}
	remaining, err := GetTasksByTaskListIDWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("clear preservou tarefas: %d", len(remaining))
	}
	if _, err := GetTaskListWithContext(ctx, list.ID); err != nil {
		t.Fatalf("clear removeu a lista: %v", err)
	}
	workflowAfter, err := GetWorkflowWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if workflowAfter.ID != workflow.ID || workflowAfter.Statuses != workflow.Statuses || workflowAfter.AllowedTransitions != workflow.AllowedTransitions {
		t.Fatalf("clear alterou workflow: antes=%+v depois=%+v", workflow, workflowAfter)
	}
	for _, taskID := range []string{parent.ID, child.ID} {
		notes, err := GetTaskNotesWithContext(ctx, taskID)
		if err != nil || len(notes) != 0 {
			t.Fatalf("notas preservadas após clear: len=%d err=%v", len(notes), err)
		}
	}
}

func TestTaskListCommandClearRollbackLeavesContent(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "Limpar", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, list.ID, "Card", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTaskNoteWithContext(ctx, task.ID, TaskNoteInternal, "nota", "", ""); err != nil {
		t.Fatal(err)
	}
	_, fingerprint, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER reject_command_clear BEFORE DELETE ON tasks BEGIN SELECT RAISE(ABORT, 'reject clear'); END").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP TRIGGER reject_command_clear").Error })

	if _, err := CommitTaskListCommandMutationWithContext(ctx, TaskListCommandMutationRequest{Operation: "clear", ID: list.ID, ExpectedFingerprint: fingerprint}); err == nil {
		t.Fatal("clear com falha deveria retornar erro")
	}
	if _, err := GetTaskWithContext(ctx, task.ID); err != nil {
		t.Fatalf("task removida parcialmente: %v", err)
	}
	if notes, err := GetTaskNotesWithContext(ctx, task.ID); err != nil || len(notes) != 1 || notes[0].Content != "nota" {
		t.Fatalf("notas removidas parcialmente: len=%d err=%v", len(notes), err)
	}
}

func TestTaskListCommandClearRejectsNoteCreatedAfterSnapshot(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "Limpar", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, list.ID, "Card", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, fingerprint, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTaskNoteWithContext(ctx, task.ID, TaskNoteInternal, "criada depois do snapshot", "", ""); err != nil {
		t.Fatal(err)
	}

	if _, err := CommitTaskListCommandMutationWithContext(ctx, TaskListCommandMutationRequest{
		Operation:           "clear",
		ID:                  list.ID,
		ExpectedFingerprint: fingerprint,
	}); !errors.Is(err, ErrTaskListCommandStale) {
		t.Fatalf("clear aceitou nota criada após snapshot: %v", err)
	}
	if _, err := GetTaskWithContext(ctx, task.ID); err != nil {
		t.Fatalf("clear stale removeu task: %v", err)
	}
	notes, err := GetTaskNotesWithContext(ctx, task.ID)
	if err != nil || len(notes) != 1 || notes[0].Content != "criada depois do snapshot" {
		t.Fatalf("nota posterior não foi preservada: len=%d err=%v notes=%+v", len(notes), err, notes)
	}
}

func TestTaskListCommandClearRejectsStaleCancellationAndOtherOwner(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "Limpar", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, list.ID, "Card", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, fingerprint, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateTaskListWithContext(ctx, list.ID, "alterada", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := CommitTaskListCommandMutationWithContext(ctx, TaskListCommandMutationRequest{Operation: "clear", ID: list.ID, ExpectedFingerprint: fingerprint}); !errors.Is(err, ErrTaskListCommandStale) {
		t.Fatalf("clear stale aceito: %v", err)
	}
	if _, err := GetTaskWithContext(ctx, task.ID); err != nil {
		t.Fatalf("clear stale removeu task: %v", err)
	}

	_, fingerprint, err = ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherOwner := WithUserID(context.Background(), "outro-owner")
	if _, err := CommitTaskListCommandMutationWithContext(otherOwner, TaskListCommandMutationRequest{Operation: "clear", ID: list.ID, ExpectedFingerprint: fingerprint}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("owner incorreto aceito: %v", err)
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := CommitTaskListCommandMutationWithContext(cancelled, TaskListCommandMutationRequest{Operation: "clear", ID: list.ID, ExpectedFingerprint: fingerprint}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento não recusado: %v", err)
	}
	if _, err := GetTaskWithContext(ctx, task.ID); err != nil {
		t.Fatalf("cancelamento removeu task: %v", err)
	}
}

func TestTaskListCommandCloneCopiesPropertiesAtomicallyWithoutTasks(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "Origem", "descrição", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := SetTaskListViewModeWithContext(ctx, list.ID, "kanban"); err != nil {
		t.Fatal(err)
	}
	if err := SetTaskListValidationPolicyWithContext(ctx, list.ID, `{"task_code_regex":"^X"}`); err != nil {
		t.Fatal(err)
	}
	if err := SetTaskListCustomActionsWithContext(ctx, list.ID, `{"actions":[{"id":"refresh","label":"Atualizar","event":"x"}]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTaskWithContext(ctx, list.ID, "não copiar", "", "", "", nil); err != nil {
		t.Fatal(err)
	}
	_, fingerprint, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	clone, err := CommitTaskListCommandMutationWithContext(ctx, TaskListCommandMutationRequest{Operation: "clone", ID: list.ID, Title: "Cópia", ExpectedFingerprint: fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	if clone.PreferredViewMode != "kanban" || clone.ValidationPolicy == "" || clone.CustomActions == "" || clone.Description != "descrição" {
		t.Fatalf("propriedades não copiadas: %+v", clone)
	}
	tasks, err := GetTasksByTaskListIDWithContext(ctx, clone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("clone copiou tasks: %d", len(tasks))
	}
}

func TestTaskListCommandCloneRollbackLeavesNoPartialList(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "Origem", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	_, fingerprint, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER reject_command_clone_workflow BEFORE INSERT ON task_list_workflows BEGIN SELECT RAISE(ABORT, 'reject clone'); END").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP TRIGGER reject_command_clone_workflow").Error })
	if _, err := CommitTaskListCommandMutationWithContext(ctx, TaskListCommandMutationRequest{Operation: "clone", ID: list.ID, Title: "Cópia", ExpectedFingerprint: fingerprint}); err == nil {
		t.Fatal("clone com falha deveria retornar erro")
	}
	var count int64
	if err := db.Model(&TaskList{}).Where("user_id = ?", testUserID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("clone parcial persistido: %d listas", count)
	}
}

func TestTaskListCommandDeleteRollbackLeavesTasksAndNotes(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	list, err := CreateTaskListWithContext(ctx, "Excluir", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	task, err := CreateTaskWithContext(ctx, list.ID, "Card", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTaskNoteWithContext(ctx, task.ID, TaskNoteInternal, "nota", "", ""); err != nil {
		t.Fatal(err)
	}
	_, fingerprint, err := ReadTaskListCommandTargetWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER reject_command_delete BEFORE DELETE ON task_lists BEGIN SELECT RAISE(ABORT, 'reject delete'); END").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec("DROP TRIGGER reject_command_delete").Error })
	if _, err := CommitTaskListCommandMutationWithContext(ctx, TaskListCommandMutationRequest{Operation: "delete", ID: list.ID, ExpectedFingerprint: fingerprint}); err == nil {
		t.Fatal("delete com falha deveria retornar erro")
	}
	if _, err := GetTaskListWithContext(ctx, list.ID); err != nil {
		t.Fatalf("lista removida parcialmente: %v", err)
	}
	if _, err := GetTaskWithContext(ctx, task.ID); err != nil {
		t.Fatalf("task removida parcialmente: %v", err)
	}
	if notes, err := GetTaskNotesWithContext(ctx, task.ID); err != nil || len(notes) != 1 {
		t.Fatalf("notas removidas parcialmente: len=%d err=%v", len(notes), err)
	}
}

func TestTaskListCommandTransactionRestoresOriginalBusyTimeoutAfterCancellation(t *testing.T) {
	setupValidationPolicyTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	const originalBusyTimeout = 37
	if err := db.Exec("PRAGMA busy_timeout=37").Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err = withTaskListCommandTransaction(ctx, func(*gorm.DB) error {
		cancel()
		return errors.New("cancelar durante a transação")
	})
	if err == nil {
		t.Fatal("transação cancelada deveria falhar")
	}

	var busyTimeout int
	if err := db.Raw("PRAGMA busy_timeout").Scan(&busyTimeout).Error; err != nil {
		t.Fatal(err)
	}
	if busyTimeout != originalBusyTimeout {
		t.Fatalf("busy_timeout vazou após cancelamento: got=%d want=%d", busyTimeout, originalBusyTimeout)
	}
}
