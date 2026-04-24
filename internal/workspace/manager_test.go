package workspace

import "testing"

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
	_ = m.AddTab(Tab{
		ID:    "tab-chat-1",
		Type:  TabTypeChat,
		Title: "Chat",
	})

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

	_ = m.AddTab(Tab{
		ID:    "tab-editor-1",
		Type:  TabTypeEditor,
		Title: "doc.txt",
		State: map[string]any{"filePath": "/home/user/doc.txt"},
	})
	_ = m.AddTab(Tab{
		ID:    "tab-editor-2",
		Type:  TabTypeEditor,
		Title: "notes.md",
		State: map[string]any{"filePath": "/tmp/notes.md"},
	})
	_ = m.AddTab(Tab{
		ID:    "tab-chat-1",
		Type:  TabTypeChat,
		Title: "Chat",
	})

	paths := m.OpenEditorFilePaths()
	if len(paths) != 2 {
		t.Fatalf("expected 2 editor paths, got %d: %v", len(paths), paths)
	}

	expected := map[string]bool{
		"/home/user/doc.txt": true,
		"/tmp/notes.md":      true,
	}
	for _, p := range paths {
		if !expected[p] {
			t.Errorf("unexpected path: %s", p)
		}
	}
}

func TestOpenEditorFilePaths_SkipsEmptyFilePath(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Editor tab sem filePath no state
	_ = m.AddTab(Tab{
		ID:    "tab-editor-empty",
		Type:  TabTypeEditor,
		Title: "empty",
		State: map[string]any{},
	})
	// Editor tab com filePath vazio
	_ = m.AddTab(Tab{
		ID:    "tab-editor-blank",
		Type:  TabTypeEditor,
		Title: "blank",
		State: map[string]any{"filePath": ""},
	})
	// Editor tab válido
	_ = m.AddTab(Tab{
		ID:    "tab-editor-valid",
		Type:  TabTypeEditor,
		Title: "valid",
		State: map[string]any{"filePath": "/path/to/file.go"},
	})

	paths := m.OpenEditorFilePaths()
	if len(paths) != 1 {
		t.Fatalf("expected 1 path, got %d: %v", len(paths), paths)
	}
	if paths[0] != "/path/to/file.go" {
		t.Errorf("expected /path/to/file.go, got %s", paths[0])
	}
}
