package workspace

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPublishedWorkspacesUpgradeDirectly(t *testing.T) {
	for _, version := range []string{"0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(version, func(t *testing.T) {
			fixtureRoot := filepath.Join("testdata", "published", version)
			workspaceFixture, err := os.ReadFile(filepath.Join(fixtureRoot, workspaceFile))
			if err != nil {
				t.Fatal(err)
			}
			remapFixture, err := os.ReadFile(filepath.Join(fixtureRoot, "uuid-migration-remap.json"))
			if err != nil {
				t.Fatal(err)
			}

			root := t.TempDir()
			workDir := filepath.Join(root, "projeto")
			assistentePath := filepath.Join(workDir, assistenteDir)
			if err := os.MkdirAll(assistentePath, 0755); err != nil {
				t.Fatal(err)
			}
			workspacePath := filepath.Join(assistentePath, workspaceFile)
			if err := os.WriteFile(workspacePath, workspaceFixture, 0644); err != nil {
				t.Fatal(err)
			}
			remapPath := filepath.Join(assistentePath, "uuid-migration-remap.json")
			if err := os.WriteFile(remapPath, remapFixture, 0600); err != nil {
				t.Fatal(err)
			}

			withChdirAndConfigReset(t, workDir)
			manager := NewManager(filepath.Join(root, "home", assistenteDir))
			if err := manager.Initialize(workDir); err != nil {
				t.Fatalf("startup do workspace %s: %v", version, err)
			}
			assertPublishedWorkspaceMigrated(t, manager.Active(), version)
			if _, err := os.Stat(remapPath); !os.IsNotExist(err) {
				t.Fatalf("remap %s não foi consumido após persistência integral: %v", version, err)
			}

			afterFirst, err := os.ReadFile(workspacePath)
			if err != nil {
				t.Fatal(err)
			}
			reloaded := NewManager(filepath.Join(root, "home", assistenteDir))
			if err := reloaded.Initialize(workDir); err != nil {
				t.Fatalf("segunda inicialização %s: %v", version, err)
			}
			assertPublishedWorkspaceMigrated(t, reloaded.Active(), version)
			afterSecond, err := os.ReadFile(workspacePath)
			if err != nil || !bytes.Equal(afterFirst, afterSecond) {
				t.Fatalf("segunda passagem alterou workspace %s: %v", version, err)
			}
		})
	}
}

func assertPublishedWorkspaceMigrated(t *testing.T, workspace *Workspace, version string) {
	t.Helper()
	if workspace == nil ||
		workspace.ID != "ws-fixture-"+version ||
		workspace.Profile != "programacao" ||
		workspace.Tabs.Active != "tab-editor" ||
		len(workspace.Tabs.Items) != 4 {
		t.Fatalf("workspace %s não preservado: %+v", version, workspace)
	}
	chat := workspace.FindTab("tab-chat")
	if chat == nil || chat.ContentID != "" || chat.ConversationID != "01970a9e-1234-7000-8000-aaaaaaaaaaaa" {
		t.Fatalf("vínculo de conversa %s não remapeado: %+v", version, chat)
	}
	editor := workspace.FindTab("tab-editor")
	if editor == nil ||
		editor.ContentID != "" ||
		editor.State["filePath"] != "documento-sintetico.md" ||
		editor.ProfileOverride["slug"] != "editor-texto" ||
		editor.ProfileOverride["model"] != "modelo-sintetico" {
		t.Fatalf("aba de editor %s não preservada: %+v", version, editor)
	}
	tasklist := workspace.FindTab("tab-tasklist")
	if tasklist == nil || tasklist.State["tasklistId"] != "01970a9e-5678-7000-8000-bbbbbbbbbbbb" {
		t.Fatalf("vínculo de tasklist %s não remapeado: %+v", version, tasklist)
	}
	terminal := workspace.FindTab("tab-terminal")
	if terminal == nil || terminal.State["sessionId"] != "terminal-sintetico" {
		t.Fatalf("estado de terminal %s não preservado: %+v", version, terminal)
	}
}
