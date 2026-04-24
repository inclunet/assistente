package workspace

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenEditorFilePaths_NoActiveWorkspace(t *testing.T) {
	m := NewManager(t.TempDir())
	paths := m.OpenEditorFilePaths()
	if paths != nil {
		t.Errorf("expected nil without active workspace, got %v", paths)
	}
}

func TestOpenEditorFilePaths_NoEditorTabs(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Adiciona apenas aba de chat
	if err := m.AddTab(Tab{
		ID:    "tab-chat-1",
		Type:  TabTypeChat,
		Title: "Chat",
	}); err != nil {
		t.Fatalf("AddTab chat: %v", err)
	}

	paths := m.OpenEditorFilePaths()
	if len(paths) != 0 {
		t.Errorf("expected 0 paths without editor tabs, got %d", len(paths))
	}
}

func TestOpenEditorFilePaths_ReturnsEditorPaths(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	editorPath1 := "/home/user/doc.txt"
	editorPath2 := "/tmp/notes.md"
	if runtime.GOOS == "windows" {
		editorPath1 = `C:\Users\user\doc.txt`
		editorPath2 = `C:\tmp\notes.md`
	}

	if err := m.AddTab(Tab{
		ID:    "tab-editor-1",
		Type:  TabTypeEditor,
		Title: "doc.txt",
		State: map[string]any{"filePath": editorPath1},
	}); err != nil {
		t.Fatalf("AddTab editor1: %v", err)
	}
	if err := m.AddTab(Tab{
		ID:    "tab-editor-2",
		Type:  TabTypeEditor,
		Title: "notes.md",
		State: map[string]any{"filePath": editorPath2},
	}); err != nil {
		t.Fatalf("AddTab editor2: %v", err)
	}
	if err := m.AddTab(Tab{
		ID:    "tab-chat-1",
		Type:  TabTypeChat,
		Title: "Chat",
	}); err != nil {
		t.Fatalf("AddTab chat: %v", err)
	}

	paths := m.OpenEditorFilePaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 editor paths, got %d: %v", len(paths), paths)
	}

	expected := map[string]bool{
		filepath.Clean(editorPath1): true,
		filepath.Clean(editorPath2): true,
	}
	for _, p := range paths {
		if !expected[p] {
			t.Errorf("unexpected path: %s", p)
		}
	}
}

func TestOpenEditorFilePaths_SkipsEmptyAndRelativePaths(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	absPath := "/path/to/file.go"
	if runtime.GOOS == "windows" {
		absPath = `C:\path\to\file.go`
	}

	// Editor tab sem filePath no state
	if err := m.AddTab(Tab{
		ID:    "tab-editor-empty",
		Type:  TabTypeEditor,
		Title: "empty",
		State: map[string]any{},
	}); err != nil {
		t.Fatalf("AddTab empty: %v", err)
	}
	// Editor tab com filePath vazio
	if err := m.AddTab(Tab{
		ID:    "tab-editor-blank",
		Type:  TabTypeEditor,
		Title: "blank",
		State: map[string]any{"filePath": ""},
	}); err != nil {
		t.Fatalf("AddTab blank: %v", err)
	}
	// Editor tab com path relativo (deve ser filtrado)
	if err := m.AddTab(Tab{
		ID:    "tab-editor-relative",
		Type:  TabTypeEditor,
		Title: "relative",
		State: map[string]any{"filePath": "relative/path.txt"},
	}); err != nil {
		t.Fatalf("AddTab relative: %v", err)
	}
	// Editor tab válido (absoluto)
	if err := m.AddTab(Tab{
		ID:    "tab-editor-valid",
		Type:  TabTypeEditor,
		Title: "valid",
		State: map[string]any{"filePath": absPath},
	}); err != nil {
		t.Fatalf("AddTab valid: %v", err)
	}

	paths := m.OpenEditorFilePaths()
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
	}
	if paths[0] != filepath.Clean(absPath) {
		t.Errorf("expected %s, got %s", filepath.Clean(absPath), paths[0])
	}
}
