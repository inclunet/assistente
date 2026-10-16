package app

import (
	"context"
	"testing"

	"assistente/controllers"
	"assistente/internal/commandledger"
	"assistente/internal/configdir"
	"assistente/internal/profiles"
)

// TestCommandGlobalHotkeyLockUnlockRejectsOldCallbackAndRevalidatesResume
// exercises the productive global reservation, UI handoff and executor. The
// old reservation represents a native callback that was minted before the OS
// lock; unlocking alone must not revive it, and a fresh reservation is only
// possible after the OS unlock bootstrap has republished the configuration.
func TestCommandGlobalHotkeyLockUnlockRejectsOldCallbackAndRevalidatesResume(t *testing.T) {
	a, binding := persistentGlobalVoiceExecutionFixture(t)
	ctx := context.Background()
	p := a.commandProduct.Load()

	old := reserveGlobalVoiceForTest(t, a, binding, func() bool { return true })
	second := reserveGlobalVoiceForTest(t, a, binding, func() bool { return true })
	if err := a.commandHost.SetOSSessionState(ctx, true, true); err != nil {
		t.Fatal("lock do SO", err)
	}
	if _, err := p.prepareGlobalOccurrence(ctx, binding, func() bool { return true }); err == nil {
		t.Fatal("prepareGlobalOccurrence aceitou hotkey global durante lock")
	}

	if err := a.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal("unlock do SO", err)
	}
	if _, err := a.TakeGlobalVoiceCommand(old.Ticket); err == nil {
		t.Fatal("callback global antigo foi revivido após unlock")
	}
	_ = a.CancelUICommand(old.Ticket)

	if _, err := p.prepareGlobalOccurrence(ctx, binding, func() bool { return true }); err == nil {
		t.Fatal("prepareGlobalOccurrence aceitou hotkey global antes do rebuild")
	}
	a.bootstrapCommandLifecycleAfterOSUnlock(ctx, a.commandHost)
	if _, err := a.TakeGlobalVoiceCommand(second.Ticket); err == nil {
		t.Fatal("callback global capturado antes do lock foi revivido após rebuild")
	}
	_ = a.CancelUICommand(second.Ticket)

	fresh := reserveGlobalVoiceForTest(t, a, binding, func() bool { return true })
	handoff, err := a.TakeGlobalVoiceCommand(fresh.Ticket)
	if err != nil {
		t.Fatal("callback global novo após revalidação", err)
	}
	if handoff.InvocationID != fresh.InvocationID {
		t.Fatalf("handoff=%+v, invocation=%q", handoff, fresh.InvocationID)
	}
	if err := a.CompleteUICommand(fresh.Ticket, handoff.HandoffID, string(commandledger.Succeeded)); err != nil {
		t.Fatal("commit do hotkey global retomado", err)
	}
	result := getUIResultEventually(t, a, fresh.Ticket)
	if result.Status != string(commandledger.Succeeded) {
		t.Fatalf("resultado=%+v", result)
	}
}

func persistentGlobalVoiceExecutionFixture(t *testing.T) (*App, commandGlobalBinding) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Cleanup(configdir.ResetForTests)
	configdir.ResetForTests()
	a := readyCommandProduct(t)
	a.profileManager = profiles.NewManager()
	profile := profiles.DefaultProfile()
	profile.Name = "Hotkey global lock unlock"
	profile.Active = true
	profile.Input.Enabled = true
	profile.Input.Triggers = []profiles.TriggerConfig{{
		Type: profiles.TriggerTypeHotkey, Enabled: true, Hotkey: "Control+Shift+A",
		HotkeyGlobal: true, HotkeyBringToFront: true,
	}}
	if _, err := a.profileManager.Create(profile); err != nil {
		t.Fatal("persistir perfil global", err)
	}
	a.hotkeyCtrl = controllers.NewHotkeysController(controllers.HotkeysControllerConfig{ProfileMgr: a.profileManager})
	ctx := context.Background()
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal("persistir configuração global", err)
	}
	bindings, err := a.hotkeyCtrl.CommandHotkeyBindings()
	if err != nil || len(bindings) != 1 {
		t.Fatalf("bindings globais persistidos=%+v err=%v", bindings, err)
	}
	binding, err := newCommandGlobalBinding(commandGlobalVoiceID, bindings[0].Hotkey, bindings[0].Fingerprint, map[string]any{
		"profile_slug": bindings[0].ProfileSlug, "trigger_type": bindings[0].TriggerType, "bring_to_front": bindings[0].BringToFront,
	})
	if err != nil {
		t.Fatal("binding global persistido", err)
	}
	return a, binding
}
