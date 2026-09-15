package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
)

func TestConflictDiagnosticRevalidateUsaSnapshotPrivadoEEpochProvider(t *testing.T) {
	f := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	if err := f.projection.db.Where("id = ?", f.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	version := "catalog-v1"
	principal := auth.LocalSessionPrincipal{UserID: f.epoch.UserID, SessionID: f.epoch.SessionID}
	service, err := NewCompleteMutationService(MutationServiceConfig{
		Store: f.projection.store, Sessions: mutationSessionFixture{principal}, Epochs: epochs,
		Receipts: f.receipts, Keys: func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{7}, 32), nil },
		KeyVersion: "v1", DecisionTTL: time.Minute,
		Authorize:    func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error { return nil },
		Version:      func(context.Context) (string, error) { return version, nil },
		Render:       func(MutationDiff) (string, error) { return "diagnostic", nil },
		OnMutationTx: completeNoopHook,
	}, func(context.Context, Scope) (CompleteProjection, error) { return options, nil })
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := service.CheckConflicts(context.Background(), "token", nil)
	if err != nil {
		t.Fatalf("CheckConflicts: %v", err)
	}
	foreignWorkspace := "workspace-do-not-use"
	diagnostic.Scope.WorkspaceID = &foreignWorkspace
	if err := service.RevalidateConflicts(context.Background(), "token", diagnostic); err != nil {
		t.Fatalf("revalidação usou Scope público: %v", err)
	}
	version = "catalog-v2"
	if err := service.RevalidateConflicts(context.Background(), "token", diagnostic); !errors.Is(err, ErrStale) {
		t.Fatalf("versão do provider não invalidou diagnóstico: %v", err)
	}
}
