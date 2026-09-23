package app

import (
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
)

func TestCommandKeyboardDefaultsProjectStableApplicationLayer(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("produto de comandos ausente")
	}
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	projectionAgain, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.BuiltinLayers) != 2 {
		t.Fatalf("camadas builtin = %+v", projection.BuiltinLayers)
	}
	var keyboardLayer commandconfig.BuiltinLayer
	for _, layer := range projection.BuiltinLayers {
		if layer.ID == commandKeyboardLayerID {
			keyboardLayer = layer
		}
	}
	if keyboardLayer.ID != commandKeyboardLayerID || !keyboardLayer.Active || len(keyboardLayer.Defaults) != 67 {
		t.Fatalf("defaults de teclado = %+v", keyboardLayer)
	}
	want := []struct {
		id, trigger, command string
	}{
		{"builtin.keyboard.alt-w.navigation.workspace.open", "keyboard.local:Alt+KeyW", "navigation.workspace.open"},
		{"builtin.keyboard.alt-c.navigation.settings.open", "keyboard.local:Alt+KeyC", "navigation.settings.open"},
		{"builtin.keyboard.alt-h.navigation.history.open", "keyboard.local:Alt+KeyH", "navigation.history.open"},
		{"builtin.keyboard.alt-l.navigation.memories.open", "keyboard.local:Alt+KeyL", "navigation.memories.open"},
		{"builtin.keyboard.alt-t.navigation.tasklists.open", "keyboard.local:Alt+KeyT", "navigation.tasklists.open"},
		{"builtin.keyboard.alt-j.navigation.jobs.open", "keyboard.local:Alt+KeyJ", "navigation.jobs.open"},
		{"builtin.keyboard.alt-p.navigation.profiles.open", "keyboard.local:Alt+KeyP", "navigation.profiles.open"},
		{"builtin.keyboard.alt-backspace.navigation.workspace.open", "keyboard.local:Alt+Backspace", "navigation.workspace.open"},
		{"builtin.keyboard.alt-m.navigation.menu.open", "keyboard.local:Alt+KeyM", "navigation.menu.open"},
		{"builtin.keyboard.ctrl-k.navigation.palette.open", "keyboard.local:Control+KeyK", "navigation.palette.open"},
		{"builtin.keyboard.alt-e.navigation.data.export.open", "keyboard.local:Alt+KeyE", "navigation.data.export.open"},
		{"builtin.keyboard.alt-i.navigation.data.import.open", "keyboard.local:Alt+KeyI", "navigation.data.import.open"},
		{"builtin.keyboard.ctrl-t.workspace.tab.chat.create", "keyboard.local:Control+KeyT", "workspace.tab.chat.create"},
		{"builtin.keyboard.ctrl-w.workspace.tab.close", "keyboard.local:Control+KeyW", "workspace.tab.close"},
		{"builtin.keyboard.ctrl-f4.workspace.tab.close", "keyboard.local:Control+F4", "workspace.tab.close"},
		{"builtin.keyboard.ctrl-tab.workspace.tab.next", "keyboard.local:Control+Tab", "workspace.tab.next"},
		{"builtin.keyboard.ctrl-shift-tab.workspace.tab.previous", "keyboard.local:Control+Shift+Tab", "workspace.tab.previous"},
		{"builtin.keyboard.ctrl-page-down.workspace.tab.next", "keyboard.local:Control+PageDown", "workspace.tab.next"},
		{"builtin.keyboard.ctrl-page-up.workspace.tab.previous", "keyboard.local:Control+PageUp", "workspace.tab.previous"},
		{"builtin.keyboard.ctrl-1.workspace.tab.first", "keyboard.local:Control+Digit1", "workspace.tab.first"},
		{"builtin.keyboard.ctrl-2.workspace.tab.second", "keyboard.local:Control+Digit2", "workspace.tab.second"},
		{"builtin.keyboard.ctrl-3.workspace.tab.third", "keyboard.local:Control+Digit3", "workspace.tab.third"},
		{"builtin.keyboard.ctrl-4.workspace.tab.fourth", "keyboard.local:Control+Digit4", "workspace.tab.fourth"},
		{"builtin.keyboard.ctrl-5.workspace.tab.fifth", "keyboard.local:Control+Digit5", "workspace.tab.fifth"},
		{"builtin.keyboard.ctrl-6.workspace.tab.sixth", "keyboard.local:Control+Digit6", "workspace.tab.sixth"},
		{"builtin.keyboard.ctrl-7.workspace.tab.seventh", "keyboard.local:Control+Digit7", "workspace.tab.seventh"},
		{"builtin.keyboard.ctrl-8.workspace.tab.eighth", "keyboard.local:Control+Digit8", "workspace.tab.eighth"},
		{"builtin.keyboard.ctrl-9.workspace.tab.ninth", "keyboard.local:Control+Digit9", "workspace.tab.ninth"},
		{"builtin.keyboard.f1.navigation.help.open", "keyboard.local:F1", "navigation.help.open"},
		{"builtin.keyboard.ctrl-shift-n.workspace.create", "keyboard.local:Control+Shift+KeyN", "workspace.create"},
		{"builtin.keyboard.ctrl-shift-i.workspace.chat.open", "keyboard.local:Control+Shift+KeyI", "workspace.chat.open"},
		{"builtin.keyboard.ctrl-m.chat.model.open", "keyboard.local:Control+KeyM", "chat.model.open"},
		{"builtin.keyboard.ctrl-h.chat.history.open", "keyboard.local:Control+KeyH", "chat.history.open"},
		{"builtin.keyboard.ctrl-p.chat.profile.open", "keyboard.local:Control+KeyP", "chat.profile.open"},
		{"builtin.keyboard.alt-s.editor.slides.open", "keyboard.local:Alt+KeyS", "editor.slides.open"},
		{"builtin.keyboard.f5.editor.presentation.fullscreen", "keyboard.local:F5", "editor.presentation.fullscreen"},
		{"builtin.keyboard.alt-i.editor.menu.insert.open", "keyboard.local:Alt+KeyI", "editor.menu.insert.open"},
		{"builtin.keyboard.alt-1.editor.mode.markdown", "keyboard.local:Alt+Digit1", "editor.mode.markdown"},
		{"builtin.keyboard.alt-2.editor.mode.rich", "keyboard.local:Alt+Digit2", "editor.mode.rich"},
		{"builtin.keyboard.alt-3.editor.mode.view", "keyboard.local:Alt+Digit3", "editor.mode.view"},
		{"builtin.keyboard.ctrl-s.editor.file.save", "keyboard.local:Control+KeyS", "editor.file.save"},
		{"builtin.keyboard.ctrl-o.editor.file.open", "keyboard.local:Control+KeyO", "editor.file.open"},
		{"builtin.keyboard.ctrl-shift-s.editor.file.save-copy", "keyboard.local:Control+Shift+KeyS", "editor.file.save_copy"},
		{"builtin.keyboard.ctrl-b.editor.format.bold", "keyboard.local:Control+KeyB", "editor.format.bold"},
		{"builtin.keyboard.ctrl-i.editor.format.italic", "keyboard.local:Control+KeyI", "editor.format.italic"},
		{"builtin.keyboard.ctrl-shift-x.editor.format.strike", "keyboard.local:Control+Shift+KeyX", "editor.format.strike"},
		{"builtin.keyboard.ctrl-alt-0.editor.format.paragraph", "keyboard.local:Control+Alt+Digit0", "editor.format.paragraph"},
		{"builtin.keyboard.ctrl-alt-1.editor.format.heading.h1", "keyboard.local:Control+Alt+Digit1", "editor.format.heading.h1"},
		{"builtin.keyboard.ctrl-alt-2.editor.format.heading.h2", "keyboard.local:Control+Alt+Digit2", "editor.format.heading.h2"},
		{"builtin.keyboard.ctrl-alt-3.editor.format.heading.h3", "keyboard.local:Control+Alt+Digit3", "editor.format.heading.h3"},
		{"builtin.keyboard.ctrl-alt-4.editor.format.heading.h4", "keyboard.local:Control+Alt+Digit4", "editor.format.heading.h4"},
		{"builtin.keyboard.ctrl-alt-5.editor.format.heading.h5", "keyboard.local:Control+Alt+Digit5", "editor.format.heading.h5"},
		{"builtin.keyboard.ctrl-alt-6.editor.format.heading.h6", "keyboard.local:Control+Alt+Digit6", "editor.format.heading.h6"},
		{"builtin.keyboard.ctrl-shift-b.editor.format.blockquote", "keyboard.local:Control+Shift+KeyB", "editor.format.blockquote"},
		{"builtin.keyboard.ctrl-alt-c.editor.format.code-block", "keyboard.local:Control+Alt+KeyC", "editor.format.code_block"},
		{"builtin.keyboard.ctrl-shift-8.editor.format.list.bullet", "keyboard.local:Control+Shift+Digit8", "editor.format.list.bullet"},
		{"builtin.keyboard.ctrl-shift-7.editor.format.list.ordered", "keyboard.local:Control+Shift+Digit7", "editor.format.list.ordered"},
		{"builtin.keyboard.ctrl-l.chat.conversation.clear", "keyboard.local:Control+KeyL", "chat.conversation.clear"},
		{"builtin.keyboard.f6.navigation.landmark.next", "keyboard.local:F6", "navigation.landmark.next"},
		{"builtin.keyboard.shift-f6.navigation.landmark.previous", "keyboard.local:Shift+F6", "navigation.landmark.previous"},
		{"builtin.keyboard.ctrl-n.tasklists.create.open", "keyboard.local:Control+KeyN", "tasklists.create.open"},
		{"builtin.keyboard.ctrl-n.profiles.create.open", "keyboard.local:Control+KeyN", "profiles.create.open"},
		{"builtin.keyboard.ctrl-n.history.workspace.open", "keyboard.local:Control+KeyN", "navigation.workspace.open"},
	}
	priorFingerprints := []string{
		"6a383d004ae0c30b6ffc1382150955350e87f5963b9a4fda799f6db347686b08",
		"535d8d2e2176eb25ccd0601919e932df5964fdb169b5be96d520244c9a798e5c",
		"1764c41c0187d01a1264f97f2c44d3f85ef2441e7b2b0b41447c40016fbe873b",
		"3c1c88392f8120bda3c4ceaaf20d01b63f03433792efe40b8193fee4c0f05fe8",
		"7197326fb646bfa380eecc5b9c787ea963153002bb9242237cf9a98b4ca918b3",
		"5952794363940195fe19e29423e3acc8cdbd9045c8fffdd4929425e661c0a3c7",
		"60cb01660c6f08f6c562bf9b3e74dbdfb71306ca00a997d174c3c60ace8cb902",
		"bba09d397b5758b5d0b00c997c5e80826f4717350e0e5cd4a0881abbe99efd04",
		"e7fd1c4b94d730c6cc3c5fd229632f6779c10885604952d0fa7745fe655ab397",
		"0e3eaac97830c3962b9fa65c6d9539a9cbfc163217627bfb4139f0ec413d9149",
		"add4467c402dfe99c0cd87fdc4b34883ae0155243812a3a094594e10cb514727",
		"0c5e4edf48be94c443d7e34d88cb73b30fbc7b13ca81c07abd2b907d44dbe057",
		"bd38d60db2ea246d695d23afb56824210faae337160683a08abbe4cd23acc8f2",
		"5d1ae512757aee47d2f522904b1133cedf9f4af850653be8e3e3ce2e98ed02d4",
		"a8b3bb996160e88e8d5290c8b26cce1f96b025a0a3003a4c16f54b681c400d36",
		"49d789e266eaa6320b97834d60880e53e0d96fb5cc1a5cad0a5a705739273045",
		"65dc3df26fc31ccb65cd82f92b303585da0fc2a5ffbe327ad42757ac14d79655",
		"2b1a800e8fe567cae5b9b1ee30c9ec176f086846f5230f0d7dc183e78ee12768",
		"ae003df64903c99f03af837d50e3df8783ebc5ada4a4646c634b265c431758d8",
		"c2d823c6889145e3b8c0b3638910d5e234512edd21fcc6540cc56e37b10038fa",
		"735e9006b3f198f4b3c2ea6dd2fba96ebfe252b6821e2bdc7ad707b1d2465cc6",
		"0f76635994d1ad8b8afd127f626d65c45813bb836ce1ae37a54c15579a0e7f4e",
		"ca3ed06d6707393a5c311be2ea2ad9eb6b56ec01c1f2067104b7fdb38f941379",
		"388d7be8938815f8b02caf78d37ae3f2e681a099d701f0d013c7390934a43215",
		"500424e6de0ae082b51c05f6cfc09e07a32957eaf2a6223bb2de481b709ddf3f",
		"a202896b16e4f78645739af30793950f08aede640340c4ac0116ea021a05801f",
		"f716df58a29b36c32c9916d04c10f615ca5c2ffb326914d6cb242aca6e070e02",
		"d7621e95df1232bd5df3941cba6ccb70a1bd1d2c5c72546c40cfab9ee762baab",
	}
	const f1Fingerprint = "826a64178fb907dff01436d53236f3b03c82a3d66970de504c873c6ae17a92f6"
	for i, expected := range want {
		got := keyboardLayer.Defaults[i]
		if got.Candidate.ID != expected.id || got.Candidate.Trigger != expected.trigger || got.Candidate.CommandID != expected.command || got.Version != "1" || got.Fingerprint == "" {
			t.Fatalf("default %d = %+v, want %+v", i, got, expected)
		}
		if i < len(priorFingerprints) && got.Fingerprint != priorFingerprints[i] {
			t.Fatalf("fingerprint legado %d mudou: got %s want %s", i, got.Fingerprint, priorFingerprints[i])
		}
		if expected.command == "navigation.help.open" && got.Fingerprint != f1Fingerprint {
			t.Fatalf("fingerprint F1 mudou: got %s want %s", got.Fingerprint, f1Fingerprint)
		}
	}
	sequenceWant := []struct {
		id, trigger, command string
	}{
		{"builtin.keyboard.ctrl-n.workspace.tab.chat.create", "keyboard.local:Control+KeyN KeyC", "workspace.tab.chat.create"},
		{"builtin.keyboard.ctrl-n.workspace.tab.editor.create", "keyboard.local:Control+KeyN KeyE", "workspace.tab.editor.create"},
		{"builtin.keyboard.ctrl-n.workspace.tab.terminal.create", "keyboard.local:Control+KeyN KeyR", "workspace.tab.terminal.create"},
		{"builtin.keyboard.ctrl-n.workspace.tab.tasklist.create", "keyboard.local:Control+KeyN KeyT", "workspace.tab.tasklist.create"},
	}
	for i, expected := range sequenceWant {
		got := keyboardLayer.Defaults[len(want)+i]
		if got.Candidate.ID != expected.id || got.Candidate.Trigger != expected.trigger || got.Candidate.CommandID != expected.command || got.Version != "2" || got.Fingerprint == "" {
			t.Fatalf("default de sequência %d = %+v, want %+v", i, got, expected)
		}
	}
	if keyboardLayer.Defaults[0].Fingerprint == keyboardLayer.Defaults[1].Fingerprint {
		t.Fatal("fingerprints de combinações distintas colidiram")
	}
	if !reflect.DeepEqual(projection.BuiltinLayers[0], projectionAgain.BuiltinLayers[0]) {
		t.Fatal("projeção da paleta mudou entre rebuilds")
	}
	if !reflect.DeepEqual(keyboardLayer, projectionAgain.BuiltinLayers[1]) {
		t.Fatal("projeção do teclado não foi estável entre rebuilds")
	}
	for _, item := range projection.BuiltinLayers[0].Defaults {
		if item.Fingerprint == "" {
			t.Fatalf("fingerprint de paleta ausente: %+v", item)
		}
	}
}

func TestCommandKeyboardDefaultsSuppressAndRestoreThroughRealAPI(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	const workspaceShortcutCode = "KeyW"
	var defaultID string
	for _, item := range projection.BuiltinLayers[1].Defaults {
		if item.Candidate.Trigger == "keyboard.local:Alt+"+workspaceShortcutCode {
			defaultID = item.Candidate.ID
		}
	}
	if defaultID == "" {
		t.Fatal("default Alt+W ausente")
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: workspaceShortcutCode, Modifiers: []string{"Alt"}}
	initial, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(initial.Bindings) != 62 {
		t.Fatalf("mapa inicial: %+v err=%v", initial, err)
	}
	initialBinding := commandKeyboardBindingFor(t, initial, shortcut)
	if initialBinding.CommandID != "navigation.workspace.open" || initialBinding.Handler != "local_ui" {
		t.Fatalf("default Alt+W incorreto: %+v", initialBinding)
	}
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed(defaultID, true)
	})
	suppressed, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(suppressed.Bindings) != 61 {
		t.Fatalf("supressão não retirou default: %+v err=%v", suppressed, err)
	}
	for _, binding := range suppressed.Bindings {
		if reflect.DeepEqual(binding.Shortcut, shortcut) {
			t.Fatalf("Alt+W continuou publicado após supressão: %+v", suppressed)
		}
	}
	if reservation, err := a.BeginLocalCommandUIKey(suppressed.Generation, shortcut, false); !errors.Is(err, commandexecution.ErrDenied) || reservation != nil {
		t.Fatalf("Begin aceitou default suprimido: reservation=%+v err=%v", reservation, err)
	}
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed(defaultID, false)
	})
	restored, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(restored.Bindings) != 62 {
		t.Fatalf("restauração não republicou default: %+v err=%v", restored, err)
	}
	restoredBinding := commandKeyboardBindingFor(t, restored, shortcut)
	if restoredBinding.CommandID != "navigation.workspace.open" || restoredBinding.Handler != "local_ui" {
		t.Fatalf("default restaurado incorreto: %+v", restoredBinding)
	}
	if reservation, err := a.BeginLocalCommandUIKey(restored.Generation, shortcut, false); !errors.Is(err, commandexecution.ErrDenied) || reservation != nil {
		t.Fatalf("Begin de local_ui restaurado criou handoff: reservation=%+v err=%v", reservation, err)
	}
}

func TestCommandChatPickerKeyboardDefaultsAreLocalAndSuppressible(t *testing.T) {
	for _, test := range []struct {
		commandID string
		code      string
	}{
		{commandChatModelOpenID, "KeyM"},
		{commandChatHistoryOpenID, "KeyH"},
		{commandChatProfileOpenID, "KeyP"},
	} {
		t.Run(test.commandID, func(t *testing.T) {
			a, decisions := settingsSecurityFixture(t)
			p := a.commandProduct.Load()
			projection, err := commandProductProjection(p.registry, nil)
			if err != nil {
				t.Fatal(err)
			}
			var defaultID string
			for _, item := range projection.BuiltinLayers[1].Defaults {
				if item.Candidate.CommandID == test.commandID {
					defaultID = item.Candidate.ID
					if item.Candidate.Trigger != "keyboard.local:Control+"+test.code || item.Version != "1" {
						t.Fatalf("default %s inesperado: %+v", test.commandID, item)
					}
				}
			}
			if defaultID == "" {
				t.Fatalf("default ausente: %s", test.commandID)
			}
			shortcut := LocalCommandShortcut{Version: 1, Code: test.code, Modifiers: []string{"Control"}}
			initial, err := a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			binding := commandKeyboardBindingFor(t, initial, shortcut)
			if binding.CommandID != test.commandID || binding.Handler != "local_ui" {
				t.Fatalf("binding local inesperado: %+v", binding)
			}
			settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
				return a.SetDefaultCommandSuppressed(defaultID, true)
			})
			suppressed, err := a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			for _, candidate := range suppressed.Bindings {
				if candidate.CommandID == test.commandID {
					t.Fatalf("default continuou publicado após supressão: %+v", suppressed)
				}
			}
			settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
				return a.SetDefaultCommandSuppressed(defaultID, false)
			})
			restored, err := a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			restoredBinding := commandKeyboardBindingFor(t, restored, shortcut)
			if restoredBinding.CommandID != test.commandID || restoredBinding.Handler != "local_ui" {
				t.Fatalf("default não foi restaurado como local_ui: %+v", restoredBinding)
			}
		})
	}
}

func TestCommandKeyboardPersonalLayerOverridesAltHAndRestoresBuiltin(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Memórias por teclado", Enabled: true})
	})
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{
			LayerID: layer.ID, CommandID: "navigation.memories.open", TriggerType: "keyboard.local",
			TriggerSpec: `{"version":1,"code":"KeyH","modifiers":["Alt"]}`, Enabled: true,
		})
	})
	settingsActivationSecurityConfirmed(t, a, decisions, func() (CommandSettingsMutation, error) {
		return a.PrepareManualCommandLayer(layer.ID)
	})
	if _, err := a.SetCommandLayerActive(layer.ID, true); err != nil {
		t.Fatal(err)
	}
	shortcut := LocalCommandShortcut{Version: 1, Code: "KeyH", Modifiers: []string{"Alt"}}
	active, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	activeBinding := commandKeyboardBindingFor(t, active, shortcut)
	if activeBinding.CommandID != "navigation.memories.open" || activeBinding.Handler != "local_ui" {
		t.Fatalf("camada pessoal não ganhou precedência em Alt+H: %+v", activeBinding)
	}
	if reservation, err := a.BeginLocalCommandUIKey(active.Generation, shortcut, false); !errors.Is(err, commandexecution.ErrDenied) || reservation != nil {
		t.Fatalf("Begin da camada pessoal local_ui criou handoff: reservation=%+v err=%v", reservation, err)
	}
	if result, err := a.DispatchLocalCommandKey(active.Generation, shortcut, "up", false); err != nil || result != nil {
		t.Fatalf("keyup da camada pessoal: result=%+v err=%v", result, err)
	}
	if _, err := a.SetCommandLayerActive(layer.ID, false); err != nil {
		t.Fatal(err)
	}
	restored, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	restoredBinding := commandKeyboardBindingFor(t, restored, shortcut)
	if restoredBinding.CommandID != "navigation.history.open" || restoredBinding.Handler != "local_ui" {
		t.Fatalf("builtin não foi restaurado após desativar camada pessoal: %+v", restoredBinding)
	}
	if reservation, err := a.BeginLocalCommandUIKey(restored.Generation, shortcut, false); !errors.Is(err, commandexecution.ErrDenied) || reservation != nil {
		t.Fatalf("Begin do builtin local_ui restaurado criou handoff: reservation=%+v err=%v", reservation, err)
	}
}
