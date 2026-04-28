package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"
	"gopkg.in/yaml.v3"
)

// writeRemapFile creates a uuid-migration-remap.json file in dir.
func writeRemapFile(t *testing.T, dir string, data database.IDRemapData) {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal remap: %v", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "uuid-migration-remap.json"), b, 0600); err != nil {
		t.Fatalf("write remap: %v", err)
	}
}

// writeWorkspaceYAML writes a workspace.yaml inside <base>/.assistente/.
func writeWorkspaceYAML(t *testing.T, base string, ws Workspace) string {
	t.Helper()
	dir := filepath.Join(base, assistenteDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	b, err := yaml.Marshal(ws)
	if err != nil {
		t.Fatalf("marshal workspace: %v", err)
	}
	p := filepath.Join(dir, workspaceFile)
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatalf("write workspace: %v", err)
	}
	return p
}

// withChdirAndConfigReset changes CWD to dir, resets configdir cache,
// and restores both on cleanup.
func withChdirAndConfigReset(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	configdir.ResetForTests()
	t.Cleanup(func() {
		_ = os.Chdir(orig)
		configdir.ResetForTests()
	})
}

func TestLoadWorkspaceFile_RemapsLegacyChatContentID(t *testing.T) {
	tmp := t.TempDir()

	// Create remap file in <tmp>/.assistente/ (will be found via workdir)
	remapDir := filepath.Join(tmp, assistenteDir)
	writeRemapFile(t, remapDir, database.IDRemapData{
		Conversations: map[string]string{
			"42": "01970a9e-1234-7000-8000-aaaaaaaaaaaa",
		},
	})

	// Create workspace with a legacy chat tab referencing content_id="42"
	ws := Workspace{
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-1",
				Type:      TabTypeChat,
				Title:     "Chat",
				ContentID: "42",
			}},
		},
	}
	wsPath := writeWorkspaceYAML(t, tmp, ws)

	// Point configdir to our temp dir
	withChdirAndConfigReset(t, tmp)

	m := NewManager(tmp)
	loaded, err := m.loadWorkspaceFile(wsPath)
	if err != nil {
		t.Fatalf("loadWorkspaceFile: %v", err)
	}

	tab := loaded.FindTab("tab-1")
	if tab == nil {
		t.Fatal("tab-1 not found")
	}
	if tab.ConversationID != "01970a9e-1234-7000-8000-aaaaaaaaaaaa" {
		t.Errorf("conversation_id: got %q, want %q", tab.ConversationID, "01970a9e-1234-7000-8000-aaaaaaaaaaaa")
	}
	if tab.ContentID != "" {
		t.Errorf("content_id should be cleared, got %q", tab.ContentID)
	}
}

func TestLoadWorkspaceFile_ClearsInvalidTasklistIDWithoutRemap(t *testing.T) {
	tmp := t.TempDir()

	// NO remap file — simulate case where remap was already consumed or never existed.
	ws := Workspace{
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-tl",
				Type:      TabTypeTasklist,
				Title:     "Tasks",
				ContentID: "99", // legacy numeric
			}},
		},
	}
	wsPath := writeWorkspaceYAML(t, tmp, ws)
	withChdirAndConfigReset(t, tmp)

	m := NewManager(tmp)
	loaded, err := m.loadWorkspaceFile(wsPath)
	if err != nil {
		t.Fatalf("loadWorkspaceFile: %v", err)
	}

	tab := loaded.FindTab("tab-tl")
	if tab == nil {
		t.Fatal("tab-tl not found")
	}
	if tab.State == nil {
		t.Fatal("tab state is nil")
	}
	tlID, _ := tab.State["tasklistId"].(string)
	if tlID != "" {
		t.Errorf("tasklistId should be empty (invalid without remap), got %q", tlID)
	}
}

func TestLoadWorkspaceFile_RemapsTasklistStateID(t *testing.T) {
	tmp := t.TempDir()

	remapDir := filepath.Join(tmp, assistenteDir)
	writeRemapFile(t, remapDir, database.IDRemapData{
		TaskLists: map[string]string{
			"7": "01970a9e-5678-7000-8000-bbbbbbbbbbbb",
		},
	})

	ws := Workspace{
		Tabs: TabsState{
			Items: []Tab{{
				ID:    "tab-tl",
				Type:  TabTypeTasklist,
				Title: "Tasks",
				State: map[string]any{"tasklistId": "7"},
			}},
		},
	}
	wsPath := writeWorkspaceYAML(t, tmp, ws)
	withChdirAndConfigReset(t, tmp)

	m := NewManager(tmp)
	loaded, err := m.loadWorkspaceFile(wsPath)
	if err != nil {
		t.Fatalf("loadWorkspaceFile: %v", err)
	}

	tab := loaded.FindTab("tab-tl")
	if tab == nil {
		t.Fatal("tab-tl not found")
	}
	tlID, _ := tab.State["tasklistId"].(string)
	if tlID != "01970a9e-5678-7000-8000-bbbbbbbbbbbb" {
		t.Errorf("tasklistId: got %q, want %q", tlID, "01970a9e-5678-7000-8000-bbbbbbbbbbbb")
	}
}

func TestLoadWorkspaceFile_ClearsInvalidTasklistStateWithoutRemap(t *testing.T) {
	tmp := t.TempDir()

	// No remap file
	ws := Workspace{
		Tabs: TabsState{
			Items: []Tab{{
				ID:    "tab-tl",
				Type:  TabTypeTasklist,
				Title: "Tasks",
				State: map[string]any{"tasklistId": "999"},
			}},
		},
	}
	wsPath := writeWorkspaceYAML(t, tmp, ws)
	withChdirAndConfigReset(t, tmp)

	m := NewManager(tmp)
	loaded, err := m.loadWorkspaceFile(wsPath)
	if err != nil {
		t.Fatalf("loadWorkspaceFile: %v", err)
	}

	tab := loaded.FindTab("tab-tl")
	if tab == nil {
		t.Fatal("tab-tl not found")
	}
	tlID, _ := tab.State["tasklistId"].(string)
	if tlID != "" {
		t.Errorf("tasklistId should be cleared (no remap), got %q", tlID)
	}
}

func TestLoadWorkspaceFile_DeletesRemapOnlyAfterSuccessfulSave(t *testing.T) {
	tmp := t.TempDir()

	remapDir := filepath.Join(tmp, assistenteDir)
	writeRemapFile(t, remapDir, database.IDRemapData{
		Conversations: map[string]string{
			"10": "01970a9e-aaaa-7000-8000-cccccccccccc",
		},
	})

	ws := Workspace{
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-1",
				Type:      TabTypeChat,
				Title:     "Chat",
				ContentID: "10",
			}},
		},
	}
	wsPath := writeWorkspaceYAML(t, tmp, ws)
	withChdirAndConfigReset(t, tmp)

	m := NewManager(tmp)
	_, err := m.loadWorkspaceFile(wsPath)
	if err != nil {
		t.Fatalf("loadWorkspaceFile: %v", err)
	}

	// Remap file should have been deleted (save succeeded for workspace in tmp/.assistente/)
	remapPath := filepath.Join(remapDir, "uuid-migration-remap.json")
	if _, err := os.Stat(remapPath); !os.IsNotExist(err) {
		t.Errorf("remap file should have been deleted after successful save, stat err: %v", err)
	}
}

func TestLoadWorkspaceFile_SanitizesLegacyConversationID(t *testing.T) {
	tmp := t.TempDir()

	remapDir := filepath.Join(tmp, assistenteDir)
	writeRemapFile(t, remapDir, database.IDRemapData{
		Conversations: map[string]string{
			"55": "01970a9e-bbbb-7000-8000-dddddddddddd",
		},
	})

	// Tab already has conversation_id set to a legacy numeric string
	ws := Workspace{
		Tabs: TabsState{
			Items: []Tab{{
				ID:             "tab-1",
				Type:           TabTypeChat,
				Title:          "Chat",
				ConversationID: "55", // legacy numeric, should be remapped
			}},
		},
	}
	wsPath := writeWorkspaceYAML(t, tmp, ws)
	withChdirAndConfigReset(t, tmp)

	m := NewManager(tmp)
	loaded, err := m.loadWorkspaceFile(wsPath)
	if err != nil {
		t.Fatalf("loadWorkspaceFile: %v", err)
	}

	tab := loaded.FindTab("tab-1")
	if tab == nil {
		t.Fatal("tab-1 not found")
	}
	if tab.ConversationID != "01970a9e-bbbb-7000-8000-dddddddddddd" {
		t.Errorf("conversation_id: got %q, want %q", tab.ConversationID, "01970a9e-bbbb-7000-8000-dddddddddddd")
	}
}

func TestLoadWorkspaceFile_ClearsLegacyConversationIDWithoutRemap(t *testing.T) {
	tmp := t.TempDir()

	// No remap file
	ws := Workspace{
		Tabs: TabsState{
			Items: []Tab{{
				ID:             "tab-1",
				Type:           TabTypeChat,
				Title:          "Chat",
				ConversationID: "77", // legacy, no remap available → cleared
			}},
		},
	}
	wsPath := writeWorkspaceYAML(t, tmp, ws)
	withChdirAndConfigReset(t, tmp)

	m := NewManager(tmp)
	loaded, err := m.loadWorkspaceFile(wsPath)
	if err != nil {
		t.Fatalf("loadWorkspaceFile: %v", err)
	}

	tab := loaded.FindTab("tab-1")
	if tab == nil {
		t.Fatal("tab-1 not found")
	}
	if tab.ConversationID != "" {
		t.Errorf("conversation_id should be cleared without remap, got %q", tab.ConversationID)
	}
}
