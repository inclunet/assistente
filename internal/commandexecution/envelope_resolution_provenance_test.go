package commandexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandcontract"
	"assistente/internal/commandledger"
)

func resolutionProvenance(t *testing.T, sourceCommand string) *json.RawMessage {
	t.Helper()
	invocationID := newTestUUID()
	raw := json.RawMessage(`{"version":1,"_source":"job","_chain_id":"chain-source","_chain_history":[],"command_chain_history":[{"command_id":"` + sourceCommand + `","invocation_id":"` + invocationID + `","layer_refs":["layer.source"]}]}`)
	return &raw
}

func provenanceChain(t *testing.T, raw *json.RawMessage) []map[string]any {
	t.Helper()
	var document struct {
		History []map[string]any `json:"command_chain_history"`
	}
	if err := json.Unmarshal(*raw, &document); err != nil {
		t.Fatal(err)
	}
	return document.History
}

func resolutionChainProvenance(t *testing.T, length int, repeated bool) *json.RawMessage {
	t.Helper()
	history := make([]map[string]any, 0, length)
	for i := 0; i < length; i++ {
		commandID := "pipe.source" + string(rune('a'+i))
		if repeated {
			commandID = "pipe.source.a"
		}
		history = append(history, map[string]any{
			"command_id": commandID, "invocation_id": newTestUUID(), "layer_refs": []string{"layer.source"},
		})
	}
	document := map[string]any{
		"version": 1, "_source": "job", "_chain_id": "chain-source", "_chain_history": []string{},
		"command_chain_history": history,
	}
	canonical, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(canonical)
	return &raw
}

func resolutionCurrentCommandProvenance(t *testing.T) *json.RawMessage {
	t.Helper()
	document := map[string]any{
		"version": 1, "_source": "job", "_chain_id": "chain-source", "_chain_history": []string{},
		"command_chain_history": []map[string]any{{
			"command_id": "pipe.write", "invocation_id": newTestUUID(), "layer_refs": []string{"layer.source"},
		}},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(encoded)
	return &raw
}

func TestEnvelopeResolutionProvenanceAppendsSourceChainBeforeExecution(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	var received *json.RawMessage
	f.start = func(_ context.Context, invocation Invocation) (ExecutionHandle, error) {
		received = cloneRawMessage(invocation.Envelope.Provenance)
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	}
	trustedProvenance := resolutionProvenance(t, "pipe.read")
	f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
		return EnvelopeResolution{
			Mode: commandcontract.ResolutionExecute, CommandID: "pipe.write", Arguments: json.RawMessage(`{}`),
			BindingIDs: []string{"binding.pipeline"}, LayerRefs: []string{"layer.resolved"},
			Provenance: trustedProvenance,
		}, nil
	}
	candidate := f.trigger(newTestUUID())
	candidate.TriggerSpec = json.RawMessage(`{"version":1,"selection":"pipe.write","provenance":{"_source":"forged"}}`)
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, candidate)
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("execução com provenance: status=%s err=%v", record.Status, err)
	}
	if received == nil {
		t.Fatal("handler não recebeu proveniência")
	}
	history := provenanceChain(t, received)
	if len(history) != 2 || history[0]["command_id"] != "pipe.read" || history[1]["command_id"] != "pipe.write" {
		t.Fatalf("cadeia efetiva=%v", history)
	}
}

func TestEnvelopeResolutionProvenanceChangeBetweenPrepareAndAdmitIsStale(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	first := resolutionProvenance(t, "pipe.read")
	second := resolutionProvenance(t, "pipe.destroy")
	f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
		call := f.resolveCalls.Add(1)
		provenance := first
		if call > 1 {
			provenance = second
		}
		return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: "pipe.write", Arguments: json.RawMessage(`{}`), LayerRefs: []string{"layer.resolved"}, Provenance: provenance}, nil
	}
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.trigger(newTestUUID()))
	if err != nil || record.Status != commandledger.CancelledStale || f.startCalls.Load() != 0 {
		t.Fatalf("proveniência alterada não ficou stale: status=%s err=%v starts=%d", record.Status, err, f.startCalls.Load())
	}
}

func TestEnvelopeResolutionNilProvenancePreservesHostProvenance(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	host := resolutionProvenance(t, "pipe.read")
	f.service.config.Envelope.Snapshot = func(_ context.Context, _ auth.LocalSessionPrincipal, c EnvelopeCandidate) (commandcontract.Envelope, error) {
		return commandcontract.Envelope{RegistryVersion: "registry-v1", GlobalConfigGeneration: pipelineStringPtr("global-v1"), ActiveLayersGeneration: pipelineStringPtr("layers-v1"), CorrelationID: c.CorrelationID, Provenance: host}, nil
	}
	var received *json.RawMessage
	f.start = func(_ context.Context, invocation Invocation) (ExecutionHandle, error) {
		received = cloneRawMessage(invocation.Envelope.Provenance)
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	}
	f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
		return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: "pipe.write", Arguments: json.RawMessage(`{}`), LayerRefs: []string{"layer.resolved"}}, nil
	}
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.trigger(newTestUUID()))
	if err != nil || record.Status != commandledger.Succeeded || received == nil || !bytes.Contains(*received, []byte(`"pipe.read"`)) {
		t.Fatalf("proveniência do host foi apagada: status=%s err=%v received=%s", record.Status, err, received)
	}
}

func TestEnvelopeResolutionDivergentHostAndResolutionProvenanceIsDenied(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	host := resolutionProvenance(t, "pipe.read")
	resolved := resolutionProvenance(t, "pipe.destroy")
	f.service.config.Envelope.Snapshot = func(_ context.Context, _ auth.LocalSessionPrincipal, c EnvelopeCandidate) (commandcontract.Envelope, error) {
		return commandcontract.Envelope{RegistryVersion: "registry-v1", GlobalConfigGeneration: pipelineStringPtr("global-v1"), ActiveLayersGeneration: pipelineStringPtr("layers-v1"), CorrelationID: c.CorrelationID, Provenance: host}, nil
	}
	f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
		return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: "pipe.write", Arguments: json.RawMessage(`{}`), LayerRefs: []string{"layer.resolved"}, Provenance: resolved}, nil
	}
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.trigger(newTestUUID()))
	if err != nil || record.Status != commandledger.Denied || f.startCalls.Load() != 0 {
		t.Fatalf("divergência não foi negada: status=%s err=%v starts=%d", record.Status, err, f.startCalls.Load())
	}
}

func TestEnvelopeResolutionProvenanceChainBoundaryAndDuplicateAreRejected(t *testing.T) {
	t.Run("15 para 16 aceita", func(t *testing.T) {
		f := newEnvelopePipelineFixture(t)
		trusted := resolutionChainProvenance(t, 15, false)
		candidate := f.trigger(newTestUUID())
		var received *json.RawMessage
		f.start = func(_ context.Context, invocation Invocation) (ExecutionHandle, error) {
			received = cloneRawMessage(invocation.Envelope.Provenance)
			return pipelineCompletedHandle(commandledger.Succeeded), nil
		}
		f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
			return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: "pipe.write", Arguments: json.RawMessage(`{}`), LayerRefs: []string{"layer.resolved"}, Provenance: trusted}, nil
		}
		record, err := f.service.ExecuteEnvelope(context.Background(), f.token, candidate)
		if err != nil || record.Status != commandledger.Succeeded || f.startCalls.Load() != 1 {
			t.Fatalf("limite 15->16: status=%s err=%v starts=%d", record.Status, err, f.startCalls.Load())
		}
		var persisted struct{ Provenance *string }
		if err := f.db.Table("command_invocations").Select("provenance").Where("invocation_id = ?", candidate.InvocationID).Scan(&persisted).Error; err != nil {
			t.Fatal(err)
		}
		var ledgerProvenance *json.RawMessage
		if persisted.Provenance != nil {
			raw := json.RawMessage(*persisted.Provenance)
			ledgerProvenance = &raw
		}
		for name, raw := range map[string]*json.RawMessage{"handler": received, "ledger": ledgerProvenance} {
			if raw == nil {
				t.Fatalf("%s sem proveniência", name)
			}
			history := provenanceChain(t, raw)
			if len(history) != 16 {
				t.Fatalf("%s history len=%d, want 16", name, len(history))
			}
			last := history[len(history)-1]
			if last["command_id"] != "pipe.write" || last["invocation_id"] != candidate.InvocationID {
				t.Fatalf("%s último item=%v", name, last)
			}
			layers, ok := last["layer_refs"].([]any)
			if !ok || len(layers) != 1 || layers[0] != "layer.resolved" {
				t.Fatalf("%s layer_refs=%v", name, last["layer_refs"])
			}
		}
	})

	for _, tc := range []struct {
		name     string
		length   int
		repeated bool
	}{
		{"16 para 17", 16, false},
		{"command_id repetido", 2, true},
		{"command_id atual repetido", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEnvelopePipelineFixture(t)
			trusted := resolutionChainProvenance(t, tc.length, tc.repeated)
			if tc.name == "command_id atual repetido" {
				trusted = resolutionCurrentCommandProvenance(t)
			}
			candidate := f.trigger(newTestUUID())
			f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
				return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: "pipe.write", Arguments: json.RawMessage(`{}`), LayerRefs: []string{"layer.resolved"}, Provenance: trusted}, nil
			}
			record, err := f.service.ExecuteEnvelope(context.Background(), f.token, candidate)
			var invocations int64
			if countErr := f.db.Table("command_invocations").Where("invocation_id = ?", candidate.InvocationID).Count(&invocations).Error; countErr != nil {
				t.Fatal(countErr)
			}
			if !errors.Is(err, ErrDenied) || record.InvocationID != "" || invocations != 0 || f.startCalls.Load() != 0 {
				t.Fatalf("cadeia inválida alcançou reserva/execução: record=%+v err=%v invocations=%d starts=%d", record, err, invocations, f.startCalls.Load())
			}
		})
	}
}
