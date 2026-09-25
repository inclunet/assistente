package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func editorModeManager(t *testing.T) *Manager {
	t.Helper()
	m := commandSnapshotManager(t)
	if err := m.AddTab(Tab{ID: "editor-mode-target", Type: TabTypeEditor, Title: "Editor"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetActiveTab("editor-mode-target"); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateTab("editor-mode-target", map[string]any{
		"state": map[string]any{"filePath": "notes.md", "displayMode": "markdown", "cursor": 7},
	}); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSetEditorModeForCommandAceitaTresModosEPreservaState(t *testing.T) {
	for _, mode := range []string{EditorModeMarkdown, EditorModeRich, EditorModeView} {
		t.Run(mode, func(t *testing.T) {
			m := editorModeManager(t)
			expected, err := m.CommandSnapshot()
			if err != nil {
				t.Fatal(err)
			}
			got, err := m.SetEditorModeForCommand(context.Background(), expected, mode)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil || got.ActiveTab().State["displayMode"] != mode || got.ActiveTab().State["filePath"] != "notes.md" || got.ActiveTab().State["cursor"] != 7 {
				t.Fatalf("estado alterado incorretamente: %#v", got.ActiveTab().State)
			}
			if got.ActiveTab().Title != m.Active().ActiveTab().Title {
				t.Fatal("campos da aba foram alterados")
			}
		})
	}
}

func TestSetEditorModeForCommandRecusaNaoEditorModoInvalidoEContextoCancelado(t *testing.T) {
	m := editorModeManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"", "plain", "MARKDOWN"} {
		if got, err := m.SetEditorModeForCommand(context.Background(), expected, mode); !errors.Is(err, ErrCommandEditorModeInvalid) || got != nil {
			t.Fatalf("modo %q: got=%#v err=%v", mode, got, err)
		}
	}

	nonEditor := editorModeManager(t)
	if err := nonEditor.AddTab(Tab{ID: "chat-target", Type: TabTypeChat}); err != nil {
		t.Fatal(err)
	}
	if err := nonEditor.SetActiveTab("chat-target"); err != nil {
		t.Fatal(err)
	}
	nonEditorSnapshot, err := nonEditor.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := nonEditor.SetEditorModeForCommand(context.Background(), nonEditorSnapshot, EditorModeView); !errors.Is(err, ErrCommandEditorModeUnsupportedType) || got != nil {
		t.Fatalf("aba não editor: got=%#v err=%v", got, err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := m.SetEditorModeForCommand(canceled, expected, EditorModeView); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatalf("cancelamento: got=%#v err=%v", got, err)
	}
}

func TestSetEditorModeForCommandRecusaSnapshotStaleABAReplayMesmoModo(t *testing.T) {
	m := editorModeManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateTab(expected.ActiveTabID, map[string]any{"state": map[string]any{"displayMode": EditorModeRich}}); err != nil {
		t.Fatal(err)
	}
	if got, err := m.SetEditorModeForCommand(context.Background(), expected, EditorModeRich); !errors.Is(err, ErrCommandEditorModeStale) || got != nil {
		t.Fatalf("snapshot stale: got=%#v err=%v", got, err)
	}

	m = editorModeManager(t)
	expected, err = m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetEditorModeForCommand(context.Background(), expected, EditorModeRich); err != nil {
		t.Fatal(err)
	}
	if got, err := m.SetEditorModeForCommand(context.Background(), expected, EditorModeRich); !errors.Is(err, ErrCommandEditorModeStale) || got != nil {
		t.Fatalf("replay mesmo modo: got=%#v err=%v", got, err)
	}
}

func TestSetEditorModeForCommandFalhaDePersistenciaFazRollback(t *testing.T) {
	root := t.TempDir()
	m := NewManager(filepath.Join(root, "home"))
	if err := m.Initialize(filepath.Join(root, "workspace")); err != nil {
		t.Fatal(err)
	}
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	beforeMode := m.Active().ActiveTab().State["displayMode"]
	beforeEpoch := m.commandEpoch
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	m.activePath = filepath.Join(blocker, "workspace.yaml")
	if got, err := m.SetEditorModeForCommand(context.Background(), expected, EditorModeView); err == nil || got != nil {
		t.Fatalf("persistência deveria falhar: got=%#v err=%v", got, err)
	}
	after, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if after != before || m.commandEpoch != beforeEpoch || m.Active().ActiveTab().State["displayMode"] != beforeMode {
		t.Fatalf("rollback incompleto: before=%#v after=%#v epoch=%d/%d", before, after, beforeEpoch, m.commandEpoch)
	}
}

func TestSetEditorModeForCommandCASConcorrenteSomenteUmaVez(t *testing.T) {
	m := editorModeManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, mode := range []string{EditorModeRich, EditorModeView} {
		mode := mode
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, callErr := m.SetEditorModeForCommand(context.Background(), expected, mode)
			results <- callErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var success, stale int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrCommandEditorModeStale):
			stale++
		default:
			t.Fatalf("erro concorrente inesperado: %v", err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("CAS concorrente: success=%d stale=%d", success, stale)
	}
}
