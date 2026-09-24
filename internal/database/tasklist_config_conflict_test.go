package database

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
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

func TestSetCustomActionsChecked_InvalidExpectedRejected(t *testing.T) {
	setupValidationPolicyTestDB(t)
	ctx := testCtx()
	tl, _ := CreateTaskListWithContext(ctx, "L", "", nil, "")
	err := SetTaskListCustomActionsCheckedWithContext(ctx, tl.ID, "{", "")
	if err == nil || errors.Is(err, ErrTaskListConfigConflict) {
		t.Fatalf("invalid expected must be a validation error, got %v", err)
	}
}
