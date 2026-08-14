package workspace

import (
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestInitialize_NewWorkspaceHasActiveChatTab(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	ws := m.Active()
	if ws == nil {
		t.Fatal("expected active workspace")
	}
	if len(ws.Tabs.Items) != 1 {
		t.Fatalf("expected one default tab, got %d", len(ws.Tabs.Items))
	}

	tab := ws.Tabs.Items[0]
	if tab.Type != TabTypeChat {
		t.Fatalf("expected default chat tab, got %q", tab.Type)
	}
	if ws.Tabs.Active != tab.ID {
		t.Fatalf("expected tab %q to be active, got %q", tab.ID, ws.Tabs.Active)
	}
}

func TestInitialize_RepairsPersistedWorkspaceWithoutTabs(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	m := NewManager(homeDir)
	if err := m.saveWorkspace(&Workspace{
		ID:   "ws-empty",
		Name: "Empty",
		Tabs: TabsState{Items: []Tab{}},
	}, workDir); err != nil {
		t.Fatalf("saveWorkspace: %v", err)
	}

	if err := m.Initialize(workDir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	ws := m.Active()
	if ws == nil || len(ws.Tabs.Items) != 1 {
		t.Fatalf("expected repaired workspace with one tab, got %#v", ws)
	}
	if ws.Tabs.Active != ws.Tabs.Items[0].ID {
		t.Fatalf("expected repaired tab to be active, got %q", ws.Tabs.Active)
	}

	reloaded := NewManager(homeDir)
	if err := reloaded.Initialize(workDir); err != nil {
		t.Fatalf("reload Initialize: %v", err)
	}
	if got := reloaded.Active(); got == nil || len(got.Tabs.Items) != 1 || got.Tabs.Active == "" {
		t.Fatalf("expected repair to persist, got %#v", got)
	}
}

func TestInitialize_RepairsMissingActiveTab(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	m := NewManager(homeDir)
	if err := m.saveWorkspace(&Workspace{
		ID:   "ws-invalid-active",
		Name: "Invalid active",
		Tabs: TabsState{
			Active: "tab-missing",
			Items: []Tab{{
				ID:    "tab-existing",
				Type:  TabTypeChat,
				Title: "Chat",
			}},
		},
	}, workDir); err != nil {
		t.Fatalf("saveWorkspace: %v", err)
	}

	if err := m.Initialize(workDir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if got := m.Active().Tabs.Active; got != "tab-existing" {
		t.Fatalf("expected existing tab to become active, got %q", got)
	}
}

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

func TestUpdateTab_MergesState(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if err := m.AddTab(Tab{
		ID:    "tab-1",
		Type:  TabTypeEditor,
		Title: "old",
		State: map[string]any{"draftId": "abc"},
	}); err != nil {
		t.Fatalf("AddTab: %v", err)
	}

	// UpdateTab com state parcial deve fazer merge, não substituir
	if err := m.UpdateTab("tab-1", map[string]any{
		"title": "new",
		"state": map[string]any{"filePath": "/tmp/file.go"},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	tab := m.active.FindTab("tab-1")
	if tab == nil {
		t.Fatal("tab not found after update")
	}
	if tab.Title != "new" {
		t.Errorf("title: got %q, want %q", tab.Title, "new")
	}
	if tab.State["draftId"] != "abc" {
		t.Errorf("draftId lost after merge: got %v", tab.State["draftId"])
	}
	if tab.State["filePath"] != "/tmp/file.go" {
		t.Errorf("filePath not merged: got %v", tab.State["filePath"])
	}
}

func TestUpdateTab_NilRemovesStateKey(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if err := m.AddTab(Tab{
		ID:    "tab-nil",
		Type:  TabTypeEditor,
		Title: "test",
		State: map[string]any{"filePath": "/tmp/file.go", "draftId": "abc"},
	}); err != nil {
		t.Fatalf("AddTab: %v", err)
	}

	// Setting a state key to nil should remove it
	if err := m.UpdateTab("tab-nil", map[string]any{
		"state": map[string]any{"draftId": nil},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	tab := m.active.FindTab("tab-nil")
	if tab == nil {
		t.Fatal("tab not found after update")
	}
	if _, exists := tab.State["draftId"]; exists {
		t.Error("draftId should have been removed by nil, but still exists")
	}
	if tab.State["filePath"] != "/tmp/file.go" {
		t.Errorf("filePath should be preserved: got %v", tab.State["filePath"])
	}
}

func TestUpdateTab_MergesStatePreservesOpenEditorPaths(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Simula aba pristine sem filePath
	if err := m.AddTab(Tab{
		ID:    "tab-pristine",
		Type:  TabTypeEditor,
		Title: "untitled",
		State: map[string]any{},
	}); err != nil {
		t.Fatalf("AddTab: %v", err)
	}

	paths := m.OpenEditorFilePaths()
	if len(paths) != 0 {
		t.Fatalf("pristine tab should have no filePath, got %v", paths)
	}

	absPath := "/home/user/doc.txt"
	if runtime.GOOS == "windows" {
		absPath = `C:\Users\user\doc.txt`
	}

	// Atualiza como o frontend faria no branch pristine
	if err := m.UpdateTab("tab-pristine", map[string]any{
		"title": "doc.txt",
		"state": map[string]any{"filePath": absPath},
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	paths = m.OpenEditorFilePaths()
	if len(paths) != 1 {
		t.Fatalf("expected 1 path after update, got %d: %v", len(paths), paths)
	}
	if paths[0] != filepath.Clean(absPath) {
		t.Errorf("expected %s, got %s", filepath.Clean(absPath), paths[0])
	}
}

func TestUpdateTab_ConversationIDFloat64Zero(t *testing.T) {
	m := NewManager(t.TempDir())
	if err := m.Initialize(t.TempDir()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if err := m.AddTab(Tab{
		ID:    "tab-conv",
		Type:  TabTypeChat,
		Title: "chat",
	}); err != nil {
		t.Fatalf("AddTab: %v", err)
	}

	// float64(0) deve virar "" (sem conversa), não "0"
	if err := m.UpdateTab("tab-conv", map[string]any{
		"conversation_id": float64(0),
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	tab := m.active.FindTab("tab-conv")
	if tab.ConversationID != "" {
		t.Errorf("conversation_id float64(0) deveria virar empty, got %q", tab.ConversationID)
	}

	// float64 positivo (legacy) deve virar "" — numeric IDs are invalid post-UUIDv7 migration
	if err := m.UpdateTab("tab-conv", map[string]any{
		"conversation_id": float64(42),
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	tab = m.active.FindTab("tab-conv")
	if tab.ConversationID != "" {
		t.Errorf("conversation_id float64(42) deveria virar empty (legacy), got %q", tab.ConversationID)
	}

	// non-UUID string should be discarded
	if err := m.UpdateTab("tab-conv", map[string]any{
		"conversation_id": "42",
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	tab = m.active.FindTab("tab-conv")
	if tab.ConversationID != "" {
		t.Errorf("conversation_id string \"42\" deveria virar empty (non-UUID), got %q", tab.ConversationID)
	}

	// string UUID deve funcionar normalmente
	if err := m.UpdateTab("tab-conv", map[string]any{
		"conversation_id": "01970a9e-1234-7000-8000-abcdef123456",
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	tab = m.active.FindTab("tab-conv")
	if tab.ConversationID != "01970a9e-1234-7000-8000-abcdef123456" {
		t.Errorf("conversation_id string UUID incorreto, got %q", tab.ConversationID)
	}

	// UUIDv4 must be rejected — only UUIDv7 is accepted post-migration
	if err := m.UpdateTab("tab-conv", map[string]any{
		"conversation_id": "550e8400-e29b-41d4-a716-446655440000",
	}); err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}

	tab = m.active.FindTab("tab-conv")
	if tab.ConversationID != "" {
		t.Errorf("conversation_id UUIDv4 deveria virar empty, got %q", tab.ConversationID)
	}
}

func TestIsValidUUIDv7_VariantCheck(t *testing.T) {
	// Valid UUIDv7 with RFC4122 variant
	if !isValidUUIDv7("01970a9e-1234-7000-8000-abcdef123456") {
		t.Error("expected valid UUIDv7 to pass")
	}

	// UUIDv4 — wrong version
	if isValidUUIDv7("550e8400-e29b-41d4-a716-446655440000") {
		t.Error("UUIDv4 should be rejected")
	}

	// Not a UUID at all
	if isValidUUIDv7("42") {
		t.Error("numeric string should be rejected")
	}
	if isValidUUIDv7("") {
		t.Error("empty string should be rejected")
	}

	// Nil UUID (version 0, variant 0)
	if isValidUUIDv7("00000000-0000-0000-0000-000000000000") {
		t.Error("nil UUID should be rejected")
	}
}

func TestTabUnmarshalYAML_NumericConversationID(t *testing.T) {
	// Legacy workspace.yaml had conversation_id as integer.
	// The custom UnmarshalYAML should convert it to string.
	yamlData := `
id: tab-1
type: chat
conversation_id: 42
title: Conversa
position: 0
`
	var tab Tab
	if err := yaml.Unmarshal([]byte(yamlData), &tab); err != nil {
		t.Fatalf("UnmarshalYAML with numeric conversation_id: %v", err)
	}
	if tab.ConversationID != "42" {
		t.Errorf("expected conversation_id '42', got %q", tab.ConversationID)
	}
	if tab.ID != "tab-1" {
		t.Errorf("expected id 'tab-1', got %q", tab.ID)
	}
	if tab.Title != "Conversa" {
		t.Errorf("expected title 'Conversa', got %q", tab.Title)
	}
}

func TestTabUnmarshalYAML_StringConversationID(t *testing.T) {
	// Post-migration: conversation_id is a UUIDv7 string.
	yamlData := `
id: tab-2
type: chat
conversation_id: "01970a9e-1234-7000-8000-abcdef123456"
title: Conversa
position: 1
`
	var tab Tab
	if err := yaml.Unmarshal([]byte(yamlData), &tab); err != nil {
		t.Fatalf("UnmarshalYAML with string conversation_id: %v", err)
	}
	if tab.ConversationID != "01970a9e-1234-7000-8000-abcdef123456" {
		t.Errorf("expected UUIDv7 conversation_id, got %q", tab.ConversationID)
	}
}

func TestTabUnmarshalYAML_NoConversationID(t *testing.T) {
	yamlData := `
id: tab-3
type: editor
title: Editor
position: 0
`
	var tab Tab
	if err := yaml.Unmarshal([]byte(yamlData), &tab); err != nil {
		t.Fatalf("UnmarshalYAML without conversation_id: %v", err)
	}
	if tab.ConversationID != "" {
		t.Errorf("expected empty conversation_id, got %q", tab.ConversationID)
	}
}

func TestTabUnmarshalYAML_NumericContentID(t *testing.T) {
	// Legacy content_id was also numeric.
	yamlData := `
id: tab-4
type: chat
content_id: 99
title: Legacy
position: 0
`
	var tab Tab
	if err := yaml.Unmarshal([]byte(yamlData), &tab); err != nil {
		t.Fatalf("UnmarshalYAML with numeric content_id: %v", err)
	}
	if tab.ContentID != "99" {
		t.Errorf("expected content_id '99', got %q", tab.ContentID)
	}
}
