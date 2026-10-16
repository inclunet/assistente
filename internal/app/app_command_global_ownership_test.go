package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/controllers"
	"assistente/internal/hotkey"
)

func TestAppGlobalCommandRegistrationRequiresOwnershipBarrier(t *testing.T) {
	for _, application := range []*App{nil, {}, {hotkeyCtrl: controllers.NewHotkeysController(controllers.HotkeysControllerConfig{})}} {
		if registrar := application.commandGlobalHotkeyRegistrar(); registrar != nil {
			t.Fatal("global registrar exposed without ownership barrier")
		}
		// A ausência de infraestrutura deve impedir o registro antes de consultar
		// perfis ou inicializar qualquer listener nativo.
		application.registerActiveProfileHotkeys()
	}
}

func TestAppGlobalCommandOwnershipAPIsUseCopiedSnapshotAndExactACK(t *testing.T) {
	published := make(chan hotkey.OwnershipFrame, 1)
	barrier := hotkey.NewOwnershipBarrier(func(frame hotkey.OwnershipFrame) { published <- frame })
	defer barrier.Close()
	application := &App{globalHotkeyOwnership: barrier}

	result := make(chan error, 1)
	go func() {
		result <- barrier.Publish(context.Background(), []hotkey.OwnershipCombination{{Key: 0x41, Modifiers: 0x06}})
	}()
	frame := receiveAppOwnershipFrame(t, published)

	snapshot := application.GetGlobalCommandOwnership()
	wire, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(wire, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 5 {
		t.Fatalf("ownership wire fields = %s; frontend accepts exactly the versioned frame", wire)
	}
	for _, name := range []string{"version", "instanceId", "revision", "platform", "combinations"} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("ownership wire lacks %s: %s", name, wire)
		}
	}
	if snapshot.Version != 1 || snapshot.Platform != "windows" || snapshot.Revision != frame.Revision || snapshot.InstanceID != frame.InstanceID {
		t.Fatalf("App snapshot = %#v, want published frame metadata", snapshot)
	}
	snapshot.Combinations[0].Key = 0x99
	if got := application.GetGlobalCommandOwnership().Combinations[0].Key; got != 0x41 {
		t.Fatalf("App getter returned aliased combinations: %#x", got)
	}

	if application.AckGlobalCommandOwnership("foreign-instance", frame.Revision) {
		t.Fatal("cross-instance ACK was accepted")
	}
	if application.AckGlobalCommandOwnership(frame.InstanceID, 0) {
		t.Fatal("revision-zero ACK was accepted")
	}
	if application.AckGlobalCommandOwnership(frame.InstanceID, frame.Revision+1) {
		t.Fatal("future ACK was accepted")
	}
	if !application.AckGlobalCommandOwnership(frame.InstanceID, frame.Revision) {
		t.Fatal("exact ACK was rejected")
	}
	if application.AckGlobalCommandOwnership(frame.InstanceID, frame.Revision) {
		t.Fatal("ACK replay was accepted")
	}
	if err := <-result; err != nil {
		t.Fatalf("Publish returned error: %v", err)
	}
}

func TestAppGlobalCommandOwnershipUnsupportedWithoutBarrier(t *testing.T) {
	application := &App{}
	snapshot := application.GetGlobalCommandOwnership()
	if snapshot.Version != 1 || snapshot.Platform != "unsupported" || snapshot.Revision != 0 || snapshot.InstanceID != "" {
		t.Fatalf("unsupported snapshot = %#v", snapshot)
	}
	if snapshot.Combinations == nil || len(snapshot.Combinations) != 0 {
		t.Fatalf("unsupported combinations = %#v, want non-nil empty slice", snapshot.Combinations)
	}
	if application.AckGlobalCommandOwnership("any", 1) {
		t.Fatal("unsupported App accepted ACK")
	}

	var nilApp *App
	if got := nilApp.GetGlobalCommandOwnership(); got.Platform != "unsupported" || got.Combinations == nil {
		t.Fatalf("nil App snapshot = %#v", got)
	}
	if nilApp.AckGlobalCommandOwnership("any", 1) {
		t.Fatal("nil App accepted ACK")
	}
}

func receiveAppOwnershipFrame(t *testing.T, published <-chan hotkey.OwnershipFrame) hotkey.OwnershipFrame {
	t.Helper()
	select {
	case frame := <-published:
		return frame
	case <-time.After(time.Second):
		t.Fatal("App ownership frame was not published")
		return hotkey.OwnershipFrame{}
	}
}
