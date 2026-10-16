package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func editorFileManager(t *testing.T) *Manager {
	t.Helper()
	m := commandSnapshotManager(t)
	if err := m.AddTab(Tab{ID: "editor-file-target", Type: TabTypeEditor, Title: "Editor", State: map[string]any{"draftId": "draft-1"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetActiveTab("editor-file-target"); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCommitEditorFileForCommandOpenReusesExistingOrCreatesEditor(t *testing.T) {
	t.Run("existing", func(t *testing.T) {
		m := editorFileManager(t)
		path := filepath.Join(t.TempDir(), "notes.md")
		if err := m.UpdateTab("editor-file-target", map[string]any{"state": map[string]any{"filePath": path}}); err != nil {
			t.Fatal(err)
		}
		if err := m.AddTab(Tab{ID: "other-editor", Type: TabTypeEditor, Title: "Other", State: map[string]any{"filePath": filepath.Join(t.TempDir(), "other.md")}}); err != nil {
			t.Fatal(err)
		}
		if err := m.SetActiveTab("other-editor"); err != nil {
			t.Fatal(err)
		}
		expected, err := m.CommandSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		got, tabID, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationOpen, path, "new-open-tab", nil)
		if err != nil || got == nil || written || tabID != "editor-file-target" {
			t.Fatalf("open existente: got=%#v tabID=%q written=%v err=%v", got, tabID, written, err)
		}
		if got.Tabs.Active != "editor-file-target" || got.FindTab("new-open-tab") != nil {
			t.Fatalf("open não reutilizou editor: active=%q tabs=%#v", got.Tabs.Active, got.Tabs.Items)
		}
	})

	t.Run("new", func(t *testing.T) {
		m := editorFileManager(t)
		expected, err := m.CommandSnapshot()
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "new.md")
		got, tabID, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationOpen, path, "new-open-tab", nil)
		if err != nil || got == nil || written || tabID != "new-open-tab" || got.Tabs.Active != tabID {
			t.Fatalf("open novo: got=%#v tabID=%q written=%v err=%v", got, tabID, written, err)
		}
		tab := got.FindTab("new-open-tab")
		if tab == nil || tab.Type != TabTypeEditor || tab.Title != "new.md" || tab.State["filePath"] != path {
			t.Fatalf("aba nova incorreta: %#v", tab)
		}
	})
}

func TestCommitEditorFileForCommandSaveWritesThenPersistsOnlyPathMetadata(t *testing.T) {
	m := editorFileManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "saved.md")
	var calls atomic.Int32
	got, tabID, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationSave, path, "", func(context.Context) error {
		calls.Add(1)
		return nil
	})
	if err != nil || got == nil || !written || calls.Load() != 1 || tabID != expected.ActiveTabID {
		t.Fatalf("save: got=%#v tabID=%q written=%v calls=%d err=%v", got, tabID, written, calls.Load(), err)
	}
	tab := got.ActiveTab()
	if tab.State["filePath"] != path || tab.Title != "saved.md" || tab.State["draftId"] != "draft-1" {
		t.Fatalf("metadata/save state incorretos: %#v", tab.State)
	}
	if _, err := os.Stat(filepath.Join(m.ActivePath(), ".assistente", "workspace.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestCommitEditorFileForCommandCopyLeavesWorkspaceUnchanged(t *testing.T) {
	m := editorFileManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before := m.Active()
	var calls atomic.Int32
	got, tabID, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationCopy, filepath.Join(t.TempDir(), "copy.md"), "", func(context.Context) error {
		calls.Add(1)
		return nil
	})
	if err != nil || got == nil || !written || calls.Load() != 1 || tabID != expected.ActiveTabID {
		t.Fatalf("save_copy: got=%#v tabID=%q written=%v calls=%d err=%v", got, tabID, written, calls.Load(), err)
	}
	if got.Tabs.Active != before.Tabs.Active || got.ActiveTab().State["draftId"] != before.ActiveTab().State["draftId"] || got.ActiveTab().State["filePath"] != before.ActiveTab().State["filePath"] {
		t.Fatalf("copy alterou workspace: before=%#v after=%#v", before.ActiveTab(), got.ActiveTab())
	}
}

func TestCommitEditorFileForCommandHoldsLockDuringWriteWithoutDeadlock(t *testing.T) {
	m := editorFileManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	blocked := make(chan struct{})
	got, tabID, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationSave, filepath.Join(t.TempDir(), "locked.md"), "", func(context.Context) error {
		go func() {
			close(started)
			_ = m.SetActiveTab(expected.ActiveTabID)
			close(blocked)
		}()
		<-started
		select {
		case <-blocked:
			t.Fatal("mutação concorrente atravessou o lock durante o callback")
		case <-time.After(20 * time.Millisecond):
		}
		return nil
	})
	if err != nil || got == nil || !written || tabID != expected.ActiveTabID {
		t.Fatalf("save com concorrência: got=%#v tabID=%q written=%v err=%v", got, tabID, written, err)
	}
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("goroutine concorrente não concluiu após liberar o lock")
	}
}

func TestCommitEditorFileForCommandCASAndTypeGuards(t *testing.T) {
	m := commandSnapshotManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationSave, filepath.Join(t.TempDir(), "x.md"), "", func(context.Context) error { t.Fatal("callback não deveria executar"); return nil }); !errors.Is(err, ErrCommandEditorFileUnsupportedType) || written {
		t.Fatalf("aba não editor: written=%v err=%v", written, err)
	}

	m = editorFileManager(t)
	expected, err = m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateTab(expected.ActiveTabID, map[string]any{"state": map[string]any{"draftId": "changed"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationSave, filepath.Join(t.TempDir(), "x.md"), "", func(context.Context) error { t.Fatal("callback não deveria executar"); return nil }); !errors.Is(err, ErrCommandEditorFileStale) || written {
		t.Fatalf("snapshot stale: written=%v err=%v", written, err)
	}
}

func TestCommitEditorFileForCommandWriteAndPersistenceFailuresAreHonest(t *testing.T) {
	m := editorFileManager(t)
	expected, err := m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	before := m.Active()
	writeErr := errors.New("write failed")
	if got, _, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationSave, filepath.Join(t.TempDir(), "x.md"), "", func(context.Context) error { return writeErr }); !errors.Is(err, writeErr) || got != nil || written {
		t.Fatalf("falha de escrita: got=%#v written=%v err=%v", got, written, err)
	}
	if m.Active().ActiveTab().State["draftId"] != before.ActiveTab().State["draftId"] {
		t.Fatal("falha de escrita alterou metadata")
	}

	expected, err = m.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	m.activePath = filepath.Join(blocker, "workspace.yaml")
	got, _, written, err := m.CommitEditorFileForCommand(context.Background(), expected, EditorFileOperationSave, filepath.Join(t.TempDir(), "x.md"), "", func(context.Context) error { return nil })
	if err == nil || got != nil || !written {
		t.Fatalf("falha de persistência: got=%#v written=%v err=%v", got, written, err)
	}
	if m.Active().ActiveTab().State["draftId"] != before.ActiveTab().State["draftId"] {
		t.Fatal("rollback de metadata incompleto")
	}
}
