package commandexecution

import (
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"testing"
)

func TestCommandChainSeparateFromJobsAndLimit(t *testing.T) {
	id := "test.command"
	provenance := json.RawMessage(`{"version":1,"_chain_id":"job-chain","_chain_history":["job-a"]}`)
	e := commandcontract.Envelope{InvocationID: uuid.Must(uuid.NewV7()).String(), CommandID: &id, Provenance: &provenance}
	d := commandcatalog.Definition{ID: id, Effect: commandcatalog.Write}
	got, err := prepareCommandChain(e, d, []string{"application.defaults"})
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(*got.Provenance, &doc); err != nil {
		t.Fatal(err)
	}
	if string(doc["_chain_history"]) != `["job-a"]` {
		t.Fatal("histórico jobs modificado")
	}
	if _, err := prepareCommandChain(got, d, nil); err == nil {
		t.Fatal("loop aceito")
	}
	history := []commandChainEntry{}
	for i := range CommandMaxChainDepth {
		history = append(history, commandChainEntry{CommandID: fmt.Sprintf("test.command%d", i), InvocationID: uuid.Must(uuid.NewV7()).String(), LayerRefs: []string{}})
	}
	for _, length := range []int{15, 16} {
		doc["command_chain_history"], _ = json.Marshal(history[:length])
		raw, _ := json.Marshal(doc)
		p := json.RawMessage(raw)
		e.Provenance = &p
		_, err := prepareCommandChain(e, d, nil)
		if (length == 16) != (err != nil) {
			t.Fatalf("limite length=%d err=%v", length, err)
		}
	}
}

func TestCommandChainRejectsMalformedOrMissingReactiveProvenance(t *testing.T) {
	id := "test.command"
	e := commandcontract.Envelope{InvocationID: uuid.Must(uuid.NewV7()).String(), CommandID: &id, AuthContextType: commandcontract.AuthJobService}
	d := commandcatalog.Definition{ID: id, Effect: commandcatalog.Write}
	for _, raw := range []string{"", `{}`, `{"_chain_id":"x","_chain_history":null}`, `{"_chain_id":"x","_chain_history":[],"command_chain_history":null}`, `{"_chain_id":"x","_chain_history":[],"command_chain_history":[{"command_id":"x","invocation_id":"bad","layer_refs":[]}]}`} {
		e.Provenance = nil
		if raw != "" {
			p := json.RawMessage(raw)
			e.Provenance = &p
		}
		if _, err := prepareCommandChain(e, d, nil); err == nil {
			t.Fatalf("proveniência inválida aceita: %s", raw)
		}
	}
}
