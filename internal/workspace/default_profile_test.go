package workspace

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultProfileIsRuntimeProjectionAcrossWorkspaces(t *testing.T) {
	root := t.TempDir()
	manager := newTestManager(t, filepath.Join(root, "home"))
	if !manager.SetDefaultProfile("app-default") {
		t.Fatal("padrão inicial não foi aplicado")
	}
	if err := manager.Initialize(filepath.Join(root, "workspace")); err != nil {
		t.Fatal(err)
	}
	original := manager.Active().ID
	other, err := manager.createForTest("Outro", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{other.ID, original} {
		snapshot, err := manager.Switch(id)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.DefaultProfile != "app-default" {
			t.Fatalf("troca perdeu padrão global: %+v", snapshot)
		}
		encoded, err := yaml.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		var persisted map[string]any
		if err := yaml.Unmarshal(encoded, &persisted); err != nil {
			t.Fatal(err)
		}
		if _, ok := persisted["profile"]; ok {
			t.Fatal("persistiu perfil de workspace removido")
		}
		if _, ok := persisted["default_profile"]; ok {
			t.Fatal("persistiu projeção do padrão do app")
		}
		payload, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(payload), `"default_profile":"app-default"`) {
			t.Fatal("snapshot não publicou padrão")
		}
	}
	manager.SetDefaultProfile("new-default")
	if manager.Active().DefaultProfile != "new-default" {
		t.Fatal("snapshot ficou com padrão antigo")
	}
}

func TestLegacyWorkspaceProfileIsIgnored(t *testing.T) {
	var workspace Workspace
	if err := yaml.Unmarshal([]byte("id: legacy\nname: Legacy\nprofile: old-workspace-profile\ndefault_profile: forged\n"), &workspace); err != nil {
		t.Fatal(err)
	}
	if workspace.DefaultProfile != "" {
		t.Fatal("arquivo do workspace escolheu padrão do aplicativo")
	}
}
