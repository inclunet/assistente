package app

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/controllers"
	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/profiles"
)

func TestCommandGlobalRegistrationsAreCompleteAndCataloged(t *testing.T) {
	registrations := commandGlobalRegistrations()
	if len(registrations) != 2 {
		t.Fatalf("registros globais = %d, want 2", len(registrations))
	}
	registry, err := commandcatalog.NewComplete(registrations)
	if err != nil {
		t.Fatalf("registros globais inválidos: %v", err)
	}
	for _, registration := range registrations {
		definition, ok := registry.Lookup(registration.Definition.ID)
		if !ok || !definition.IsComplete() || !definition.AllowsSource(commandcatalog.KeyboardGlobal) {
			t.Fatalf("registro global não catalogado como completo: %#v", registration.Definition)
		}
		if registration.Handler.Effect != definition.Effect || registration.Handler.Route != definition.HandlerRoute || registration.Handler.Classification != definition.HandlerClassification {
			t.Fatalf("contrato do handler diverge da definição %q: %#v", definition.ID, registration.Handler)
		}
	}
}

func TestCommandGlobalBindingNormalizesAliasesAndFingerprint(t *testing.T) {
	first, err := newCommandGlobalBinding(commandGlobalVoiceID, "Ctrl+Shift+A", "profile-fingerprint", map[string]any{"profile_slug": "perfil"})
	if err != nil {
		t.Fatal(err)
	}
	alias, err := newCommandGlobalBinding(commandGlobalVoiceID, "Control+Shift+A", "profile-fingerprint", map[string]any{"profile_slug": "perfil"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Identity != alias.Identity || first.Fingerprint != alias.Fingerprint {
		t.Fatalf("aliases não convergiram: first=%#v alias=%#v", first, alias)
	}
	if first.Identity != "keyboard.global:Control+Shift+KeyA" {
		t.Fatalf("identidade normalizada = %q", first.Identity)
	}
	if first.ID == "" || first.Trigger == nil || first.Arguments == nil {
		t.Fatalf("binding global incompleto: %#v", first)
	}
}

func TestCommandGlobalBindingRejectsMalformedInput(t *testing.T) {
	for _, test := range []struct {
		name string
		keys string
		fp   string
	}{
		{name: "acorde vazio", keys: "", fp: "fingerprint"},
		{name: "modificador desconhecido", keys: "Ctrl+Bogus+A", fp: "fingerprint"},
		{name: "sequência", keys: "Ctrl+A+B", fp: "fingerprint"},
		{name: "fingerprint ausente", keys: "Ctrl+A"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newCommandGlobalBinding(commandGlobalVoiceID, test.keys, test.fp, map[string]any{}); err == nil {
				t.Fatalf("entrada inválida aceita: keys=%q fp=%q", test.keys, test.fp)
			}
		})
	}
}

func TestCommandGlobalProjectionPublishesValidProfileDefaults(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Cleanup(configdir.ResetForTests)
	configdir.ResetForTests()
	a := readyCommandProduct(t)
	a.profileManager = profiles.NewManager()
	profile := profiles.DefaultProfile()
	profile.Name = "Projeção Global"
	profile.Active = true
	profile.Input.Enabled = true
	profile.Input.Triggers = []profiles.TriggerConfig{{
		Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Control+Shift+A", HotkeyGlobal: true, HotkeyBringToFront: true,
	}}
	if _, err := a.profileManager.Create(profile); err != nil {
		t.Fatal(err)
	}
	a.hotkeyCtrl = controllers.NewHotkeysController(controllers.HotkeysControllerConfig{ProfileMgr: a.profileManager})

	registry := a.commandProduct.Load().registry
	projection, err := a.commandProductGlobalProjection(context.Background(), registry, nil)
	if err != nil {
		t.Fatalf("projeção global: %v", err)
	}
	var global commandconfig.BuiltinLayer
	found := false
	for _, layer := range projection.BuiltinLayers {
		if layer.ID == commandGlobalLayerID {
			global = layer
			found = true
			break
		}
	}
	if !found || len(global.Defaults) != 1 {
		t.Fatalf("defaults globais = found=%v defaults=%#v", found, global.Defaults)
	}
	defaultBinding := global.Defaults[0]
	if defaultBinding.Candidate.CommandID != commandGlobalVoiceID || defaultBinding.Candidate.Trigger != "keyboard.global:Control+Shift+KeyA" || defaultBinding.Fingerprint == "" {
		t.Fatalf("default global inválido: %#v", defaultBinding)
	}

	ctx := context.Background()
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	scope, err := a.commandMutationCurrentScope(a.commandProduct.Load().principal)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := commandconfig.ProjectComplete(ctx, snapshot, projection)
	if err != nil {
		t.Fatalf("projeção completa rejeitou defaults globais válidos: %v", err)
	}
	resolved, err := configuration.Resolve(defaultBinding.Candidate.Trigger, commandbindings.Facts{}, nil)
	if err != nil || resolved.Status != commandbindings.Selected || resolved.CommandID != commandGlobalVoiceID {
		t.Fatalf("default global não resolve: result=%#v err=%v", resolved, err)
	}
	if defaultBinding.Candidate.ArgumentsKey == "" {
		t.Fatal("argumentos do default global ausentes")
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(defaultBinding.Candidate.ArgumentsKey), &arguments); err != nil || arguments["profile_slug"] != "projecao-global" {
		t.Fatalf("argumentos globais não canônicos: %s err=%v", defaultBinding.Candidate.ArgumentsKey, err)
	}
}
