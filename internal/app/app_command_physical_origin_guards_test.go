package app

import (
	"context"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandexecution"
)

func TestCommandPhysicalOriginMissingSnapshotNeverRecaptures(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	reader := &countingForegroundReader{}
	p := &commandProductRuntime{app: app, principal: principal, workspaceMgr: manager, workspaceID: workspaceID, foregroundReader: reader}
	for _, source := range []commandcatalog.Source{commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck} {
		if _, err := p.commandOriginFactsFromSnapshot(context.Background(), source, []commandbindings.Field{commandbindings.Process}, "physical-device", nil); err == nil {
			t.Fatalf("%s aceitou condição de programa sem captura do evento", source)
		}
	}
	if reader.calls != 0 {
		t.Fatalf("revalidação fez %d capturas nativas tardias", reader.calls)
	}
}

func TestCommandPhysicalDeviceFactComesFromOccurrenceNotCandidate(t *testing.T) {
	app, principal, manager, workspaceID := newCommandWorkspaceProviderFixture(t)
	p := &commandProductRuntime{app: app, principal: principal, workspaceMgr: manager, workspaceID: workspaceID,
		deckExecution: &commandDeckExecution{occurrences: map[string]commandDeckOccurrence{
			"observed": {ctx: context.Background(), serial: "device-observed"},
		}},
	}
	origin, err := p.commandOccurrenceOriginFacts(context.Background(), commandexecution.EnvelopeCandidate{
		InvocationID: "observed", TriggerType: string(commandcatalog.StreamDeck),
		TriggerSpec: []byte(`{"version":1,"device":"device-asserted","key":0}`),
	}, []commandbindings.Field{commandbindings.Device})
	if err != nil || origin.facts[commandbindings.Device] != "device-observed" {
		t.Fatalf("payload substituiu dispositivo observado: %+v err=%v", origin, err)
	}
}
