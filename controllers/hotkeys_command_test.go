package controllers

import (
	"context"
	"testing"

	"assistente/internal/profiles"
)

func TestCommandHotkeyBindingsUsesAuthoritativeActiveProfile(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	profile := controllerProfile("Perfil Ativo", true)
	profile.Input.Enabled = true
	profile.Input.Triggers = []profiles.TriggerConfig{
		{Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Ctrl+Shift+A", HotkeyGlobal: true, HotkeyBringToFront: true},
		{Type: profiles.TriggerTypeHotkey, Enabled: false, Hotkey: "Ctrl+Shift+B"},
	}
	if _, err := manager.Create(profile); err != nil {
		t.Fatal(err)
	}
	c := NewHotkeysController(HotkeysControllerConfig{ProfileMgr: manager})
	bindings, err := c.CommandHotkeyBindings()
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 {
		t.Fatalf("bindings = %#v", bindings)
	}
	binding := bindings[0]
	if binding.ProfileSlug == "" || binding.Hotkey != "Ctrl+Shift+A" || binding.TriggerType != profiles.TriggerTypeHotkey || !binding.Global || !binding.BringToFront || binding.Fingerprint == "" {
		t.Fatalf("binding = %#v", binding)
	}
	bindings[0].Hotkey = "mutado"
	again, err := c.CommandHotkeyBindings()
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Hotkey != "Ctrl+Shift+A" {
		t.Fatal("snapshot expôs memória mutável")
	}
}

func TestProfileHotkeyOccurrenceValidateRejectsForgedAndStaleValues(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	profile := controllerProfile("Perfil Ativo", true)
	profile.Input.Enabled = true
	profile.Input.Triggers = []profiles.TriggerConfig{{Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Ctrl+Shift+A"}}
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	var occurrence ProfileHotkeyOccurrence
	registrar := &profileHotkeyFake{}
	c := NewHotkeysController(HotkeysControllerConfig{
		ProfileMgr: manager,
		DispatchCommandHotkey: func(_ context.Context, got ProfileHotkeyOccurrence) error {
			occurrence = got
			return nil
		},
	})
	c.registrar = registrar
	c.RegisterActiveProfileHotkeys()
	registrar.callbacks[0]()
	if occurrence.Binding().ProfileSlug != slug || !occurrence.Current() {
		t.Fatalf("ocorrência = %#v, current=%v", occurrence.Binding(), occurrence.Current())
	}
	if err := occurrence.Validate(context.Background()); err != nil {
		t.Fatalf("ocorrência válida rejeitada: %v", err)
	}
	if err := (ProfileHotkeyOccurrence{}).Validate(context.Background()); err == nil {
		t.Fatal("ocorrência zero foi aceita")
	}

	updated := *profile
	updated.Input.Triggers = append([]profiles.TriggerConfig(nil), profile.Input.Triggers...)
	updated.Input.Triggers[0].Hotkey = "Ctrl+Shift+B"
	if err := manager.Update(slug, &updated); err != nil {
		t.Fatal(err)
	}
	if !occurrence.Current() {
		t.Fatal("Current consultou autoridade externa em vez de memória")
	}
	if err := occurrence.Validate(context.Background()); err == nil {
		t.Fatal("fingerprint/trigger obsoleto foi aceito")
	}

	c.RegisterActiveProfileHotkeys()
	if occurrence.Current() {
		t.Fatal("reload não aposentou ocorrência")
	}
}

func TestProfileHotkeyOccurrenceContextCanceledOnStop(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	profile := controllerProfile("Perfil Ativo", true)
	profile.Input.Enabled = true
	profile.Input.Triggers = []profiles.TriggerConfig{{Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Ctrl+Shift+A"}}
	if _, err := manager.Create(profile); err != nil {
		t.Fatal(err)
	}
	var occurrence ProfileHotkeyOccurrence
	registrar := &profileHotkeyFake{}
	c := NewHotkeysController(HotkeysControllerConfig{ProfileMgr: manager, DispatchCommandHotkey: func(_ context.Context, got ProfileHotkeyOccurrence) error { occurrence = got; return nil }})
	c.registrar = registrar
	c.RegisterActiveProfileHotkeys()
	registrar.callbacks[0]()
	ctx := occurrence.Context()
	if ctx == nil {
		t.Fatal("ocorrência válida sem contexto")
	}
	c.Stop()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Stop não cancelou contexto da geração")
	}
}
