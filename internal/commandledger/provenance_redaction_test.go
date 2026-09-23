package commandledger

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestReserveEnvelopeRedactsAndPreservesValidatedCommandChainHistory(t *testing.T) {
	req, now := envelopeRequest(t)
	previousInvocation := uuid.Must(uuid.NewV7()).String()
	currentInvocation := uuid.Must(uuid.NewV7()).String()
	req.Envelope.Provenance = rawPtr(`{"version":1,"redacted":false,"_source":"job","_source_job_id":"job-1","_chain_id":"chain-1","_chain_history":["job-root"],"command_chain_history":[{"command_id":"workspace.read","invocation_id":"` + previousInvocation + `","layer_refs":["layer.a"]},{"command_id":"workspace.write","invocation_id":"` + currentInvocation + `","layer_refs":[]}],"runtime_identity":{"user_id":"secret-user"},"claims":["secret-claim"],"password":"secret-payload"}`)
	store, db := testStore(t, &now)
	if _, err := store.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var audit invocationRow
	if err := db.First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.Provenance == nil {
		t.Fatal("proveniência validada não foi persistida")
	}
	got := *audit.Provenance
	for _, secret := range []string{"secret-user", "secret-claim", "secret-payload", "runtime_identity", "claims", "password"} {
		if strings.Contains(got, secret) {
			t.Fatalf("proveniência vazou %q: %s", secret, got)
		}
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(got), &document); err != nil {
		t.Fatal(err)
	}
	var redacted bool
	if err := json.Unmarshal(document["redacted"], &redacted); err != nil || !redacted {
		t.Fatalf("redacted deve ser sempre true: %s", got)
	}
	var version int
	if err := json.Unmarshal(document["version"], &version); err != nil || version != 1 {
		t.Fatalf("version deve ser sempre 1: %s", got)
	}
	for _, field := range []string{"version", "redacted", "_source", "_source_job_id", "_chain_id", "_chain_history", "command_chain_history"} {
		if _, ok := document[field]; !ok {
			t.Fatalf("campo estrutural ausente %q: %s", field, got)
		}
	}
	if _, ok := document["password"]; ok {
		t.Fatal("campo arbitrário persistido")
	}
	var history []map[string]json.RawMessage
	if err := json.Unmarshal(document["command_chain_history"], &history); err != nil || len(history) != 2 {
		t.Fatalf("histórico de comandos: %s, %v", document["command_chain_history"], err)
	}
	for _, entry := range history {
		if len(entry) != 3 {
			t.Fatalf("campos extras no item do histórico: %#v", entry)
		}
	}
}

func TestReserveEnvelopeInvalidCommandChainHistoryUsesRedactedMarker(t *testing.T) {
	req, now := envelopeRequest(t)
	invocation := uuid.Must(uuid.NewV7()).String()
	req.Envelope.Provenance = rawPtr(`{"version":1,"_source":"job","_chain_id":"chain-1","_chain_history":[],"command_chain_history":[{"command_id":"workspace.read","invocation_id":"` + invocation + `","layer_refs":["layer.a"],"secret":"must-not-persist"}]}`)
	store, db := testStore(t, &now)
	if _, err := store.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatal("redação inválida não deve impedir a reserva: ", err)
	}
	var audit invocationRow
	if err := db.First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.Provenance == nil || *audit.Provenance != redactedDocument {
		t.Fatalf("histórico inválido não foi redigido fechado: %v", audit.Provenance)
	}
}

func TestRedactedProvenanceRequiresVersionOne(t *testing.T) {
	for _, raw := range []string{`{"redacted":false}`, `{"version":2,"redacted":false}`} {
		value := json.RawMessage(raw)
		got := redactedProvenanceIfPresent(&value)
		if got == nil || *got != redactedDocument {
			t.Fatalf("proveniência sem version 1 não caiu no marcador: %s", raw)
		}
	}
}

func TestRedactedProvenanceAllowsEmptySourceJobIDForUserOrigin(t *testing.T) {
	req, now := envelopeRequest(t)
	req.Envelope.Provenance = rawPtr(`{"version":1,"_source":"user","_source_job_id":"","_chain_id":"chain-1","_chain_history":[]}`)
	store, db := testStore(t, &now)
	if _, err := store.ReserveEnvelope(context.Background(), req); err != nil {
		t.Fatal("origem user com source_job_id vazio não deve ser redigida como inválida: ", err)
	}
	var audit invocationRow
	if err := db.First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.Provenance == nil || !strings.Contains(*audit.Provenance, `"_source_job_id":""`) {
		t.Fatalf("source_job_id vazio não foi preservado: %v", audit.Provenance)
	}
}

func TestRedactedProvenanceRejectsNullStringFields(t *testing.T) {
	for _, field := range []string{"_source", "_source_job_id", "_chain_id"} {
		t.Run(field, func(t *testing.T) {
			value := json.RawMessage(`{"version":1,"` + field + `":null}`)
			if got := redactedProvenanceIfPresent(&value); got == nil || *got != redactedDocument {
				t.Fatalf("null em %s não foi rejeitado: %v", field, got)
			}
		})
	}
}
