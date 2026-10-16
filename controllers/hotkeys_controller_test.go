package controllers

import (
	"context"
	"sync"
	"testing"
	"time"

	"assistente/internal/hotkey"
	"assistente/internal/profiles"
)

type blockingHotkeyDispatcher struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	ctx     context.Context
}

func (d *blockingHotkeyDispatcher) dispatch(ctx context.Context, _ ProfileHotkeyOccurrence) error {
	d.ctx = ctx
	d.once.Do(func() { close(d.entered) })
	select {
	case <-d.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestHotkeysControllerDispatchIsOutsideLockAndStopCancelsGeneration(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	profile := controllerProfile("Voz", true)
	profile.Input.Enabled = true
	profile.Input.Triggers = []profiles.TriggerConfig{{Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Ctrl+Shift+A"}}
	if _, err := manager.Create(profile); err != nil {
		t.Fatal(err)
	}
	dispatcher := &blockingHotkeyDispatcher{entered: make(chan struct{}), release: make(chan struct{})}
	registrar := &profileHotkeyFake{}
	c := NewHotkeysController(HotkeysControllerConfig{ProfileMgr: manager, DispatchCommandHotkey: dispatcher.dispatch})
	c.registrar = registrar
	c.RegisterActiveProfileHotkeys()

	delivered := make(chan struct{})
	go func() {
		registrar.callbacks[0]()
		close(delivered)
	}()
	<-dispatcher.entered

	stopped := make(chan struct{})
	go func() {
		c.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop bloqueou no dispatcher fora do mutex")
	}
	if dispatcher.ctx == nil {
		t.Fatal("dispatcher não recebeu contexto")
	}
	select {
	case <-dispatcher.ctx.Done():
	default:
		t.Fatal("Stop não cancelou o contexto da geração")
	}
	close(dispatcher.release)
	<-delivered
}

func TestHotkeysControllerRetiresCallbacksOnReloadAndStop(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	profile := controllerProfile("Voz", true)
	profile.Input.Enabled = true
	profile.Input.Triggers = []profiles.TriggerConfig{{Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Ctrl+Shift+A"}}
	if _, err := manager.Create(profile); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var occurrences []ProfileHotkeyOccurrence
	dispatch := func(_ context.Context, occurrence ProfileHotkeyOccurrence) error {
		mu.Lock()
		defer mu.Unlock()
		occurrences = append(occurrences, occurrence)
		return nil
	}
	registrar := &profileHotkeyFake{}
	c := NewHotkeysController(HotkeysControllerConfig{ProfileMgr: manager, DispatchCommandHotkey: dispatch})
	c.registrar = registrar
	c.RegisterActiveProfileHotkeys()
	if len(registrar.callbacks) != 1 {
		t.Fatalf("callbacks: %d", len(registrar.callbacks))
	}
	old := registrar.callbacks[0]
	old()
	old()
	mu.Lock()
	if len(occurrences) != 1 {
		mu.Unlock()
		t.Fatal("throttle não suprimiu duplicata")
	}
	oldOccurrence := occurrences[0]
	mu.Unlock()

	c.RegisterActiveProfileHotkeys()
	if oldOccurrence.Current() {
		t.Fatal("ocorrência antiga continuou atual após reload")
	}
	old()
	mu.Lock()
	if len(occurrences) != 1 {
		mu.Unlock()
		t.Fatal("callback aposentado ativou o dispatcher")
	}
	mu.Unlock()
	registrar.callbacks[1]()
	mu.Lock()
	if len(occurrences) != 2 {
		mu.Unlock()
		t.Fatal("novo registro herdou throttle antigo")
	}
	mu.Unlock()
	c.Stop()
	registrar.callbacks[1]()
	mu.Lock()
	defer mu.Unlock()
	if len(occurrences) != 2 {
		t.Fatal("callback parado ativou o dispatcher")
	}
}

func TestHotkeysControllerDisabledInputRegistersNothing(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	profile := controllerProfile("Voz desativada", true)
	profile.Input.Enabled = false
	profile.Input.Triggers = []profiles.TriggerConfig{{Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Ctrl+Shift+A"}}
	if _, err := manager.Create(profile); err != nil {
		t.Fatal(err)
	}
	registrar := &profileHotkeyFake{}
	c := NewHotkeysController(HotkeysControllerConfig{ProfileMgr: manager, DispatchCommandHotkey: func(context.Context, ProfileHotkeyOccurrence) error { return nil }})
	c.registrar = registrar
	c.RegisterActiveProfileHotkeys()
	if len(registrar.callbacks) != 0 {
		t.Fatal("input desativado registrou hotkey")
	}
}

type profileHotkeyFake struct {
	callbacks []hotkey.HotkeyCallback
	removed   int
}

func (r *profileHotkeyFake) RegisterProfileHotkey(_ int, _ string, _, _ bool, callback hotkey.HotkeyCallback) (int, error) {
	r.callbacks = append(r.callbacks, callback)
	return len(r.callbacks), nil
}

func (r *profileHotkeyFake) UnregisterAllProfileHotkeys() { r.removed++ }
