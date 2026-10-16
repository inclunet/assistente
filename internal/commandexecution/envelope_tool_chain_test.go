package commandexecution

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
)

func TestPrepareCommandChainSeedsDirectLocalTool(t *testing.T) {
	invocationID := newTestUUID()
	commandID := "tool.workspace.list"

	got, err := prepareCommandChain(commandcontract.Envelope{
		InvocationID:    invocationID,
		CommandID:       &commandID,
		AuthContextType: commandcontract.AuthLocalSession,
		ActorType:       commandcontract.ActorUser,
	}, commandcatalog.Definition{
		ID:                    commandID,
		Effect:                commandcatalog.Write,
		HandlerClassification: commandcatalog.HandlerTool,
	}, []string{"layer.direct-tool"})
	if err != nil {
		t.Fatalf("seed da ferramenta direta: %v", err)
	}

	document := decodeToolChainDocument(t, got)
	var chainID string
	if err := json.Unmarshal(document["_chain_id"], &chainID); err != nil || chainID != invocationID {
		t.Fatalf("_chain_id inesperado: %q", chainID)
	}
	var jobHistory []string
	if err := json.Unmarshal(document["_chain_history"], &jobHistory); err != nil || jobHistory == nil || len(jobHistory) != 0 {
		t.Fatalf("_chain_history inicial inválido: %s", document["_chain_history"])
	}
	history, err := commandcontract.DecodeCommandChainHistory(document["command_chain_history"])
	if err != nil || len(history) != 1 {
		t.Fatalf("entrada da ferramenta ausente: history=%+v err=%v", history, err)
	}
	if history[0].CommandID != commandID || history[0].InvocationID != invocationID || len(history[0].LayerRefs) != 1 || history[0].LayerRefs[0] != "layer.direct-tool" {
		t.Fatalf("entrada inicial incorreta: %+v", history[0])
	}
}

func TestPrepareCommandChainReactiveToolPreservesRootAndHistory(t *testing.T) {
	commandID := "tool.workspace.rename"
	invocationID := newTestUUID()
	provenance := json.RawMessage(`{"version":1,"_chain_id":"job-root","_chain_history":["job-source"],"command_chain_history":[{"command_id":"tool.workspace.list","invocation_id":"` + newTestUUID() + `","layer_refs":["layer.parent"]}]}`)

	got, err := prepareCommandChain(commandcontract.Envelope{
		InvocationID:    invocationID,
		CommandID:       &commandID,
		AuthContextType: commandcontract.AuthLocalSession,
		ActorType:       commandcontract.ActorUser,
		Provenance:      &provenance,
	}, commandcatalog.Definition{
		ID:                    commandID,
		Effect:                commandcatalog.Write,
		HandlerClassification: commandcatalog.HandlerTool,
	}, []string{"layer.child"})
	if err != nil {
		t.Fatalf("cadeia reativa de ferramenta: %v", err)
	}

	document := decodeToolChainDocument(t, got)
	var chainID string
	var jobHistory []string
	if json.Unmarshal(document["_chain_id"], &chainID) != nil || chainID != "job-root" {
		t.Fatalf("raiz alterada: %q", chainID)
	}
	if json.Unmarshal(document["_chain_history"], &jobHistory) != nil || len(jobHistory) != 1 || jobHistory[0] != "job-source" {
		t.Fatalf("histórico de jobs alterado: %v", jobHistory)
	}
	history, err := commandcontract.DecodeCommandChainHistory(document["command_chain_history"])
	if err != nil || len(history) != 2 {
		t.Fatalf("histórico de ferramentas inesperado: history=%+v err=%v", history, err)
	}
	if history[0].CommandID != "tool.workspace.list" || history[1].CommandID != commandID || history[1].InvocationID != invocationID || len(history[1].LayerRefs) != 1 || history[1].LayerRefs[0] != "layer.child" {
		t.Fatalf("cadeia reativa incorreta: %+v", history)
	}
}

func TestPrepareCommandChainToolRejectsReplayAndDepthOverflow(t *testing.T) {
	commandID := "tool.workspace.write"
	invocationID := newTestUUID()
	provenance := json.RawMessage(`{"version":1,"_chain_id":"root","_chain_history":[],"command_chain_history":[]}`)
	envelope := commandcontract.Envelope{InvocationID: invocationID, CommandID: &commandID, Provenance: &provenance}
	definition := commandcatalog.Definition{ID: commandID, Effect: commandcatalog.Write, HandlerClassification: commandcatalog.HandlerTool}

	got, err := prepareCommandChain(envelope, definition, []string{"layer.tool"})
	if err != nil {
		t.Fatalf("primeira ferramenta: %v", err)
	}
	if _, err := prepareCommandChain(got, definition, []string{"layer.tool"}); !errors.Is(err, ErrDenied) {
		t.Fatalf("replay da ferramenta aceito: %v", err)
	}

	for _, length := range []int{CommandMaxChainDepth - 1, CommandMaxChainDepth} {
		history := make([]commandcontract.CommandChainEntry, 0, length)
		for i := 0; i < length; i++ {
			history = append(history, commandcontract.CommandChainEntry{
				CommandID:    fmt.Sprintf("tool.chain.x%d", i),
				InvocationID: newTestUUID(),
				LayerRefs:    []string{"layer.tool"},
			})
		}
		document := map[string]any{
			"version":               1,
			"_chain_id":             "root",
			"_chain_history":        []string{},
			"command_chain_history": history,
		}
		raw, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		candidate := envelope
		candidate.Provenance = func() *json.RawMessage { value := json.RawMessage(raw); return &value }()
		_, err = prepareCommandChain(candidate, definition, []string{"layer.tool"})
		if (length == CommandMaxChainDepth) != errors.Is(err, ErrDenied) {
			t.Fatalf("limite da cadeia length=%d err=%v", length, err)
		}
	}
}

func decodeToolChainDocument(t *testing.T, envelope commandcontract.Envelope) map[string]json.RawMessage {
	t.Helper()
	if envelope.Provenance == nil {
		t.Fatal("provenance não publicada")
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(*envelope.Provenance, &document); err != nil {
		t.Fatalf("decodificar provenance: %v", err)
	}
	return document
}
