package app

import (
	"encoding/json"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
)

func TestCommandEditorModesCatalogDefaultsAndHandlers(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("produto de comandos ausente")
	}
	if got := len(p.registry.List()); got != 149 {
		t.Fatalf("catálogo = %d, esperado 149", got)
	}
	for _, item := range []struct {
		id, mode, trigger string
	}{
		{commandEditorModeMarkdownID, "markdown", "keyboard.local:Alt+Digit1"},
		{commandEditorModeRichID, "rich", "keyboard.local:Alt+Digit2"},
		{commandEditorModeViewID, "view", "keyboard.local:Alt+Digit3"},
	} {
		definition, ok := p.registry.Lookup(item.id)
		if !ok || definition.Effect != commandcatalog.Write || definition.HandlerClassification != commandcatalog.HandlerBackend ||
			!definition.HasMutableTarget || len(definition.Context.Facts) != 1 || definition.Context.Facts[0].Mode != commandcatalog.ExactVersion {
			t.Fatalf("registro %s inválido: %+v", item.id, definition)
		}
		if _, ok := editorModeForCommand(item.id); !ok {
			t.Fatalf("modo %s não está fechado no backend", item.id)
		}
	}
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	var keyboardDefaults int
	for _, layer := range projection.BuiltinLayers {
		if layer.ID != commandKeyboardLayerID {
			continue
		}
		keyboardDefaults = len(layer.Defaults)
		for _, item := range []struct{ id, trigger string }{
			{commandEditorModeMarkdownID, "keyboard.local:Alt+Digit1"},
			{commandEditorModeRichID, "keyboard.local:Alt+Digit2"},
			{commandEditorModeViewID, "keyboard.local:Alt+Digit3"},
		} {
			found := false
			for _, def := range layer.Defaults {
				if def.Candidate.CommandID == item.id {
					found = def.Candidate.Trigger == item.trigger && def.Candidate.Scope == commandbindings.Application
					break
				}
			}
			if !found {
				t.Fatalf("default global ausente ou incorreto para %s", item.id)
			}
		}
	}
	if keyboardDefaults != 67 {
		t.Fatalf("defaults de teclado = %d, esperado 62", keyboardDefaults)
	}
	payload, err := json.Marshal(workspaceEditorModeChangedEvent{Workspace: p.workspaceMgr.Active(), TabID: "tab-editor", Mode: "markdown"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	if _, ok := fields["workspace"]; !ok {
		t.Fatal("payload de editor_mode_changed sem workspace")
	}
	var tabID, mode string
	if err := json.Unmarshal(fields["tabId"], &tabID); err != nil || tabID != "tab-editor" {
		t.Fatalf("tabId do evento = %q, err=%v", tabID, err)
	}
	if err := json.Unmarshal(fields["mode"], &mode); err != nil || mode != "markdown" {
		t.Fatalf("mode do evento = %q, err=%v", mode, err)
	}
}
