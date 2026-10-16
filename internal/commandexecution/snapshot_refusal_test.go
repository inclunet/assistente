package commandexecution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandcontract"
	"assistente/internal/commandledger"
	"assistente/internal/database"
)

func TestEnvelopeSnapshotFailurePersistsDeniedAndNeverReexecutesAfterRecovery(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	request := f.candidate(newTestUUID(), "pipe.read", `{}`)
	var failedSnapshots atomic.Int32
	f.service.config.Envelope.Snapshot = func(context.Context, auth.LocalSessionPrincipal, EnvelopeCandidate) (commandcontract.Envelope, error) {
		failedSnapshots.Add(1)
		return commandcontract.Envelope{
			GlobalConfigGeneration: pipelineStringPtr("global-v1"),
			ActiveLayersGeneration: pipelineStringPtr("layers-v1"),
		}, errors.New("snapshot indisponível")
	}

	first, err := f.service.ExecuteEnvelope(context.Background(), f.token, request)
	if err != nil {
		t.Fatalf("falha de snapshot deveria ser recusada no ledger: %v", err)
	}
	if first.Status != commandledger.Denied || first.Mode != commandledger.ModeDenied {
		t.Fatalf("recusa persistida = status %q modo %q", first.Status, first.Mode)
	}
	if failedSnapshots.Load() != 1 || f.startCalls.Load() != 0 || f.resolveCalls.Load() != 0 {
		t.Fatalf("pipeline avançou após snapshot: snapshots=%d starts=%d resolves=%d", failedSnapshots.Load(), f.startCalls.Load(), f.resolveCalls.Load())
	}

	owner := commandledger.FullOwnership{
		UserID:         &f.user,
		AuthContextType: commandcontract.AuthLocalSession,
		AuthContextID:   f.session,
		ActorType:       commandcontract.ActorUser,
		ActorID:         f.user,
	}
	persisted, err := f.store.GetEnvelopeByID(context.Background(), owner, request.InvocationID)
	if err != nil {
		t.Fatalf("ler recusa no SQLite: %v", err)
	}
	if persisted.Status != commandledger.Denied || persisted.RequestFingerprint == "" || persisted.InputFingerprint == "" {
		t.Fatalf("registro terminal incompleto: status=%q request_fp=%q input_fp=%q", persisted.Status, persisted.RequestFingerprint, persisted.InputFingerprint)
	}
	if persisted.Envelope.WorkspaceID != nil || persisted.Envelope.GlobalConfigGeneration == nil || *persisted.Envelope.GlobalConfigGeneration != "global-v1" || persisted.Envelope.ActiveLayersGeneration == nil || *persisted.Envelope.ActiveLayersGeneration != "layers-v1" || persisted.Envelope.SourceType == nil || *persisted.Envelope.SourceType != commandcontract.SourcePalette {
		t.Fatalf("recusa inventou/perdeu contexto autoritativo: workspace=%v global=%v layers=%v source=%v", persisted.Envelope.WorkspaceID, persisted.Envelope.GlobalConfigGeneration, persisted.Envelope.ActiveLayersGeneration, persisted.Envelope.SourceType)
	}

	// A fonte se recupera, mas o mesmo invocation ID deve ser somente leitura
	// do tombstone; não há nova captura, resolução ou handler.
	f.service.config.Envelope.Snapshot = f.snapshot
	replayed, err := f.service.ExecuteEnvelope(context.Background(), f.token, request)
	if err != nil || replayed.ID != first.ID || replayed.Status != commandledger.Denied {
		t.Fatalf("reentrega após recuperação = record=%+v err=%v", replayed, err)
	}
	if failedSnapshots.Load() != 1 || f.startCalls.Load() != 0 || f.resolveCalls.Load() != 0 {
		t.Fatalf("reentrega reabriu pipeline: snapshots=%d starts=%d resolves=%d", failedSnapshots.Load(), f.startCalls.Load(), f.resolveCalls.Load())
	}

	changed := request
	changed.Arguments = []byte(`{"different":true}`)
	if _, err := f.service.ExecuteEnvelope(context.Background(), f.token, changed); !errors.Is(err, commandledger.ErrConflict) {
		t.Fatalf("argumentos alterados reutilizaram recusa = %v, want %v", err, commandledger.ErrConflict)
	}

	var user database.User
	if err := f.db.Where("id = ?", f.user).First(&user).Error; err != nil {
		t.Fatalf("ler usuário para segunda sessão: %v", err)
	}
	secondPair, err := f.sessions.IssueSession(context.Background(), &user, "snapshot-cross-session")
	if err != nil {
		t.Fatalf("emitir segunda sessão: %v", err)
	}
	if _, err := f.service.ExecuteEnvelope(context.Background(), secondPair.AccessToken, request); !errors.Is(err, commandledger.ErrConflict) {
		t.Fatalf("reentrega cross-session = %v, want %v", err, commandledger.ErrConflict)
	}
	if f.startCalls.Load() != 0 {
		t.Fatalf("cross-session iniciou handler: %d", f.startCalls.Load())
	}
}

func TestEnvelopeSnapshotFailureWithoutGenerationsDoesNotReserve(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	request := f.candidate(newTestUUID(), "pipe.read", `{}`)
	f.service.config.Envelope.Snapshot = func(context.Context, auth.LocalSessionPrincipal, EnvelopeCandidate) (commandcontract.Envelope, error) {
		return commandcontract.Envelope{}, errors.New("snapshot sem gerações")
	}

	if _, err := f.service.ExecuteEnvelope(context.Background(), f.token, request); !errors.Is(err, ErrStale) {
		t.Fatalf("snapshot sem gerações = %v, want %v", err, ErrStale)
	}
	var ledgers, invocations int64
	if err := f.db.Table("command_idempotency_keys").Count(&ledgers).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Table("command_invocations").Count(&invocations).Error; err != nil {
		t.Fatal(err)
	}
	if ledgers != 0 || invocations != 0 || f.startCalls.Load() != 0 {
		t.Fatalf("falha sem gerações deixou estado: ledgers=%d invocations=%d starts=%d", ledgers, invocations, f.startCalls.Load())
	}
}

func TestEnvelopeSnapshotCancellationDoesNotReserve(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	request := f.candidate(newTestUUID(), "pipe.read", `{}`)
	f.service.config.Envelope.Snapshot = func(context.Context, auth.LocalSessionPrincipal, EnvelopeCandidate) (commandcontract.Envelope, error) {
		return commandcontract.Envelope{}, context.Canceled
	}

	if _, err := f.service.ExecuteEnvelope(context.Background(), f.token, request); !errors.Is(err, ErrStale) {
		t.Fatalf("snapshot cancelado = %v, want %v", err, ErrStale)
	}
	var ledgers int64
	if err := f.db.Table("command_idempotency_keys").Count(&ledgers).Error; err != nil {
		t.Fatal(err)
	}
	if ledgers != 0 || f.startCalls.Load() != 0 {
		t.Fatalf("snapshot cancelado deixou ledger/start: ledgers=%d starts=%d", ledgers, f.startCalls.Load())
	}
}
