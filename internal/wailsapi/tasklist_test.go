package wailsapi

import (
	"errors"
	"testing"

	"assistente/controllers"
)

func TestTasklistNotWired(t *testing.T) {
	t.Parallel()
	api := NewTasklist()
	if _, err := api.GetAllTaskLists(); !errors.Is(err, ErrTasklistNotWired) {
		t.Fatalf("GetAllTaskLists: got %v", err)
	}
	if _, err := api.CreateTaskList("t", "d", ""); !errors.Is(err, ErrTasklistNotWired) {
		t.Fatalf("CreateTaskList: got %v", err)
	}
	if err := api.DeleteTaskList("id"); !errors.Is(err, ErrTasklistNotWired) {
		t.Fatalf("DeleteTaskList: got %v", err)
	}
	if _, err := api.GetTask("id"); !errors.Is(err, ErrTasklistNotWired) {
		t.Fatalf("GetTask: got %v", err)
	}
	if _, err := api.GetWorkflow("list"); !errors.Is(err, ErrTasklistNotWired) {
		t.Fatalf("GetWorkflow: got %v", err)
	}
	if _, err := api.GetTaskNotes("task"); !errors.Is(err, ErrTasklistNotWired) {
		t.Fatalf("GetTaskNotes: got %v", err)
	}
	if _, err := api.GetTaskListStats("list"); !errors.Is(err, ErrTasklistNotWired) {
		t.Fatalf("GetTaskListStats: got %v", err)
	}
}

func TestTasklistNilControllerIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewTasklist()
	AttachTasklist(api, stubSession{}, nil)
	if _, err := api.GetAllTaskLists(); !errors.Is(err, ErrTasklistNotWired) {
		t.Fatalf("GetAllTaskLists com ctrl nil: got %v", err)
	}
}

// TestTasklistUsesWithUserNotRequireAuth cobre o fail-closed da borda:
// sem contexto autenticado o domínio não roda (TaskSvc nil panica se chamado)
// e o erro da sessão sobe como veio.
func TestTasklistUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewTasklist()
	// TaskSvc nil: se WithUser for esquecido e o controller for chamado, panica.
	AttachTasklist(api, stubSession{err: semAuth}, controllers.NewTaskListController(controllers.TaskListControllerConfig{}))

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"GetAllTaskLists", func() error {
			_, err := api.GetAllTaskLists()
			return err
		}},
		{"CreateTaskList", func() error {
			_, err := api.CreateTaskList("t", "d", "")
			return err
		}},
		{"GetTaskList", func() error {
			_, err := api.GetTaskList("id")
			return err
		}},
		{"UpdateTaskList", func() error {
			return api.UpdateTaskList("id", "t", "d")
		}},
		{"DeleteTaskList", func() error {
			return api.DeleteTaskList("id")
		}},
		{"GetWorkflow", func() error {
			_, err := api.GetWorkflow("list")
			return err
		}},
		{"UpdateWorkflowFull", func() error {
			return api.UpdateWorkflowFull("list", nil, nil, 0, nil)
		}},
		{"CreateTask", func() error {
			_, err := api.CreateTask("list", "t", "", "", "", nil)
			return err
		}},
		{"GetTask", func() error {
			_, err := api.GetTask("id")
			return err
		}},
		{"UpdateTaskStatus", func() error {
			return api.UpdateTaskStatus("id", 1)
		}},
		{"DeleteTask", func() error {
			return api.DeleteTask("id")
		}},
		{"CreateTaskNote", func() error {
			_, err := api.CreateTaskNote("task", 0, "n", "", "")
			return err
		}},
		{"GetTaskNotes", func() error {
			_, err := api.GetTaskNotes("task")
			return err
		}},
		{"GetTaskListStats", func() error {
			_, err := api.GetTaskListStats("list")
			return err
		}},
		{"GetTaskListWithHierarchy", func() error {
			_, err := api.GetTaskListWithHierarchy("id")
			return err
		}},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
		})
	}
}
