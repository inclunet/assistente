package tasklist

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/database"
)

// Estes testes garantem que o repositório falha fechado (fail-closed) quando o
// contexto não traz userID. O cenário simula a chamada feita antes do login ou
// por um caller que esqueceu de propagar o contexto autenticado — exatamente o
// vetor de ataque levantado na revisão do AEP-0052.

func TestDBStore_UnauthenticatedErrors(t *testing.T) {
	store := NewDBStore()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		// ── Task List ────────────────────────────────────────────────────
		{"CreateTaskList", func() error {
			_, err := store.CreateTaskList(ctx, "L", "", nil, "")
			return err
		}},
		{"GetTaskList", func() error {
			_, err := store.GetTaskList(ctx, "x")
			return err
		}},
		{"GetAllTaskLists", func() error {
			_, err := store.GetAllTaskLists(ctx)
			return err
		}},
		{"UpdateTaskList", func() error {
			return store.UpdateTaskList(ctx, "x", "T", "")
		}},
		{"UpdateTaskListFull", func() error {
			return store.UpdateTaskListFull(ctx, "x", "T", "", "", nil)
		}},
		{"ResolveTaskListRef", func() error {
			_, err := store.ResolveTaskListRef(ctx, nil, "slug")
			return err
		}},
		{"SetTaskListValidationPolicy", func() error {
			return store.SetTaskListValidationPolicy(ctx, "x", "{}")
		}},
		{"SetTaskListViewMode", func() error {
			return store.SetTaskListViewMode(ctx, "x", "list")
		}},
		{"CloneTaskList", func() error {
			_, err := store.CloneTaskList(ctx, "x", "Novo")
			return err
		}},
		{"ClearTaskList", func() error {
			return store.ClearTaskList(ctx, "x")
		}},
		{"DeleteTaskList", func() error {
			return store.DeleteTaskList(ctx, "x")
		}},
		{"GetTaskListStats", func() error {
			_, err := store.GetTaskListStats(ctx, "x")
			return err
		}},
		{"GetTaskListWithHierarchy", func() error {
			_, err := store.GetTaskListWithHierarchy(ctx, "x")
			return err
		}},
		// ── Workflow ────────────────────────────────────────────────────
		{"GetWorkflow", func() error {
			_, err := store.GetWorkflow(ctx, "x")
			return err
		}},
		{"UpdateWorkflow", func() error {
			return store.UpdateWorkflow(ctx, "x", nil, nil)
		}},
		{"UpdateWorkflowFull", func() error {
			return store.UpdateWorkflowFull(ctx, "x", nil, nil, 0, nil)
		}},
		{"GetTaskCountsByStatus", func() error {
			_, err := store.GetTaskCountsByStatus(ctx, "x")
			return err
		}},
		{"ReorderWorkflowStatuses", func() error {
			return store.ReorderWorkflowStatuses(ctx, "x", nil)
		}},
		{"ValidateStatusTransition", func() error {
			return store.ValidateStatusTransition(ctx, "x", 1, 2)
		}},
		// ── Task ────────────────────────────────────────────────────────
		{"CreateTask", func() error {
			_, err := store.CreateTask(ctx, "x", "T", "", "", "", nil)
			return err
		}},
		{"CreateTaskFull", func() error {
			_, err := store.CreateTaskFull(ctx, "x", "T", "", "", "", "", "", "", "", nil)
			return err
		}},
		{"GetTask", func() error {
			_, err := store.GetTask(ctx, "x")
			return err
		}},
		{"GetTasksByTaskListID", func() error {
			_, err := store.GetTasksByTaskListID(ctx, "x")
			return err
		}},
		{"GetTasksByStatus", func() error {
			_, err := store.GetTasksByStatus(ctx, "x", 1)
			return err
		}},
		{"FindTaskByCode", func() error {
			_, err := store.FindTaskByCode(ctx, "x", "ABC-1")
			return err
		}},
		{"ResolveTaskRef", func() error {
			_, err := store.ResolveTaskRef(ctx, nil, "slug", nil, "ABC-1")
			return err
		}},
		{"ResolveTaskIDByTaskCode", func() error {
			_, err := store.ResolveTaskIDByTaskCode(ctx, nil, "ABC-1")
			return err
		}},
		{"UpdateTask", func() error {
			return store.UpdateTask(ctx, "x", "T", "", "", "")
		}},
		{"UpdateTaskFull", func() error {
			return store.UpdateTaskFull(ctx, "x", "T", "", "", "", "", "", "", "")
		}},
		{"UpdateTaskAssignee", func() error {
			return store.UpdateTaskAssignee(ctx, "x", "n", "id")
		}},
		{"UpdateTaskStatus", func() error {
			return store.UpdateTaskStatus(ctx, "x", 1)
		}},
		{"ReorderTasks", func() error {
			return store.ReorderTasks(ctx, "x", 1, nil)
		}},
		{"PromoteTask", func() error {
			return store.PromoteTask(ctx, "x")
		}},
		{"DemoteTask", func() error {
			return store.DemoteTask(ctx, "x", "p")
		}},
		{"MoveTaskToList", func() error {
			_, err := store.MoveTaskToList(ctx, "x", "y")
			return err
		}},
		{"DeleteTask", func() error {
			return store.DeleteTask(ctx, "x")
		}},
		{"GetSubtasks", func() error {
			_, err := store.GetSubtasks(ctx, "x")
			return err
		}},
		// ── Task Note ───────────────────────────────────────────────────
		{"CreateTaskNote", func() error {
			_, err := store.CreateTaskNote(ctx, "x", database.TaskNoteAgent, "c", "", "")
			return err
		}},
		{"UpsertTaskNoteByExternal", func() error {
			_, _, err := store.UpsertTaskNoteByExternal(ctx, database.UpsertTaskNoteByExternalParams{TaskID: "x"})
			return err
		}},
		{"GetTaskNotes", func() error {
			_, err := store.GetTaskNotes(ctx, "x")
			return err
		}},
		{"GetTaskNote", func() error {
			_, err := store.GetTaskNote(ctx, "x")
			return err
		}},
		{"UpdateTaskNote", func() error {
			return store.UpdateTaskNote(ctx, "x", "c")
		}},
		{"DeleteTaskNote", func() error {
			return store.DeleteTaskNote(ctx, "x")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if !errors.Is(err, database.ErrUserScopeRequired) {
				t.Fatalf("esperava ErrUserScopeRequired, obteve %v", err)
			}
		})
	}
}
