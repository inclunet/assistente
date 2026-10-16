package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
	"assistente/internal/commandforeground"
)

type countingForegroundReader struct{ calls int }

func (r *countingForegroundReader) Capture(context.Context) (commandforeground.Snapshot, error) {
	r.calls++
	return commandforeground.Snapshot{}, commandexecution.ErrDenied
}

func TestCommandOccurrenceOriginFactsDeviceOnlyUsesSerialAndDoesNotCapture(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	reader := &countingForegroundReader{}
	p := &commandProductRuntime{
		app: app, principal: principal, workspaceMgr: manager, workspaceID: workspaceID,
		foregroundReader: reader,
		deckExecution: &commandDeckExecution{occurrences: map[string]commandDeckOccurrence{
			"deck-invocation": {serial: "SERIAL-7", ctx: context.Background()},
		}},
	}
	origin, err := p.commandOccurrenceOriginFacts(context.Background(), commandexecution.EnvelopeCandidate{
		InvocationID: "deck-invocation", TriggerType: string(commandcatalog.StreamDeck),
		TriggerSpec: []byte(`{"version":1,"device":"SERIAL-7","key":2}`),
	}, []commandbindings.Field{commandbindings.Device})
	if err != nil {
		t.Fatalf("device-only origin: %v", err)
	}
	if got := origin.facts[commandbindings.Device]; got != "SERIAL-7" || origin.version == "" {
		t.Fatalf("origin device/version = %#v/%q", got, origin.version)
	}
	if reader.calls != 0 {
		t.Fatalf("device-only invoked foreground reader %d time(s)", reader.calls)
	}
}

func TestCommandOccurrenceOriginFactsRejectsPhysicalContextWithoutOccurrence(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	reader := &countingForegroundReader{}
	p := &commandProductRuntime{app: app, principal: principal, workspaceMgr: manager, workspaceID: workspaceID, foregroundReader: reader, deckExecution: &commandDeckExecution{occurrences: map[string]commandDeckOccurrence{}}}
	_, err := p.commandOccurrenceOriginFacts(context.Background(), commandexecution.EnvelopeCandidate{InvocationID: "missing", TriggerType: string(commandcatalog.KeyboardGlobal)}, []commandbindings.Field{commandbindings.Process})
	if err == nil {
		t.Fatal("process physical sem ocorrência foi aceito")
	}
	if reader.calls != 0 {
		t.Fatalf("resolver capturou foreground nativo %d vez(es)", reader.calls)
	}
}
