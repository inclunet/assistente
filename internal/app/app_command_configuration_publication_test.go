package app

import (
	"context"
	"sync/atomic"
	"testing"

	"assistente/controllers"
	"assistente/internal/commandbindings"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/profiles"
)

func TestCommandConfigurationPublicationPreservesGlobalVoiceBindings(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	a, decisions := settingsSecurityFixture(t)
	a.profileManager = profiles.NewManager()
	profile := profiles.DefaultProfile()
	profile.Name, profile.Active, profile.Input.Enabled = "Configuração global", true, true
	profile.Input.Triggers = []profiles.TriggerConfig{{Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Control+Shift+A", HotkeyGlobal: true, HotkeyBringToFront: true}}
	if _, err := a.profileManager.Create(profile); err != nil {
		t.Fatal(err)
	}
	a.hotkeyCtrl = controllers.NewHotkeysController(controllers.HotkeysControllerConfig{ProfileMgr: a.profileManager})
	p := a.commandProduct.Load()
	inputs, err := a.commandDesktopMutationInputs(p, a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	scope, err := a.commandMutationCurrentScope(p.principal)
	if err != nil {
		t.Fatal(err)
	}
	options, err := inputs.Projection(a.ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(a.ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	configuration, _, _, err := inputs.BuildConfiguration(a.ctx, scope, snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := configuration.Resolve("keyboard.global:Control+Shift+KeyA", commandbindings.Facts{}, nil)
	if err != nil || resolved.Status != commandbindings.Selected || resolved.CommandID != commandGlobalVoiceID {
		t.Fatalf("global lost after configuration build: %+v %v", resolved, err)
	}
	// The applier publishes a map notification exactly after a successful rebuild.
	var notifications atomic.Int32
	a.emitter = globalJobTestEmitter(func(name string, _ any) {
		if name == "command:keyboard-map-changed" {
			notifications.Add(1)
		}
	})
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Camada publicada", Enabled: true})
	})
	appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	if out := settingsSecurityFinish(t, done); out.err != nil || !out.result.Published {
		t.Fatalf("mutation: %+v", out)
	}
	if notifications.Load() == 0 {
		t.Fatal("no keyboard map notification after mutation")
	}
	published, _, _, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = published.Resolve("keyboard.global:Control+Shift+KeyA", commandbindings.Facts{}, nil)
	if err != nil || resolved.Status != commandbindings.Selected || resolved.CommandID != commandGlobalVoiceID {
		t.Fatalf("mutation lost global binding: %+v %v", resolved, err)
	}
}
