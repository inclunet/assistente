package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

func TestSealCoreDrainBindsExactGenerationAndSupportsSystem(t *testing.T) {
	now := time.Now().UTC()
	store, db := testStore(t, &now)
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	req := validRequest()
	ctx := context.Background()
	epoch, err := core.Capture(ctx, req.Owner.UserID, req.Owner.AuthContextID)
	if err != nil {
		t.Fatal(err)
	}
	req.SecurityGeneration = epoch.SecurityGeneration
	req.ExpiresAt = now.Add(time.Hour)
	if _, err := store.Reserve(ctx, req); err != nil {
		t.Fatal(err)
	}
	id := uuid.Must(uuid.NewV7()).String()
	systemID := "test-system-instance"
	if err := db.Create(&ledgerRow{ID: uuid.Must(uuid.NewV7()).String(), Key: "invocation:" + id, InvocationID: id, AuthContextType: "system", AuthContextID: systemID, RequestFingerprintVersion: "v1", RequestFingerprint: "system-fp", Status: Running, ReceivedAt: now, ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&invocationRow{InvocationID: id, SchemaVersion: 1, AuthContextType: "system", AuthContextID: systemID, AuthGeneration: "system-auth", SecurityGeneration: epoch.SecurityGeneration, RegistryVersion: "r1", BindingIDs: "[]", ActorType: "system", ActorID: "instance", ArgumentsSummary: "{}", ArgumentsFingerprint: "args", CorrelationID: "corr", RequestFingerprintVersion: "v1", RequestFingerprint: "system-fp", Risk: "low", PolicyDecision: "allowed", Status: Running, ReceivedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	scope := GenerationScope{UserID: &req.Owner.UserID, AuthContextType: "local_session", AuthContextID: req.Owner.AuthContextID, SecurityGeneration: epoch.SecurityGeneration}
	if _, err := store.SealDrainedGeneration(ctx, commandsecurity.DrainedGenerations{}, scope); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("zero proof=%v", err)
	}
	foreign, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	foreignProof, err := foreign.CloseAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SealDrainedGeneration(ctx, foreignProof, scope); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("foreign proof=%v", err)
	}
	proof, err := core.CloseAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []GenerationScope{scope, {AuthContextType: "system", AuthContextID: systemID, SecurityGeneration: epoch.SecurityGeneration}} {
		sealed, err := store.SealDrainedGeneration(ctx, proof, candidate)
		if err != nil {
			t.Fatal(err)
		}
		again, err := store.SealDrainedGeneration(ctx, proof, candidate)
		if err != nil || again.marker != sealed.marker {
			t.Fatalf("selo duplicado=%v", err)
		}
		batch, err := store.RecoverClosedGenerationWithProof(ctx, sealed, 1)
		if err != nil || batch.Processed != 1 || batch.More {
			t.Fatalf("recover=%+v %v", batch, err)
		}
	}
	var markers int64
	if err := db.Model(&closedGenerationRow{}).Count(&markers).Error; err != nil {
		t.Fatal(err)
	}
	if markers != 2 {
		t.Fatalf("marcadores=%d", markers)
	}
}
