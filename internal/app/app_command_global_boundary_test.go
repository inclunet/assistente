package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
)

// Matching source strings and a valid chord cannot substitute for the
// trusted native occurrence, even after the global grammar is available.
func TestCommandProductRejectsUnregisteredGlobalOccurrence(t *testing.T) {
	ctx := context.Background()
	raw := json.RawMessage(`{"version":1,"code":"KeyK","modifiers":["Control"]}`)
	identity, err := (commandconfig.KeyboardGlobalTriggerPort{}).Normalize(ctx, raw)
	if err != nil || identity != "keyboard.global:Control+KeyK" {
		t.Fatalf("global grammar = %q, %v", identity, err)
	}
	ports := commandProductTriggerPorts()
	global, exists := ports[commandcatalog.KeyboardGlobal]
	if !exists {
		t.Fatal("global projection port absent")
	}
	if published, err := global.Normalize(ctx, raw); err != nil || published != identity {
		t.Fatalf("projection changed global identity: %q, %v", published, err)
	}
	local, err := ports[commandcatalog.KeyboardLocal].Normalize(ctx, raw)
	if err != nil || local != "keyboard.local:Control+KeyK" {
		t.Fatalf("global publication changed local grammar: %q, %v", local, err)
	}
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	_, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil {
		t.Fatal(err)
	}
	source := commandcontract.SourceKeyboardGlobal
	envelope := commandcontract.Envelope{SourceType: &source, RegistryVersion: versions.Registry,
		GlobalConfigGeneration: &versions.GlobalConfig, ActiveLayersGeneration: &versions.ActiveLayers}
	candidate := commandexecution.EnvelopeCandidate{TriggerType: string(source), TriggerSpec: raw, Arguments: json.RawMessage(`{}`)}
	if _, err := p.resolvePersistedTrigger(ctx, p.principal, candidate, envelope); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("matching global source strings bypassed native ownership: %v", err)
	}
}
