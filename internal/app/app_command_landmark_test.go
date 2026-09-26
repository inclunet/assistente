package app

import (
	"testing"

	"assistente/internal/commandcatalog"
)

func TestCommandLandmarkLocalPresentationAndDefaults(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.registry.List()) != 150 || len(view.LocalPaletteCommands) != 61 || commandProductRegistryVersion != "product-v41-agent-commands" {
		t.Fatal("catalog counts/version")
	}
	for _, tc := range []struct {
		id    string
		names [3]string
	}{
		{"navigation.landmark.next", [3]string{"Próxima região", "Next region", "Siguiente región"}},
		{"navigation.landmark.previous", [3]string{"Região anterior", "Previous region", "Región anterior"}},
		{"navigation.landmark.default", [3]string{"Região padrão", "Default region", "Región predeterminada"}},
	} {
		d, ok := p.registry.Lookup(tc.id)
		if !ok || !isLocalUICommand(tc.id) || !localKeyboardCommandAllowed(tc.id) || commandDeckLedgerCommand(d) || d.Effect != commandcatalog.Read || d.Decision != commandcatalog.NoDecision || d.HandlerClassification != commandcatalog.HandlerUI || d.HasMutableTarget || d.Persistence.Audit != commandcatalog.PersistenceNever {
			t.Fatalf("classification: %+v", d)
		}
		for i, locale := range []string{"pt-BR", "en", "es"} {
			if d.Presentation.Locales[locale].Name != tc.names[i] {
				t.Fatalf("label %s/%s", tc.id, locale)
			}
		}
		for _, source := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck} {
			if !d.AllowsSource(source) {
				t.Fatalf("source %s/%s", tc.id, source)
			}
		}
		if !containsString(view.LocalPaletteCommands, tc.id) {
			t.Fatalf("palette missing %s", tc.id)
		}
		if _, err := a.BeginUICommand(tc.id); err == nil {
			t.Fatal("local presentation reserved")
		}
		if commandInvocationCount(t, tc.id) != 0 || commandLedgerCount(t, tc.id) != 0 {
			t.Fatal("local presentation persisted")
		}
	}
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, layer := range projection.BuiltinLayers {
		if layer.ID != commandKeyboardLayerID {
			continue
		}
		if len(layer.Defaults) != 67 {
			t.Fatalf("defaults=%d", len(layer.Defaults))
		}
		for _, entry := range layer.Defaults {
			if entry.Candidate.CommandID == "navigation.landmark.default" || entry.Candidate.Trigger == "keyboard.local:Escape" {
				t.Fatal("Escape/default must remain native")
			}
		}
	}
	for _, tc := range []struct {
		key LocalCommandShortcut
		id  string
	}{
		{LocalCommandShortcut{Version: 1, Code: "F6", Modifiers: []string{}}, "navigation.landmark.next"},
		{LocalCommandShortcut{Version: 1, Code: "F6", Modifiers: []string{"Shift"}}, "navigation.landmark.previous"},
	} {
		binding := commandKeyboardBindingFor(t, view, tc.key)
		if binding.CommandID != tc.id || binding.Handler != "local_ui" {
			t.Fatalf("binding: %+v", binding)
		}
		if r, err := a.BeginLocalCommandUIKey(view.Generation, tc.key, true); err == nil || r != nil {
			t.Fatal("repeat reached audited path")
		}
		if r, err := a.DispatchLocalCommandKey(view.Generation, tc.key, "down", true); err == nil || r != nil {
			t.Fatal("repeat reached backend")
		}
		if commandInvocationCount(t, tc.id) != 0 || commandLedgerCount(t, tc.id) != 0 {
			t.Fatal("repeat persisted")
		}
	}
	for _, key := range []LocalCommandShortcut{{Version: 1, Code: "F7", Modifiers: []string{}}, {Version: 1, Code: "F7", Modifiers: []string{"Shift"}}, {Version: 1, Code: "Escape", Modifiers: []string{}}} {
		if localCommandShortcutAllowed(key) {
			t.Fatalf("broadened keys: %+v", key)
		}
	}
}
