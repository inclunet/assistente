package app

import (
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
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
	// Current catalog goldens: origins are executable semantics, so a catalog
	// source-set change can require review of persisted, versioned deltas even
	// when default IDs and actions are unchanged.
	currentFingerprints := []string{
		"f0fa6f64bfd8d1e7a4a63e352af0d2078c37a10b3aa2d63036245bbeaf3d6de1",
		"6f09b2dcb2b97c06f12db3a3548977f90ab9aef8fcd8ec2f6c92d30b4b095fe8",
		"c661f21febbda957195f76f2c21ab498016f3f5ac89bc4c533f118e6470f47a1",
		"116c6511cdac8e0d0444cfed59f3b16d04ec79e57ed25f7e2e1a994aa49e182d",
		"c50643345b0d5f89e28ddad7574da96abf5bcf8ca4b0074fc024cab8389a62e0",
		"b0079bd603f3145729eeb2b9f8cfa5c6b5cf5705b2a5e4576bdb2790056070eb",
		"634d9dc839e1c9a2ed0841b170c8072a1a5e9337fb627aafb6b9b4b78a439070",
		"7aac6439c332bb264f5b4a91d619cb4c00654ef6d5c4945d7db5f2828c39ab46",
		"e7fd1c4b94d730c6cc3c5fd229632f6779c10885604952d0fa7745fe655ab397",
		"0e3eaac97830c3962b9fa65c6d9539a9cbfc163217627bfb4139f0ec413d9149",
		"edb59800d974b3cac7d046d2bb2e0f271cc5951e6ca2e640fc7cb167ba75d9fa",
		"9095c043238f529fa2f610c1c3f6650d77735787f16b47aa0f8b271e810c92c5",
		"bd38d60db2ea246d695d23afb56824210faae337160683a08abbe4cd23acc8f2",
		"5d1ae512757aee47d2f522904b1133cedf9f4af850653be8e3e3ce2e98ed02d4",
		"a8b3bb996160e88e8d5290c8b26cce1f96b025a0a3003a4c16f54b681c400d36",
		"cbe8da419b54b1353ea82ed8189abff5e1ed0e7c5c96675eee7cf6dd883b126c",
		"c3a268351c43b6a3eb24d7645cb097fc643d7d339675fb87ff327ddc4f7e7f3b",
		"d5be823c7ee0d45f9b4775cfc287169d788d0f96dddb8f757f1bed7ab2f12df1",
		"fbbe6ac7e365c088ba460b98bb41bd603807aa26dd10142bde103ae3d0996e60",
		"31e09486cf6f6a0678df523037ed506b396f791f53f8ff951dc2b55ffcd26e0a",
		"5c89d9d0e0e3f7933dc1f6d3464cafbd63ae8491a24ca1701bf2dcd1644e59a5",
		"8a974532a43fcc690350e559a0244ce585f88301a89076efa7fdf8db2caf2516",
		"85831b293358dec612c4b81da74daecc466e7d41edbacccef10a2465c1822752",
		"d7dc7dd258348e87a0af41296b8a305bcbd27c094c664c3fe32b50e06f64c355",
		"385a1a9869eac1d7f0d819756f349db17ccbd79c70691fc02d1d303ccdc98666",
		"361d2fa0bbb4441d2cb54ebce5cb230c22b4cbb5ff23d1ebe28b5121cb0845ef",
		"9000d08ac375c2fe8578a0549b2eff760908be9349055d79cd9224b688e08a65",
		"7f97770d566f6b2f2b0cf2f2a0cf9d7c9d9a5cee4a8747b144aa9e28421425d9",
	}
	const f1Fingerprint = "4afa3ec87589cfb28dba7578b6239769d7d3505d7095a43d0c3c70ad4198fd6c"
	for i, expected := range want {
		got := keyboardLayer.Defaults[i]
		if got.Candidate.ID != expected.id || got.Candidate.Trigger != expected.trigger || got.Candidate.CommandID != expected.command || got.Version != "1" || got.Fingerprint == "" {
			t.Fatalf("default %d = %+v, want %+v", i, got, expected)
		}
		if i < len(currentFingerprints) {
			if got.Fingerprint != currentFingerprints[i] {
				t.Errorf("golden atual de fingerprint %d (%s): got %s want %s", i, got.Candidate.CommandID, got.Fingerprint, currentFingerprints[i])
			}
		}
		if expected.command == "navigation.help.open" {
			if got.Fingerprint != f1Fingerprint {
				t.Errorf("golden atual F1 mudou: got %s want %s", got.Fingerprint, f1Fingerprint)
			}
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

func TestCommandKeyboardDefaultFingerprintIncludesAllowedSources(t *testing.T) {
	candidate := commandbindings.Candidate{ID: "builtin.keyboard.test", Trigger: "keyboard.local:Alt+KeyW", CommandID: "navigation.workspace.open", Enabled: true, LayerActive: true}
	base := commandcatalog.Definition{ID: candidate.CommandID, AllowedSources: []commandcatalog.Source{commandcatalog.KeyboardLocal}}
	withUI := commandcatalog.Definition{ID: candidate.CommandID, AllowedSources: []commandcatalog.Source{commandcatalog.KeyboardLocal, commandcatalog.UI}}
	withoutUIFingerprint, err := commandKeyboardDefaultFingerprint(candidate, base, commandKeyboardLayerID)
	if err != nil {
		t.Fatal(err)
	}
	withUIFingerprint, err := commandKeyboardDefaultFingerprint(candidate, withUI, commandKeyboardLayerID)
	if err != nil {
		t.Fatal(err)
	}
	if withUIFingerprint == withoutUIFingerprint {
		t.Fatal("mudança em origens permitidas não alterou fingerprint semântico")
	}
}

func TestOldKeyboardDeltasRequireReviewAfterAllowedSourcesChange(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	p := a.commandProduct.Load()
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	var base commandbindings.Default
	for _, layer := range projection.BuiltinLayers {
		if layer.ID != commandKeyboardLayerID {
			continue
		}
		for _, candidate := range layer.Defaults {
			if candidate.Candidate.ID == "builtin.keyboard.alt-w.navigation.workspace.open" {
				base = candidate
				break
			}
		}
	}
	if base.Candidate.ID == "" || base.Fingerprint == "6a383d004ae0c30b6ffc1382150955350e87f5963b9a4fda799f6db347686b08" {
		t.Fatalf("default atual ausente ou fingerprint não versionado: %+v", base)
	}

	for _, test := range []struct {
		name   string
		effect commandbindings.DeltaEffect
		cmd    string
		args   string
	}{
		{name: "override", effect: commandbindings.Execute, cmd: base.Candidate.CommandID, args: "{}"},
		{name: "suppression", effect: commandbindings.Suppress},
	} {
		t.Run(test.name, func(t *testing.T) {
			delta := commandbindings.Delta{
				ID: "old-keyboard-delta", DefaultID: base.Candidate.ID, DefaultVersion: base.Version,
				DefaultFingerprint: "6a383d004ae0c30b6ffc1382150955350e87f5963b9a4fda799f6db347686b08",
				Trigger:            base.Candidate.Trigger, Effect: test.effect, CommandID: test.cmd, ArgumentsKey: test.args,
				Enabled: true, LayerActive: true, ReviewStatus: commandbindings.Active,
			}
			configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{base}, []commandbindings.Delta{delta}, nil)
			if err != nil {
				t.Fatal(err)
			}
			adjustments := configuration.Adjustments()
			if len(adjustments) != 1 || adjustments[0].ReviewStatus != commandbindings.NeedsReview || adjustments[0].Reason != "changed_default" {
				t.Fatalf("delta antiga não marcada para revisão: %+v", adjustments)
			}
			resolved, err := configuration.Resolve(base.Candidate.Trigger, nil, nil)
			if err != nil || resolved.Status != commandbindings.ReviewRequired || resolved.CommandID != "" {
				t.Fatalf("delta antiga restaurou override/supressão sem revisão: result=%+v err=%v", resolved, err)
			}
		})
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
