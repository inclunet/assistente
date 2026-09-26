package app

import (
	"testing"

	"assistente/internal/commandcatalog"
)

func TestCommandChatInspectionLocalPresentation(t *testing.T) {
	for _, tc := range []struct {
		id    string
		names [3]string
	}{
		{commandChatPinnedOpenID, [3]string{"Mensagens fixadas", "Pinned messages", "Mensajes fijados"}},
		{commandChatTokensOpenID, [3]string{"Estatísticas de tokens", "Token statistics", "Estadísticas de tokens"}},
		{commandMessageEditID, [3]string{"Abrir edição da mensagem", "Open message editor", "Abrir edición del mensaje"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			a, decisions := settingsSecurityFixture(t)
			p := a.commandProduct.Load()
			if len(p.registry.List()) != 150 || commandProductRegistryVersion != "product-v41-agent-commands" {
				t.Fatal("catalog/version")
			}
			d, ok := p.registry.Lookup(tc.id)
			if !ok || !isLocalUICommand(tc.id) || !localKeyboardCommandAllowed(tc.id) || commandDeckLedgerCommand(d) || d.Effect != commandcatalog.Read || d.Decision != commandcatalog.NoDecision || d.HandlerClassification != commandcatalog.HandlerUI || d.Persistence.Audit != commandcatalog.PersistenceNever {
				t.Fatalf("unsafe classification: %+v", d)
			}
			for i, locale := range []string{"pt-BR", "en", "es"} {
				if d.Presentation.Locales[locale].Name != tc.names[i] {
					t.Fatalf("label %s: %+v", locale, d.Presentation)
				}
			}
			projection, err := commandProductProjection(p.registry, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, layer := range projection.BuiltinLayers {
				if layer.ID == commandKeyboardLayerID && len(layer.Defaults) != 67 {
					t.Fatal("default count changed")
				}
				for _, item := range layer.Defaults {
					if layer.ID == commandKeyboardLayerID && item.Candidate.CommandID == tc.id {
						t.Fatal("unexpected default")
					}
				}
			}
			view, err := a.GetLocalCommandKeyboardMap()
			if err != nil || len(view.LocalPaletteCommands) != 61 || !containsString(view.LocalPaletteCommands, tc.id) {
				t.Fatalf("palette: %+v %v", view, err)
			}
			layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
			settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
				return a.SaveCommandBinding(CommandBindingEdit{LayerID: layer, CommandID: tc.id, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control","Shift"]}`, Enabled: true})
			})
			if _, err := a.SetCommandLayerActive(layer, true); err != nil {
				t.Fatal(err)
			}
			view, err = a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			key := LocalCommandShortcut{Version: 1, Code: "KeyK", Modifiers: []string{"Control", "Shift"}}
			binding := commandKeyboardBindingFor(t, view, key)
			if binding.CommandID != tc.id || binding.Handler != "local_ui" {
				t.Fatalf("custom key: %+v", binding)
			}
			if _, err := a.BeginUICommand(tc.id); err == nil {
				t.Fatal("local command admitted to broker")
			}
			if r, err := a.BeginLocalCommandUIKey(view.Generation, key, false); err == nil || r != nil {
				t.Fatal("local key admitted to broker")
			}
			if commandInvocationCount(t, tc.id) != 0 || commandLedgerCount(t, tc.id) != 0 {
				t.Fatal("local presentation persisted")
			}
		})
	}
}
