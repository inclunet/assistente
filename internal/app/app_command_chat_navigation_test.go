package app

import (
	"assistente/internal/commandcatalog"
	"testing"
)

func TestCommandChatNavigationLocalPresentation(t *testing.T) {
	for _, tc := range []struct {
		id    string
		names [3]string
	}{
		{"chat.focus.input", [3]string{"Focar campo de mensagem", "Focus message input", "Enfocar campo de mensaje"}},
		{"chat.focus.messages", [3]string{"Focar mensagens", "Focus messages", "Enfocar mensajes"}},
		{"chat.message.read.open", [3]string{"Abrir leitura da mensagem", "Open message reading", "Abrir lectura del mensaje"}},
		{"chat.message.menu.open", [3]string{"Abrir menu da mensagem", "Open message menu", "Abrir menú del mensaje"}},
		{"chat.message.reasoning.toggle", [3]string{"Alternar exibição do raciocínio", "Toggle reasoning visibility", "Alternar visibilidad del razonamiento"}},
		{"chat.message.thread.expand", [3]string{"Expandir respostas da mensagem", "Expand message thread", "Expandir respuestas del mensaje"}},
		{"chat.message.thread.collapse", [3]string{"Recolher respostas da mensagem", "Collapse message thread", "Contraer respuestas del mensaje"}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			key := LocalCommandShortcut{Version: 1, Code: "KeyJ", Modifiers: []string{"Control", "Shift"}}
			a, view := localKeyboardRepeatFixture(t, localKeyboardRepeatBinding{tc.id, key})
			p := a.commandProduct.Load()
			if len(p.registry.List()) != 151 || len(view.LocalPaletteCommands) != 62 || commandProductRegistryVersion != "product-v42-command-settings-create" {
				t.Fatal("counts/version")
			}
			d, ok := p.registry.Lookup(tc.id)
			if !ok || !isLocalUICommand(tc.id) || !localKeyboardCommandAllowed(tc.id) || commandDeckLedgerCommand(d) || d.Effect != commandcatalog.Read || d.Decision != commandcatalog.NoDecision || d.HandlerClassification != commandcatalog.HandlerUI || d.HasMutableTarget || d.MutatesEffectiveCapability || d.Persistence.Audit != commandcatalog.PersistenceNever || d.Persistence.Arguments != commandcatalog.PersistenceNever || d.Persistence.Result != commandcatalog.PersistenceNever {
				t.Fatalf("unsafe classification: %+v", d)
			}
			for i, locale := range []string{"pt-BR", "en", "es"} {
				if d.Presentation.Locales[locale].Name != tc.names[i] {
					t.Fatalf("label %s", locale)
				}
			}
			for _, allowed := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck, commandcatalog.UI} {
				if !d.AllowsSource(allowed) {
					t.Fatalf("missing source %s", allowed)
				}
			}
			for _, denied := range []commandcatalog.Source{commandcatalog.KeyboardGlobal, commandcatalog.Chat, commandcatalog.CLI, commandcatalog.Event, commandcatalog.System} {
				if d.AllowsSource(denied) {
					t.Fatalf("expanded source %s", denied)
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
				if len(layer.Defaults) != 68 {
					t.Fatal("defaults changed")
				}
				for _, item := range layer.Defaults {
					if item.Candidate.CommandID == tc.id {
						t.Fatal("unexpected shortcut default")
					}
				}
			}
			if !containsString(view.LocalPaletteCommands, tc.id) {
				t.Fatal("not projected in palette")
			}
			binding := commandKeyboardBindingFor(t, view, key)
			if binding.CommandID != tc.id || binding.Handler != "local_ui" {
				t.Fatalf("binding: %+v", binding)
			}
			if _, err := a.BeginUICommand(tc.id); err == nil {
				t.Fatal("local presentation reserved")
			}
			for _, repeat := range []bool{false, true} {
				if r, err := a.BeginLocalCommandUIKey(view.Generation, key, repeat); err == nil || r != nil {
					t.Fatal("local presentation reached broker")
				}
				if r, err := a.DispatchLocalCommandKey(view.Generation, key, "down", repeat); err == nil || r != nil {
					t.Fatal("local presentation reached backend")
				}
			}
			if _, _, ok := workspaceTabNavigationForCommand(tc.id); ok {
				t.Fatal("tab repeat eligibility expanded")
			}
			if commandInvocationCount(t, tc.id) != 0 || commandLedgerCount(t, tc.id) != 0 {
				t.Fatal("local presentation persisted")
			}
		})
	}
}

func TestWorkspaceTabGoToRegistrationIsSingleParameterizedLocalCommand(t *testing.T) {
	definition, handler := commandWorkspaceTabNavigationRegistration(commandWorkspaceTabGoToID)
	if definition.ID != "workspace.tab.go_to" || handler.Route != "ui/workspace/tab/navigate" ||
		commandExecutionClassForDefinition(definition) != commandExecutionLocalUI || definition.ArgumentsSchema == nil {
		t.Fatalf("registro de destino de aba inesperado: definition=%+v handler=%+v", definition, handler)
	}
	if len(definition.ArgumentsSchema.Properties) != 4 || len(definition.ArgumentsSchema.Required) != 2 ||
		definition.ArgumentsSchema.Properties["workspace_id"].Type != commandcatalog.SchemaString ||
		definition.ArgumentsSchema.Properties["target_mode"].Type != commandcatalog.SchemaString ||
		definition.ArgumentsSchema.Properties["position"].Type != commandcatalog.SchemaInteger ||
		definition.ArgumentsSchema.Properties["position"].Minimum == nil || *definition.ArgumentsSchema.Properties["position"].Minimum != 1 ||
		definition.ArgumentsSchema.Properties["position"].Maximum != nil ||
		definition.ArgumentsSchema.Properties["tab_id"].Type != commandcatalog.SchemaString {
		t.Fatalf("schema parametrizado incompleto ou limitado: %+v", definition.ArgumentsSchema)
	}
	for _, source := range []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck} {
		if !definition.AllowsSource(source) {
			t.Fatalf("origem local ausente: %s", source)
		}
	}
	if definition.AllowsSource(commandcatalog.UI) || definition.Persistence.Arguments != commandcatalog.PersistenceRedacted ||
		definition.Persistence.Result != commandcatalog.PersistenceNever || definition.Persistence.Audit != commandcatalog.PersistenceNever {
		t.Fatalf("origem externa ou persistência inesperada: %+v", definition)
	}
}
