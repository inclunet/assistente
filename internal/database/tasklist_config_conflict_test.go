package database

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func workflowSnapshot(t *testing.T, ctx context.Context, taskListID string) TaskListWorkflowSnapshot {
	t.Helper()
	wf, err := GetWorkflowWithContext(ctx, taskListID)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	var snap TaskListWorkflowSnapshot
	if err := json.Unmarshal([]byte(wf.Statuses), &snap.Statuses); err != nil {
		t.Fatalf("statuses: %v", err)
	}
	if err := json.Unmarshal([]byte(wf.AllowedTransitions), &snap.Transitions); err != nil {
		t.Fatalf("transitions: %v", err)
	}
	snap.InitialStatusID = wf.InitialStatusID
	return snap
}

func TestUpdateWorkflowFullChecked_SavesWhenUnchanged(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	base := workflowSnapshot(t, ctx, tl.ID)

	next := append([]TaskListWorkflowStatus(nil), base.Statuses...)
	next[0].Label = "Renomeado"
	if err := UpdateWorkflowFullCheckedWithContext(ctx, tl.ID, base, next, base.Transitions, base.InitialStatusID, nil); err != nil {
		t.Fatalf("expected save, got %v", err)
	}
	if got := workflowSnapshot(t, ctx, tl.ID); got.Statuses[0].Label != "Renomeado" {
		t.Fatalf("label not saved: %#v", got.Statuses)
	}
}

func TestUpdateWorkflowFullChecked_IgnoresOrderOfArraysAndEmptyTransitions(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	base := workflowSnapshot(t, ctx, tl.ID)

	equivalent := TaskListWorkflowSnapshot{InitialStatusID: base.InitialStatusID, Transitions: map[int][]int{}}
	for i := len(base.Statuses) - 1; i >= 0; i-- {
		equivalent.Statuses = append(equivalent.Statuses, base.Statuses[i])
	}
	for from, to := range base.Transitions {
		reversed := make([]int, 0, len(to))
		for i := len(to) - 1; i >= 0; i-- {
			reversed = append(reversed, to[i])
		}
		equivalent.Transitions[from] = reversed
	}
	for _, s := range base.Statuses {
		if _, ok := equivalent.Transitions[s.ID]; !ok {
			equivalent.Transitions[s.ID] = []int{}
		}
	}

	if err := UpdateWorkflowFullCheckedWithContext(ctx, tl.ID, equivalent, base.Statuses, base.Transitions, base.InitialStatusID, nil); err != nil {
		t.Fatalf("equivalent snapshot must not conflict, got %v", err)
	}
}

func TestUpdateWorkflowFullChecked_ConflictDoesNotWriteNorMigrate(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	stale := workflowSnapshot(t, ctx, tl.ID)
	if len(stale.Statuses) < 2 {
		t.Fatalf("default workflow needs at least 2 statuses, got %d", len(stale.Statuses))
	}

	// Outra origem (outra aba ou o agente) altera o workflow.
	changed := append([]TaskListWorkflowStatus(nil), stale.Statuses...)
	changed[0].Color = "var(--color-danger)"
	if err := UpdateWorkflowFullWithContext(ctx, tl.ID, changed, stale.Transitions, stale.InitialStatusID, nil); err != nil {
		t.Fatalf("concurrent write: %v", err)
	}

	task, err := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	removed := task.StatusID
	var kept []TaskListWorkflowStatus
	target := 0
	for _, s := range stale.Statuses {
		if s.ID != removed {
			kept = append(kept, s)
			if target == 0 {
				target = s.ID
			}
		}
	}
	transitions := map[int][]int{}
	for from, to := range stale.Transitions {
		if from == removed {
			continue
		}
		for _, id := range to {
			if id != removed {
				transitions[from] = append(transitions[from], id)
			}
		}
	}

	err = UpdateWorkflowFullCheckedWithContext(ctx, tl.ID, stale, kept, transitions, target, map[int]int{removed: target})
	if !errors.Is(err, ErrTaskListConfigConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if !strings.HasPrefix(err.Error(), "TASKLIST_CONFIG_CONFLICT") {
		t.Fatalf("frontend depends on the stable prefix, got %q", err.Error())
	}
	if got := workflowSnapshot(t, ctx, tl.ID); got.Statuses[0].Color != "var(--color-danger)" || len(got.Statuses) != len(stale.Statuses) {
		t.Fatalf("conflict must not overwrite the workflow: %#v", got.Statuses)
	}
	reloaded, err := GetTaskWithContext(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if reloaded.StatusID != removed {
		t.Fatalf("conflict must not migrate tasks: status %d, want %d", reloaded.StatusID, removed)
	}
}

func TestSetCustomActionsChecked_SavesWhenUnchangedAndIgnoresFormatting(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")

	if err := SetTaskListCustomActionsCheckedWithContext(ctx, tl.ID, "", `{"actions":[{"id":"a","label":"A","link":"x"}]}`); err != nil {
		t.Fatalf("first save from empty: %v", err)
	}
	// Mesmo conteúdo com outra formatação e campos em outra ordem.
	expected := `{ "actions": [ { "link": "x", "label": "A", "id": "a" } ] }`
	if err := SetTaskListCustomActionsCheckedWithContext(ctx, tl.ID, expected, `{"actions":[{"id":"b","label":"B","link":"y"}]}`); err != nil {
		t.Fatalf("equivalent expected must not conflict: %v", err)
	}
	ca, _ := LoadTaskListCustomActionsWithContext(ctx, tl.ID)
	if len(ca.Actions) != 1 || ca.Actions[0].ID != "b" {
		t.Fatalf("save mismatch: %#v", ca)
	}
}

func TestSetCustomActionsChecked_ConflictDoesNotWrite(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	stale := `{"actions":[{"id":"a","label":"A","link":"x"}]}`
	if err := SetTaskListCustomActionsWithContext(ctx, tl.ID, stale); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := SetTaskListCustomActionsWithContext(ctx, tl.ID, `{"actions":[{"id":"agente","label":"Do agente","link":"z"}]}`); err != nil {
		t.Fatalf("concurrent write: %v", err)
	}

	err := SetTaskListCustomActionsCheckedWithContext(ctx, tl.ID, stale, `{"actions":[{"id":"a","label":"A2","link":"x"}]}`)
	if !errors.Is(err, ErrTaskListConfigConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	ca, _ := LoadTaskListCustomActionsWithContext(ctx, tl.ID)
	if len(ca.Actions) != 1 || ca.Actions[0].ID != "agente" {
		t.Fatalf("conflict must not overwrite: %#v", ca)
	}
}

// setupConcurrentConfigDB abre um banco em arquivo com WAL e várias conexões,
// como o do app, para gravações verificadas disputarem o lock de verdade.
func setupConcurrentConfigDB(t *testing.T) context.Context {
	return setupConcurrentConfigDBWithBusyTimeout(t, 1)
}

// setupWaitingConfigDB espera pelo lock como o banco do app, para uma
// gravação sem lock próprio concluir com o que leu antes em vez de falhar
// com SQLITE_BUSY.
func setupWaitingConfigDB(t *testing.T) context.Context {
	return setupConcurrentConfigDBWithBusyTimeout(t, 5000)
}

func setupConcurrentConfigDBWithBusyTimeout(t *testing.T, busyTimeoutMS int) context.Context {
	t.Helper()
	previous := DB()
	path := t.TempDir() + "/tasklist-config-concurrency.db"
	dsn := "file:" + path + "?_pragma=busy_timeout(" + strconv.Itoa(busyTimeoutMS) + ")&_pragma=journal_mode(WAL)"
	testDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	if err := testDB.AutoMigrate(&TaskList{}, &TaskListWorkflow{}, &Task{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	SetDB(testDB)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		SetDB(previous)
	})
	return WithUserID(context.Background(), "user-concurrency")
}

// runConcurrently dispara as gravações ao mesmo tempo e devolve os erros.
func runConcurrently(n int, write func(i int) error) []error {
	errs := make([]error, n)
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(n)
	done.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer done.Done()
			ready.Done()
			<-start
			errs[i] = write(i)
		}(i)
	}
	ready.Wait()
	close(start)
	done.Wait()
	return errs
}

func assertOneWinnerRestConflict(t *testing.T, errs []error) {
	t.Helper()
	winners := 0
	for i, err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrTaskListConfigConflict):
		default:
			t.Fatalf("gravação %d: esperado sucesso ou conflito, veio %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("exatamente uma gravação sobre a mesma base deve vencer, venceram %d", winners)
	}
}

func TestCheckedWritesUnderConcurrencyYieldConflictNotBusy(t *testing.T) {
	ctx := setupConcurrentConfigDB(t)
	tl, err := CreateTaskListWithContext(ctx, "L", "", nil, "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	const writers = 8

	errs := runConcurrently(writers, func(i int) error {
		return SetTaskListCustomActionsCheckedWithContext(ctx, tl.ID, "",
			`{"actions":[{"id":"a`+strconv.Itoa(i)+`","label":"A","link":"x"}]}`)
	})
	assertOneWinnerRestConflict(t, errs)

	base := workflowSnapshot(t, ctx, tl.ID)
	errs = runConcurrently(writers, func(i int) error {
		next := append([]TaskListWorkflowStatus(nil), base.Statuses...)
		next[0].Label = "Gravação " + strconv.Itoa(i)
		return UpdateWorkflowFullCheckedWithContext(ctx, tl.ID, base, next, base.Transitions, base.InitialStatusID, nil)
	})
	assertOneWinnerRestConflict(t, errs)
}

// writeWhileLocked segura o lock de escrita numa outra conexão, dispara write
// e, enquanto ela espera pelo lock, aplica a mudança concorrente e faz COMMIT.
// Devolve o resultado de write.
func writeWhileLocked(t *testing.T, write func() error, concurrentSQL string, args ...any) error {
	t.Helper()
	sqlDB, err := DB().DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	lockConn, err := sqlDB.Conn(context.Background())
	if err != nil {
		t.Fatalf("lock conn: %v", err)
	}
	defer func() { _ = lockConn.Close() }()
	if _, err := lockConn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("begin: %v", err)
	}

	result := make(chan error, 1)
	go func() { result <- write() }()
	time.Sleep(100 * time.Millisecond)
	if _, err := lockConn.ExecContext(context.Background(), concurrentSQL, args...); err != nil {
		t.Fatalf("mudança concorrente: %v", err)
	}
	if _, err := lockConn.ExecContext(context.Background(), "COMMIT"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	select {
	case err = <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("a gravação não terminou após liberar o lock")
		return nil
	}
}

// removeStatusSQL tira um status do workflow e passa o status inicial para
// outro, como uma edição concorrente da configuração.
func removeStatusSQL(t *testing.T, base TaskListWorkflowSnapshot, removed, newInitial int) (string, []any) {
	t.Helper()
	var kept []TaskListWorkflowStatus
	for _, s := range base.Statuses {
		if s.ID != removed {
			kept = append(kept, s)
		}
	}
	statuses, err := json.Marshal(kept)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return "UPDATE task_list_workflows SET statuses = ?, allowed_transitions = ?, initial_status_id = ? WHERE task_list_id = ?",
		[]any{string(statuses), "{}", newInitial}
}

func TestCreateTask_ReadsInitialStatusUnderLock(t *testing.T) {
	ctx := setupWaitingConfigDB(t)

	for _, tc := range []struct {
		name   string
		create func(taskListID string) (*Task, error)
	}{
		{"CreateTask", func(taskListID string) (*Task, error) {
			return CreateTaskWithContext(ctx, taskListID, "T", "", "", "", nil)
		}},
		{"CreateTaskFull", func(taskListID string) (*Task, error) {
			return CreateTaskFullWithContext(ctx, taskListID, "T", "", "", "", "", "", "", "", nil)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tl, err := CreateTaskListWithContext(ctx, tc.name, "", nil, "")
			if err != nil {
				t.Fatalf("create list: %v", err)
			}
			base := workflowSnapshot(t, ctx, tl.ID)
			oldInitial := base.InitialStatusID
			newInitial := base.Statuses[len(base.Statuses)-1].ID
			if newInitial == oldInitial {
				t.Fatalf("o novo status inicial precisa ser diferente do atual")
			}
			query, args := removeStatusSQL(t, base, oldInitial, newInitial)

			var task *Task
			err = writeWhileLocked(t, func() error {
				var err error
				task, err = tc.create(tl.ID)
				return err
			}, query, append(args, tl.ID)...)
			if err != nil {
				t.Fatalf("create task: %v", err)
			}
			stored, err := GetTaskWithContext(ctx, task.ID)
			if err != nil {
				t.Fatalf("get task: %v", err)
			}
			if stored.StatusID != newInitial {
				t.Fatalf("a task deve nascer no status inicial vigente %d, nasceu em %d", newInitial, stored.StatusID)
			}
		})
	}
}

func TestUpdateTaskStatus_ValidatesWorkflowUnderLock(t *testing.T) {
	ctx := setupWaitingConfigDB(t)
	tl, err := CreateTaskListWithContext(ctx, "L", "", nil, "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	base := workflowSnapshot(t, ctx, tl.ID)
	task, err := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	target := base.Statuses[len(base.Statuses)-1].ID
	if err := ValidateStatusTransitionWithContext(ctx, tl.ID, task.StatusID, target); err != nil {
		t.Fatalf("a transição precisa ser válida antes da edição concorrente: %v", err)
	}
	query, args := removeStatusSQL(t, base, target, task.StatusID)

	err = writeWhileLocked(t, func() error {
		return UpdateTaskStatusWithContext(ctx, task.ID, target)
	}, query, append(args, tl.ID)...)
	if err == nil || !strings.Contains(err.Error(), "não existe no workflow") {
		t.Fatalf("mover para status removido no intervalo deve falhar, veio %v", err)
	}
	stored, err := GetTaskWithContext(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if stored.StatusID != task.StatusID {
		t.Fatalf("a task não pode apontar para o status removido: %d", stored.StatusID)
	}
}

func TestUpdateWorkflowFull_RevalidatesTasksUnderLock(t *testing.T) {
	ctx := setupConcurrentConfigDB(t)
	tl, err := CreateTaskListWithContext(ctx, "L", "", nil, "")
	if err != nil {
		t.Fatalf("create list: %v", err)
	}
	base := workflowSnapshot(t, ctx, tl.ID)
	task, err := CreateTaskWithContext(ctx, tl.ID, "T", "", "", "", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	removed := base.Statuses[len(base.Statuses)-1].ID
	if task.StatusID == removed {
		t.Fatalf("a task precisa começar fora do status removido")
	}
	kept := base.Statuses[:len(base.Statuses)-1]
	transitions := map[int][]int{}
	for _, s := range kept {
		transitions[s.ID] = []int{}
	}

	// Enquanto a gravação espera pelo lock, a task passa para o status que
	// será removido.
	err = writeWhileLocked(t, func() error {
		return UpdateWorkflowFullCheckedWithContext(ctx, tl.ID, base, kept, transitions, kept[0].ID, nil)
	}, "UPDATE tasks SET status_id = ? WHERE id = ?", removed, task.ID)
	if err == nil || !strings.Contains(err.Error(), "em uso") {
		t.Fatalf("remover status com task criada no intervalo deve falhar por status em uso, veio %v", err)
	}
	if got := workflowSnapshot(t, ctx, tl.ID); len(got.Statuses) != len(base.Statuses) {
		t.Fatalf("o status não pode ter sido removido: %#v", got.Statuses)
	}
}

func TestSetCustomActionsChecked_InvalidExpectedRejected(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	err := SetTaskListCustomActionsCheckedWithContext(ctx, tl.ID, "{", "")
	if err == nil || errors.Is(err, ErrTaskListConfigConflict) {
		t.Fatalf("invalid expected must be a validation error, got %v", err)
	}
}
