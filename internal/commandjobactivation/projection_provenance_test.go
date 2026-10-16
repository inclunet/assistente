package commandjobactivation

import (
	"bytes"
	"context"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/commandjson"
)

func TestProjectionProvenanceUsesVerifiedFactNotClaim(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	var claim commandactivation.Claim
	if err := c.db.Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Model(&commandactivation.Claim{}).Where("activation_id = ?", claim.ActivationID).Update("provenance", `{"tampered":true}`).Error; err != nil {
		t.Fatal(err)
	}

	snapshot, err := c.Projection(context.Background(), projectionOwner(fact))
	if err != nil || len(snapshot.Claims) != 1 {
		t.Fatalf("projeção=%+v err=%v", snapshot, err)
	}
	expected, err := commandjson.Marshal(fact.Provenance)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot.Claims[0].Provenance, expected) {
		t.Fatalf("proveniência projetada=%s, want fato=%s", snapshot.Claims[0].Provenance, expected)
	}
}

func TestProjectionProvenanceFactFingerprintRejectsTamperedOutbox(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).Update("provenance", `{"_source":"job","_source_job_id":"tampered"}`).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := c.Projection(context.Background(), projectionOwner(fact))
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Claims) != 0 {
		t.Fatalf("outbox adulterada prevaleceu na projeção: %+v", snapshot.Claims)
	}
}

func TestProjectionProvenanceCanonicalAndDetached(t *testing.T) {
	first, err := projectionProvenance(map[string]any{"z": 1, "a": map[string]any{"b": 2, "a": 1}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := projectionProvenance(map[string]any{"a": map[string]any{"a": 1, "b": 2}, "z": 1})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("proveniência não canônica: %s != %s", first, second)
	}
	missing, err := projectionProvenance(nil)
	if err != nil || missing != nil {
		t.Fatalf("proveniência ausente=%s err=%v, want nil", missing, err)
	}
	if _, err := projectionProvenance(map[string]any{"unsupported": func() {}}); err == nil {
		t.Fatal("marshal canônico aceitou valor não serializável")
	}

	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	snapshot, err := c.Projection(context.Background(), projectionOwner(fact))
	if err != nil || len(snapshot.Claims) != 1 {
		t.Fatalf("projeção=%+v err=%v", snapshot, err)
	}
	original := append([]byte(nil), snapshot.Claims[0].Provenance...)
	if len(snapshot.Claims[0].Provenance) == 0 {
		t.Fatal("proveniência projetada vazia")
	}
	snapshot.Claims[0].Provenance[0] = 'X'
	secondSnapshot, err := c.Projection(context.Background(), projectionOwner(fact))
	if err != nil || len(secondSnapshot.Claims) != 1 {
		t.Fatalf("segunda projeção=%+v err=%v", secondSnapshot, err)
	}
	if !bytes.Equal(secondSnapshot.Claims[0].Provenance, original) {
		t.Fatalf("proveniência retornada compartilha memória: %s", secondSnapshot.Claims[0].Provenance)
	}
}
