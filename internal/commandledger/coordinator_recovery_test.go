package commandledger

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandmaintenance"
	"assistente/internal/commandsecurity"
	"github.com/google/uuid"
)

func TestCoordinatorRecoveryPagesAllOwnersAndPreservesLiveCore(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, db := testStore(t, &now)
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	var requests []LocalReadRequest
	for i := 0; i < 5; i++ {
		req := validRequest()
		epoch, err := core.Capture(ctx, req.Owner.UserID, req.Owner.AuthContextID)
		if err != nil {
			t.Fatal(err)
		}
		req.SecurityGeneration = epoch.SecurityGeneration
		req.ExpiresAt = now.Add(time.Hour)
		if _, err := store.Reserve(ctx, req); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, req)
	}
	live := validRequest()
	live.ExpiresAt = now.Add(time.Hour)
	if _, err := store.Reserve(ctx, live); err != nil {
		t.Fatal(err)
	}
	id := uuid.Must(uuid.NewV7()).String()
	if err := db.Create(&ledgerRow{ID: uuid.Must(uuid.NewV7()).String(), Key: "invocation:" + id, InvocationID: id, AuthContextType: "system", AuthContextID: "instance", RequestFingerprintVersion: "v1", RequestFingerprint: "fp", Status: Running, ReceivedAt: now, ExpiresAt: now.Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&invocationRow{InvocationID: id, SchemaVersion: 1, AuthContextType: "system", AuthContextID: "instance", AuthGeneration: "auth", SecurityGeneration: requests[0].SecurityGeneration, RegistryVersion: "r1", BindingIDs: "[]", ActorType: "system", ActorID: "instance", ArgumentsSummary: "{}", ArgumentsFingerprint: "args", CorrelationID: "corr", RequestFingerprintVersion: "v1", RequestFingerprint: "fp", Risk: "low", PolicyDecision: "allowed", Status: Running, ReceivedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := NewCoordinatorRecovery(store, commandsecurity.DrainedGenerations{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("prova zero: %v", err)
	}
	proof, err := core.CloseAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewCoordinatorRecovery(store, proof)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for page := 0; page < 5; page++ {
		result, err := adapter.Recover(ctx, 2)
		if err != nil {
			t.Fatal(err)
		}
		if result.Processed > 2 {
			t.Fatal("lote excedido")
		}
		total += result.Processed
		if !result.More {
			break
		}
		if page == 4 {
			t.Fatal("não encerrou")
		}
	}
	if total != 6 {
		t.Fatalf("recuperados=%d", total)
	}
	var liveRow invocationRow
	if err := db.Where("invocation_id = ?", live.InvocationID).First(&liveRow).Error; err != nil {
		t.Fatal(err)
	}
	if liveRow.Status != Evaluating {
		t.Fatalf("core não comprovado alterado: %s", liveRow.Status)
	}
	if adapter.after != "" {
		t.Fatal("ciclo não reiniciou cursor")
	}
	result, err := adapter.Recover(ctx, 2)
	if err != nil || result.Processed != 0 || result.More {
		t.Fatalf("repetição=%+v %v", result, err)
	}
	adapter.mu.Lock()
	_, err = adapter.Recover(ctx, 2)
	adapter.mu.Unlock()
	if !errors.Is(err, commandmaintenance.ErrAlreadyRunning) {
		t.Fatalf("concorrência=%v", err)
	}
}

func TestCoordinatorRecoveryErrorKeepsCommittedProgressAndRetries(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	store, db := testStore(t, &now)
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := 0; i < 2; i++ {
		req := validRequest()
		epoch, err := core.Capture(ctx, req.Owner.UserID, req.Owner.AuthContextID)
		if err != nil {
			t.Fatal(err)
		}
		req.SecurityGeneration = epoch.SecurityGeneration
		req.ExpiresAt = now.Add(time.Hour)
		if _, err := store.Reserve(ctx, req); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, req.InvocationID)
	}
	if err := db.Model(&ledgerRow{}).Where("invocation_id = ?", ids[1]).Update("request_fingerprint", "divergent").Error; err != nil {
		t.Fatal(err)
	}
	proof, err := core.CloseAndDrain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewCoordinatorRecovery(store, proof)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Recover(ctx, 2)
	if !errors.Is(err, ErrInconsistent) || result.Processed != 1 || !result.More {
		t.Fatalf("parcial=%+v %v", result, err)
	}
	if adapter.after != ids[0] {
		t.Fatalf("cursor adiantou falha: %s", adapter.after)
	}
	if err := db.Model(&ledgerRow{}).Where("invocation_id = ?", ids[1]).Update("request_fingerprint", "request").Error; err != nil {
		t.Fatal(err)
	}
	result, err = adapter.Recover(ctx, 2)
	if err != nil || result.Processed != 1 || result.More {
		t.Fatalf("retry=%+v %v", result, err)
	}
}
