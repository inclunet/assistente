package commandjobactivation

import (
	"context"
	"testing"

	"assistente/internal/commandactivation"
	"assistente/internal/commandjobevents"
)

func TestProjectionRootOriginUsesVerifiedFact(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}

	snapshot, err := c.Projection(context.Background(), projectionOwner(fact))
	if err != nil || len(snapshot.Claims) != 1 {
		t.Fatalf("projeção=%+v err=%v", snapshot, err)
	}
	claim := snapshot.Claims[0]
	if claim.RootOriginType != fact.RootOriginType || claim.RootOriginID != fact.RootOriginID {
		t.Fatalf("origem raiz projetada=%q/%q, want=%q/%q", claim.RootOriginType, claim.RootOriginID, fact.RootOriginType, fact.RootOriginID)
	}
}

func TestProjectionRootOriginDoesNotReadClaimAliases(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	var claim commandactivation.Claim
	if err := c.db.Where("source_event_id = ?", fact.SourceEventID).Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if err := c.db.Model(&commandactivation.Claim{}).Where("activation_id = ?", claim.ActivationID).
		Update("provenance", `{"root_origin_type":"tampered","root_origin_id":"tampered"}`).Error; err != nil {
		t.Fatal(err)
	}

	snapshot, err := c.Projection(context.Background(), projectionOwner(fact))
	if err != nil || len(snapshot.Claims) != 1 {
		t.Fatalf("projeção=%+v err=%v", snapshot, err)
	}
	projected := snapshot.Claims[0]
	if projected.RootOriginType != fact.RootOriginType || projected.RootOriginID != fact.RootOriginID {
		t.Fatalf("alias da claim virou autoridade: %q/%q", projected.RootOriginType, projected.RootOriginID)
	}
}

func TestProjectionRootOriginRejectsTamperedOutbox(t *testing.T) {
	c, out, fact, _, _ := fixture(t)
	if got := deliver(t, c, out, fact); got.Applied != 1 {
		t.Fatalf("evento inicial não aplicado: %+v", got)
	}
	if err := c.db.Model(&commandjobevents.ActivationOutbox{}).Where("source_event_id = ?", fact.SourceEventID).
		Update("root_origin_id", "tampered-root").Error; err != nil {
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
