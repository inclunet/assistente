package commandexecution

import (
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
)

func TestPrepareCommandChainSeedsDirectLocalJob(t *testing.T) {
	invocationID := newTestUUID()
	commandID := "job.execute"
	envelope := commandcontract.Envelope{
		InvocationID:    invocationID,
		CommandID:       &commandID,
		AuthContextType: commandcontract.AuthLocalSession,
		ActorType:       commandcontract.ActorUser,
	}
	definition := commandcatalog.Definition{ID: commandID, Effect: commandcatalog.Write, HandlerClassification: commandcatalog.HandlerJob}

	got, err := prepareCommandChain(envelope, definition, []string{"layer.job"})
	if err != nil {
		t.Fatalf("seed local job: %v", err)
	}
	if got.Provenance == nil {
		t.Fatal("seed não publicou provenance")
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(*got.Provenance, &document); err != nil {
		t.Fatalf("decodificar provenance: %v", err)
	}
	var version int
	if err := json.Unmarshal(document["version"], &version); err != nil || version != 1 {
		t.Fatalf("version inesperada: %s", document["version"])
	}
	var chainID string
	if err := json.Unmarshal(document["_chain_id"], &chainID); err != nil || chainID != invocationID {
		t.Fatalf("_chain_id não foi semeado pela invocation: %q", chainID)
	}
	var jobHistory []string
	if err := json.Unmarshal(document["_chain_history"], &jobHistory); err != nil || jobHistory == nil || len(jobHistory) != 0 {
		t.Fatalf("_chain_history inicial inválida: %s", document["_chain_history"])
	}
	history, err := commandcontract.DecodeCommandChainHistory(document["command_chain_history"])
	if err != nil || len(history) != 1 {
		t.Fatalf("primeira entrada ausente: history=%+v err=%v", history, err)
	}
	if history[0].CommandID != commandID || history[0].InvocationID != invocationID || len(history[0].LayerRefs) != 1 || history[0].LayerRefs[0] != "layer.job" {
		t.Fatalf("primeira entrada incorreta: %+v", history[0])
	}
}

func TestPrepareCommandChainRejectsUnsupportedNilProvenance(t *testing.T) {
	for _, tc := range []struct {
		name  string
		auth  commandcontract.AuthContextType
		actor commandcontract.ActorType
	}{
		{name: "agent", auth: commandcontract.AuthExternalToken, actor: commandcontract.ActorAgent},
		{name: "system", auth: commandcontract.AuthSystem, actor: commandcontract.ActorAutomation},
		{name: "job service", auth: commandcontract.AuthJobService, actor: commandcontract.ActorAutomation},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commandID := "job.execute"
			got, err := prepareCommandChain(commandcontract.Envelope{
				InvocationID:    newTestUUID(),
				CommandID:       &commandID,
				AuthContextType: tc.auth,
				ActorType:       tc.actor,
			}, commandcatalog.Definition{ID: commandID, Effect: commandcatalog.Write, HandlerClassification: commandcatalog.HandlerJob}, nil)
			if !errors.Is(err, ErrDenied) {
				t.Fatalf("provenance nil aceita para %s: err=%v envelope=%+v", tc.name, err, got)
			}
		})
	}
}

func TestPrepareCommandChainPreservesExistingReactiveProvenance(t *testing.T) {
	commandID := "job.execute"
	invocationID := newTestUUID()
	provenance := json.RawMessage(`{"version":1,"_chain_id":"existing-chain","_chain_history":["job-root"],"command_chain_history":[]}`)
	got, err := prepareCommandChain(commandcontract.Envelope{
		InvocationID:    invocationID,
		CommandID:       &commandID,
		AuthContextType: commandcontract.AuthLocalSession,
		ActorType:       commandcontract.ActorUser,
		Provenance:      &provenance,
	}, commandcatalog.Definition{ID: commandID, Effect: commandcatalog.Write, HandlerClassification: commandcatalog.HandlerJob}, []string{"layer.existing"})
	if err != nil {
		t.Fatalf("preservar provenance reativa: %v", err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(*got.Provenance, &document); err != nil {
		t.Fatal(err)
	}
	var chainID string
	var jobHistory []string
	if json.Unmarshal(document["_chain_id"], &chainID) != nil || chainID != "existing-chain" {
		t.Fatalf("_chain_id foi resetado: %q", chainID)
	}
	if json.Unmarshal(document["_chain_history"], &jobHistory) != nil || len(jobHistory) != 1 || jobHistory[0] != "job-root" {
		t.Fatalf("_chain_history foi resetado: %v", jobHistory)
	}
	history, err := commandcontract.DecodeCommandChainHistory(document["command_chain_history"])
	if err != nil || len(history) != 1 || history[0].InvocationID != invocationID {
		t.Fatalf("entrada atual não foi anexada: history=%+v err=%v", history, err)
	}
}
