package workspace

import (
	"encoding/json"
	"errors"
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

// writeIndex writes an index.yaml inside <homeDir>/workspaces/.
func writeIndex(t *testing.T, homeDir string, idx Index) {
	t.Helper()
	dir := filepath.Join(homeDir, workspacesDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	b, err := yaml.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, indexFile), b, 0644); err != nil {
		t.Fatalf("write index: %v", err)
	}
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

func TestLoadWorkspaceFile_PreservesRemapAfterSave(t *testing.T) {
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

	// Remap file must NOT be deleted by loadWorkspaceFile — Initialize handles cleanup.
	remapPath := filepath.Join(remapDir, "uuid-migration-remap.json")
	if _, err := os.Stat(remapPath); os.IsNotExist(err) {
		t.Errorf("remap file should be preserved after loadWorkspaceFile, but it was deleted")
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

func TestLoadIDRemap_HighestPriorityWins(t *testing.T) {
	// Create two directories simulating home (lower priority) and workdir (higher priority).
	// Each has a remap with conflicting conversation mappings.
	// loadIDRemap must choose the workdir (higher-priority) remap.
	tmpHome := t.TempDir()
	tmpWork := t.TempDir()

	homeRemapDir := filepath.Join(tmpHome, assistenteDir)
	writeRemapFile(t, homeRemapDir, database.IDRemapData{
		Conversations: map[string]string{
			"42": "01970a9e-0001-7000-8000-111111111111", // wrong — lower priority
		},
	})

	workRemapDir := filepath.Join(tmpWork, assistenteDir)
	writeRemapFile(t, workRemapDir, database.IDRemapData{
		Conversations: map[string]string{
			"42": "01970a9e-0002-7000-8000-222222222222", // correct — higher priority
		},
	})

	// Set CWD to tmpWork so configdir resolves workdir=tmpWork, home=tmpHome.
	// We override the HOME env so configdir picks tmpHome as cachedHomeDir.
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	withChdirAndConfigReset(t, tmpWork)

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
	wsPath := writeWorkspaceYAML(t, tmpWork, ws)

	m := NewManager(tmpHome)
	loaded, err := m.loadWorkspaceFile(wsPath)
	if err != nil {
		t.Fatalf("loadWorkspaceFile: %v", err)
	}

	tab := loaded.FindTab("tab-1")
	if tab == nil {
		t.Fatal("tab-1 not found")
	}
	// Must use the higher-priority (workdir) remap value.
	want := "01970a9e-0002-7000-8000-222222222222"
	if tab.ConversationID != want {
		t.Errorf("conversation_id: got %q, want %q (should use workdir remap, not home)", tab.ConversationID, want)
	}
}

func TestInitialize_MigratesAllWorkspacesBeforeDeletingRemap(t *testing.T) {
	// Two workspaces with legacy IDs and one remap.
	// After Initialize (which loads only one), both must be migrated
	// and the remap file must be deleted.
	homeDir := t.TempDir()

	remapDir := filepath.Join(homeDir, assistenteDir)
	writeRemapFile(t, remapDir, database.IDRemapData{
		Conversations: map[string]string{
			"10": "01970a9e-aaaa-7000-8000-aaaaaaaaaaaa",
			"20": "01970a9e-bbbb-7000-8000-bbbbbbbbbbbb",
		},
	})

	// Create workspace 1 (will be the active one via workDir)
	ws1Dir := filepath.Join(homeDir, workspacesDir, "ws1")
	ws1 := Workspace{
		ID:   "ws-1",
		Name: "WS 1",
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-1",
				Type:      TabTypeChat,
				Title:     "Chat 1",
				ContentID: "10",
			}},
		},
	}
	writeWorkspaceYAML(t, ws1Dir, ws1)

	// Create workspace 2 (in the index, will be migrated by cleanup)
	ws2Dir := filepath.Join(homeDir, workspacesDir, "ws2")
	ws2 := Workspace{
		ID:   "ws-2",
		Name: "WS 2",
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-2",
				Type:      TabTypeChat,
				Title:     "Chat 2",
				ContentID: "20",
			}},
		},
	}
	writeWorkspaceYAML(t, ws2Dir, ws2)

	// Write index with both workspaces
	writeIndex(t, homeDir, Index{
		LastOpened: "ws-1",
		Workspaces: []IndexEntry{
			{ID: "ws-1", Name: "WS 1", Path: ws1Dir},
			{ID: "ws-2", Name: "WS 2", Path: ws2Dir},
		},
	})

	withChdirAndConfigReset(t, homeDir)

	m := NewManager(homeDir)
	if err := m.Initialize(ws1Dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// 1. Active workspace (ws-1) should be migrated
	if m.Active().FindTab("tab-1").ConversationID != "01970a9e-aaaa-7000-8000-aaaaaaaaaaaa" {
		t.Errorf("ws-1 tab-1: got %q, want remapped UUID", m.Active().FindTab("tab-1").ConversationID)
	}

	// 2. Non-active workspace (ws-2) should also be migrated (read YAML back)
	ws2Path := filepath.Join(ws2Dir, assistenteDir, workspaceFile)
	ws2Loaded, err := m.loadWorkspaceFile(ws2Path)
	if err != nil {
		t.Fatalf("reload ws-2: %v", err)
	}
	tab2 := ws2Loaded.FindTab("tab-2")
	if tab2 == nil {
		t.Fatal("tab-2 not found in ws-2")
	}
	if tab2.ConversationID != "01970a9e-bbbb-7000-8000-bbbbbbbbbbbb" {
		t.Errorf("ws-2 tab-2: got %q, want remapped UUID", tab2.ConversationID)
	}

	// 3. Remap file should be deleted after Initialize
	remapPath := filepath.Join(remapDir, "uuid-migration-remap.json")
	if _, err := os.Stat(remapPath); !os.IsNotExist(err) {
		t.Errorf("remap file should have been deleted after Initialize, stat err: %v", err)
	}
}

func TestLoadWorkspaceFile_ReturnsErrMigrationSaveFailedOnSaveError(t *testing.T) {
	tmp := t.TempDir()

	remapDir := filepath.Join(tmp, assistenteDir)
	writeRemapFile(t, remapDir, database.IDRemapData{
		Conversations: map[string]string{
			"50": "01970a9e-cccc-7000-8000-eeeeeeeeeeee",
		},
	})

	ws := Workspace{
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-1",
				Type:      TabTypeChat,
				Title:     "Chat",
				ContentID: "50",
			}},
		},
	}
	wsPath := writeWorkspaceYAML(t, tmp, ws)
	withChdirAndConfigReset(t, tmp)

	// Make the workspace.yaml file itself read-only so saveWorkspace fails.
	// os.Chmod on files works on Windows (sets the read-only attribute).
	if err := os.Chmod(wsPath, 0444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(wsPath, 0644) })

	m := NewManager(tmp)
	loaded, err := m.loadWorkspaceFile(wsPath)

	// Should return the workspace AND an ErrMigrationSaveFailed error
	if loaded == nil {
		t.Fatal("workspace should be returned even on save failure")
	}
	if !errors.Is(err, ErrMigrationSaveFailed) {
		t.Errorf("expected ErrMigrationSaveFailed, got: %v", err)
	}

	// Workspace should be migrated in memory
	tab := loaded.FindTab("tab-1")
	if tab == nil {
		t.Fatal("tab-1 not found")
	}
	if tab.ConversationID != "01970a9e-cccc-7000-8000-eeeeeeeeeeee" {
		t.Errorf("conversation_id: got %q, want %q", tab.ConversationID, "01970a9e-cccc-7000-8000-eeeeeeeeeeee")
	}
}

func TestInitialize_PreservesRemapOnPartialSaveFailure(t *testing.T) {
	// Two workspaces with legacy IDs. ws-2's directory is made read-only
	// so saving migration fails. The remap file must NOT be deleted.
	homeDir := t.TempDir()

	remapDir := filepath.Join(homeDir, assistenteDir)
	writeRemapFile(t, remapDir, database.IDRemapData{
		Conversations: map[string]string{
			"10": "01970a9e-aaaa-7000-8000-aaaaaaaaaaaa",
			"20": "01970a9e-bbbb-7000-8000-bbbbbbbbbbbb",
		},
	})

	// ws-1: will be the active workspace, save will succeed
	ws1Dir := filepath.Join(homeDir, workspacesDir, "ws1")
	ws1 := Workspace{
		ID:   "ws-1",
		Name: "WS 1",
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-1",
				Type:      TabTypeChat,
				Title:     "Chat 1",
				ContentID: "10",
			}},
		},
	}
	writeWorkspaceYAML(t, ws1Dir, ws1)

	// ws-2: save will fail because .assistente dir is read-only
	ws2Dir := filepath.Join(homeDir, workspacesDir, "ws2")
	ws2 := Workspace{
		ID:   "ws-2",
		Name: "WS 2",
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-2",
				Type:      TabTypeChat,
				Title:     "Chat 2",
				ContentID: "20",
			}},
		},
	}
	writeWorkspaceYAML(t, ws2Dir, ws2)

	// Make ws-2's workspace.yaml file read-only so saveWorkspace fails.
	ws2WsPath := filepath.Join(ws2Dir, assistenteDir, workspaceFile)
	if err := os.Chmod(ws2WsPath, 0444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(ws2WsPath, 0644) })

	writeIndex(t, homeDir, Index{
		LastOpened: "ws-1",
		Workspaces: []IndexEntry{
			{ID: "ws-1", Name: "WS 1", Path: ws1Dir},
			{ID: "ws-2", Name: "WS 2", Path: ws2Dir},
		},
	})

	withChdirAndConfigReset(t, homeDir)

	m := NewManager(homeDir)
	if err := m.Initialize(ws1Dir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// ws-1 should be migrated successfully
	if m.Active().FindTab("tab-1").ConversationID != "01970a9e-aaaa-7000-8000-aaaaaaaaaaaa" {
		t.Errorf("ws-1 tab-1: got %q, want remapped UUID", m.Active().FindTab("tab-1").ConversationID)
	}

	// Remap file must be PRESERVED because ws-2's save failed
	remapPath := filepath.Join(remapDir, "uuid-migration-remap.json")
	if _, err := os.Stat(remapPath); os.IsNotExist(err) {
		t.Errorf("remap file should be preserved when a workspace migration save failed, but it was deleted")
	}
}

// TestInitialize_PreservesRemapOnActiveWorkspaceSaveFailure verifies that when
// the ACTIVE workspace fails to persist its migration, the remap file is NOT
// deleted — even if all other workspaces succeed.
func TestInitialize_PreservesRemapOnActiveWorkspaceSaveFailure(t *testing.T) {
	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	activeDir := filepath.Join(tmpDir, "workspaces", "active-ws")

	// Create remap in homeDir/.assistente/ (where configdir resolves it)
	remapDir := filepath.Join(homeDir, assistenteDir)
	writeRemapFile(t, remapDir, database.IDRemapData{
		Conversations: map[string]string{
			"99": "01970a9e-cccc-7000-8000-cccccccccccc",
		},
	})

	// Create workspace with a legacy content_id that needs remapping
	activeWs := Workspace{
		ID:   "active-ws",
		Name: "Active WS",
		Tabs: TabsState{
			Items: []Tab{{
				ID:        "tab-1",
				Type:      TabTypeChat,
				Title:     "Chat",
				ContentID: "99",
			}},
		},
	}
	activeWsPath := writeWorkspaceYAML(t, activeDir, activeWs)

	// Make workspace.yaml read-only to force save failure
	if err := os.Chmod(activeWsPath, 0444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(activeWsPath, 0644) })

	// Write index (no other workspaces)
	writeIndex(t, homeDir, Index{
		LastOpened: "active-ws",
		Workspaces: []IndexEntry{
			{ID: "active-ws", Name: "Active WS", Path: activeDir},
		},
	})

	withChdirAndConfigReset(t, homeDir)

	m := NewManager(homeDir)
	if err := m.Initialize(activeDir); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Active workspace should still be loaded (in-memory migration OK)
	tab := m.Active().FindTab("tab-1")
	if tab == nil {
		t.Fatal("tab-1 not found in active workspace")
	}
	if tab.ConversationID != "01970a9e-cccc-7000-8000-cccccccccccc" {
		t.Errorf("expected remapped UUID, got %q", tab.ConversationID)
	}

	// Remap file must be PRESERVED because the active workspace's save failed
	remapPath := filepath.Join(remapDir, "uuid-migration-remap.json")
	if _, err := os.Stat(remapPath); os.IsNotExist(err) {
		t.Errorf("remap file should be preserved when active workspace migration save failed, but it was deleted")
	}
}
