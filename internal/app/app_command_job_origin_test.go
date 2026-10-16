package app

import (
	"encoding/json"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandjobactivation"
	"assistente/internal/jobs"
	"github.com/google/uuid"
)

func TestCommandJobOriginRootIsPreserved(t *testing.T) {
	in, proof := commandJobOriginFixture(t, []string{"layer.selected"}, "manual", "root-1", nil)

	origin, err := commandJobOriginFromProjection(in, proof)
	if err != nil {
		t.Fatalf("origem válida recusada: %v", err)
	}
	if origin != (jobs.CommandJobOrigin{RootOriginType: "manual", RootOriginID: "root-1"}) {
		t.Fatalf("origem=%+v", origin)
	}
}

func TestCommandJobOriginAcceptsMultipleClaimsWithSameRoot(t *testing.T) {
	in, proof := commandJobOriginFixture(t, []string{"layer.a", "layer.b"}, "internal_event", "event-1", nil)

	proof.Claims = append(proof.Claims, commandjobactivation.ProjectionClaim{
		Claim:          commandactivation.Claim{ActivationID: "activation-b", LayerRefKind: commandactivation.UserRef, LayerRef: "layer.b"},
		RootOriginType: "internal_event",
		RootOriginID:   "event-1",
		Provenance:     proof.Claims[0].Provenance,
	})
	origin, err := commandJobOriginFromProjection(in, proof)
	if err != nil {
		t.Fatalf("claims da mesma raiz recusadas: %v", err)
	}
	if origin.RootOriginType != "internal_event" || origin.RootOriginID != "event-1" {
		t.Fatalf("origem=%+v", origin)
	}
}

func TestCommandJobOriginRejectsDivergentRootsWithSameChain(t *testing.T) {
	in, proof := commandJobOriginFixture(t, []string{"layer.a", "layer.b"}, "manual", "root-a", nil)
	proof.Claims = append(proof.Claims, commandjobactivation.ProjectionClaim{
		Claim:          commandactivation.Claim{ActivationID: "activation-b", LayerRefKind: commandactivation.UserRef, LayerRef: "layer.b"},
		RootOriginType: "manual",
		RootOriginID:   "root-b",
		Provenance:     proof.Claims[0].Provenance,
	})

	if _, err := commandJobOriginFromProjection(in, proof); err == nil {
		t.Fatal("raízes divergentes com a mesma cadeia foram aceitas")
	}
}

func TestCommandJobOriginIgnoresUnselectedClaim(t *testing.T) {
	in, proof := commandJobOriginFixture(t, []string{"layer.selected"}, "manual", "root-1", nil)
	proof.Claims = append(proof.Claims, commandjobactivation.ProjectionClaim{
		Claim:          commandactivation.Claim{ActivationID: "activation-unselected", LayerRefKind: commandactivation.UserRef, LayerRef: "layer.unselected"},
		RootOriginType: "forged-root",
		RootOriginID:   "forged-id",
		// A claim não selecionada não pode influenciar a composição.
	})
	proof.Claims = append(proof.Claims, commandjobactivation.ProjectionClaim{
		Claim:          commandactivation.Claim{ActivationID: "activation-builtin", LayerRefKind: commandactivation.BuiltinRef, LayerRef: "layer.selected"},
		RootOriginType: "forged-root",
		RootOriginID:   "forged-id",
	})

	origin, err := commandJobOriginFromProjection(in, proof)
	if err != nil || origin.RootOriginID != "root-1" {
		t.Fatalf("claim não selecionada influenciou: origin=%+v err=%v", origin, err)
	}
}

func TestCommandJobOriginRejectsAdulteratedChainOrPrefix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*commandexecution.Invocation, *commandjobactivation.ProjectionSnapshot)
	}{
		{
			name: "chain id",
			mutate: func(in *commandexecution.Invocation, _ *commandjobactivation.ProjectionSnapshot) {
				var document map[string]any
				if err := json.Unmarshal(*in.Envelope.Provenance, &document); err != nil {
					panic(err)
				}
				document["_chain_id"] = "different-chain"
				rawBytes, _ := json.Marshal(document)
				raw := json.RawMessage(rawBytes)
				in.Envelope.Provenance = &raw
			},
		},
		{
			name: "prefix",
			mutate: func(_ *commandexecution.Invocation, proof *commandjobactivation.ProjectionSnapshot) {
				var document map[string]json.RawMessage
				if err := json.Unmarshal(proof.Claims[0].Provenance, &document); err != nil {
					panic(err)
				}
				document["command_chain_history"] = json.RawMessage(`[{"command_id":"prior.command","invocation_id":"01999999-9999-7999-8999-999999999999","layer_refs":[]}]`)
				rawBytes, _ := json.Marshal(document)
				raw := json.RawMessage(rawBytes)
				proof.Claims[0].Provenance = raw
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, proof := commandJobOriginFixture(t, []string{"layer.selected"}, "manual", "root-1", nil)
			tt.mutate(&in, &proof)
			if _, err := commandJobOriginFromProjection(in, proof); err == nil {
				t.Fatal("proveniência adulterada foi aceita")
			}
		})
	}
}

func TestCommandJobOriginRejectsMissingClaimsOrInvalidChain(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*commandexecution.Invocation, *commandjobactivation.ProjectionSnapshot)
	}{
		{name: "sem claims", mutate: func(_ *commandexecution.Invocation, proof *commandjobactivation.ProjectionSnapshot) {
			proof.Claims = nil
		}},
		{name: "sem chain", mutate: func(in *commandexecution.Invocation, _ *commandjobactivation.ProjectionSnapshot) {
			var document map[string]json.RawMessage
			if err := json.Unmarshal(*in.Envelope.Provenance, &document); err != nil {
				panic(err)
			}
			delete(document, "command_chain_history")
			rawBytes, _ := json.Marshal(document)
			raw := json.RawMessage(rawBytes)
			in.Envelope.Provenance = &raw
		}},
		{name: "chain inválida", mutate: func(in *commandexecution.Invocation, _ *commandjobactivation.ProjectionSnapshot) {
			var document map[string]json.RawMessage
			if err := json.Unmarshal(*in.Envelope.Provenance, &document); err != nil {
				panic(err)
			}
			document["command_chain_history"] = json.RawMessage(`[{"command_id":"invalid","invocation_id":"bad","layer_refs":[]}]`)
			rawBytes, _ := json.Marshal(document)
			raw := json.RawMessage(rawBytes)
			in.Envelope.Provenance = &raw
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, proof := commandJobOriginFixture(t, []string{"layer.selected"}, "manual", "root-1", nil)
			tt.mutate(&in, &proof)
			if _, err := commandJobOriginFromProjection(in, proof); err == nil {
				t.Fatal("entrada incompleta foi aceita")
			}
		})
	}
}

func commandJobOriginFixture(t *testing.T, layerRefs []string, rootType, rootID string, prefix []commandcontract.CommandChainEntry) (commandexecution.Invocation, commandjobactivation.ProjectionSnapshot) {
	t.Helper()
	if prefix == nil {
		prefix = []commandcontract.CommandChainEntry{}
	}
	invocationID := uuid.Must(uuid.NewV7()).String()
	commandID := "fixture.command"
	entry := commandcontract.CommandChainEntry{CommandID: commandID, InvocationID: invocationID, LayerRefs: append([]string(nil), layerRefs...)}
	history := append(append([]commandcontract.CommandChainEntry(nil), prefix...), entry)
	provenance := map[string]any{
		"version":               1,
		"_chain_id":             "chain-1",
		"_chain_history":        []string{"job-root"},
		"command_chain_history": history,
	}
	rawBytes, err := json.Marshal(provenance)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(rawBytes)
	claimProvenance := map[string]any{
		"version":               1,
		"_chain_id":             "chain-1",
		"_chain_history":        []string{"job-root"},
		"command_chain_history": prefix,
	}
	claimRaw, err := json.Marshal(claimProvenance)
	if err != nil {
		t.Fatal(err)
	}
	proof := commandjobactivation.ProjectionSnapshot{Claims: []commandjobactivation.ProjectionClaim{{
		Claim:          commandactivation.Claim{ActivationID: "activation-a", LayerRefKind: commandactivation.UserRef, LayerRef: layerRefs[0]},
		RootOriginType: rootType,
		RootOriginID:   rootID,
		Provenance:     claimRaw,
	}}}
	return commandexecution.Invocation{
		ID: invocationID, CommandID: commandID,
		Envelope: &commandcontract.Envelope{Provenance: &raw},
	}, proof
}
