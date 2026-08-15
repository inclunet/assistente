package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/controllers"
	"assistente/internal/workspace"
)

func TestWorkspaceNotWired(t *testing.T) {
	t.Parallel()
	api := NewWorkspace()
	if _, err := api.GetActiveWorkspace(); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("GetActiveWorkspace: got %v", err)
	}
	if _, err := api.ListWorkspaces(); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("ListWorkspaces: got %v", err)
	}
	if _, err := api.CreateWorkspace("x"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("CreateWorkspace: got %v", err)
	}
	if _, err := api.SwitchWorkspace("id"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("SwitchWorkspace: got %v", err)
	}
	if err := api.RenameWorkspace("n"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("RenameWorkspace: got %v", err)
	}
	if err := api.DeleteWorkspace("id"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("DeleteWorkspace: got %v", err)
	}
	if err := api.SetWorkspaceProfile("p"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("SetWorkspaceProfile: got %v", err)
	}
	if err := api.SaveWorkspace(); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("SaveWorkspace: got %v", err)
	}
	if _, err := api.AddWorkspaceTab(workspace.Tab{}); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("AddWorkspaceTab: got %v", err)
	}
	if _, err := api.RemoveWorkspaceTab("t"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("RemoveWorkspaceTab: got %v", err)
	}
	if err := api.SetActiveWorkspaceTab("t"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("SetActiveWorkspaceTab: got %v", err)
	}
	if err := api.UpdateWorkspaceTab("t", nil); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("UpdateWorkspaceTab: got %v", err)
	}
	if err := api.ReorderWorkspaceTabs(nil); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("ReorderWorkspaceTabs: got %v", err)
	}
	if _, err := api.MoveWorkspaceTabTo("t", "w"); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("MoveWorkspaceTabTo: got %v", err)
	}
	if _, err := api.ExportWorkspace(); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("ExportWorkspace: got %v", err)
	}
	if _, err := api.ImportWorkspace(""); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("ImportWorkspace: got %v", err)
	}
}

func TestWorkspaceNilControllerIsNotWired(t *testing.T) {
	t.Parallel()
	api := NewWorkspace()
	AttachWorkspace(api, stubSession{}, nil)
	if _, err := api.ListWorkspaces(); !errors.Is(err, ErrWorkspaceNotWired) {
		t.Fatalf("ListWorkspaces com ctrl nil: got %v", err)
	}
}

// TestWorkspaceUsesWithUserNotRequireAuth cobre o fail-closed da borda.
func TestWorkspaceUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewWorkspace()
	AttachWorkspace(
		api,
		stubSession{err: semAuth},
		controllers.NewWorkspaceController(controllers.WorkspaceControllerConfig{}),
	)

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"GetActiveWorkspace", func() error {
			_, err := api.GetActiveWorkspace()
			return err
		}},
		{"ListWorkspaces", func() error {
			_, err := api.ListWorkspaces()
			return err
		}},
		{"CreateWorkspace", func() error {
			_, err := api.CreateWorkspace("x")
			return err
		}},
		{"SwitchWorkspace", func() error {
			_, err := api.SwitchWorkspace("id")
			return err
		}},
		{"RenameWorkspace", func() error {
			return api.RenameWorkspace("n")
		}},
		{"DeleteWorkspace", func() error {
			return api.DeleteWorkspace("id")
		}},
		{"SetWorkspaceProfile", func() error {
			return api.SetWorkspaceProfile("p")
		}},
		{"SaveWorkspace", func() error {
			return api.SaveWorkspace()
		}},
		{"AddWorkspaceTab", func() error {
			_, err := api.AddWorkspaceTab(workspace.Tab{ID: "t", Type: workspace.TabTypeChat, Title: "c"})
			return err
		}},
		{"RemoveWorkspaceTab", func() error {
			_, err := api.RemoveWorkspaceTab("t")
			return err
		}},
		{"SetActiveWorkspaceTab", func() error {
			return api.SetActiveWorkspaceTab("t")
		}},
		{"UpdateWorkspaceTab", func() error {
			return api.UpdateWorkspaceTab("t", map[string]any{"title": "x"})
		}},
		{"ReorderWorkspaceTabs", func() error {
			return api.ReorderWorkspaceTabs([]string{"t"})
		}},
		{"MoveWorkspaceTabTo", func() error {
			_, err := api.MoveWorkspaceTabTo("t", "w")
			return err
		}},
		{"ExportWorkspace", func() error {
			_, err := api.ExportWorkspace()
			return err
		}},
		{"ImportWorkspace", func() error {
			_, err := api.ImportWorkspace("name: x")
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

func TestWorkspaceUsesWithUserNotRequireAuthSource(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "workspace.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("workspace.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("workspace.go deve chamar WithUser(session,")
	}
}
