package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
)

func newDynamicConflictDiagnosticService(t *testing.T, fixture decisionTestFixture, active func() []string) *CompleteMutationService {
	t.Helper()
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	principal := auth.LocalSessionPrincipal{UserID: fixture.epoch.UserID, SessionID: fixture.epoch.SessionID}
	service, err := NewCompleteMutationService(MutationServiceConfig{
		Store: fixture.projection.store, Sessions: mutationSessionFixture{principal}, Epochs: epochs,
		Receipts: fixture.receipts, Keys: func(context.Context, string) ([]byte, error) {
			return bytes.Repeat([]byte{7}, 32), nil
		},
		KeyVersion: "v1", DecisionTTL: time.Minute,
		Authorize: func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error {
			return nil
		},
		Version:      func(context.Context) (string, error) { return "catalog-v1", nil },
		Render:       func(MutationDiff) (string, error) { return "diagnostic", nil },
		OnMutationTx: completeNoopHook,
	}, func(context.Context, Scope) (CompleteProjection, error) {
		projected := options
		projected.ActiveUserLayerIDs = active()
		return projected, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestConflictDiagnosticProjetaERevalidaAtivacaoDinamica(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	if err := fixture.projection.db.Where("id = ?", fixture.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	secondLayer := storeTestLayer(t, fixture.projection.db, fixture.projection.scope.UserID, nil, "segunda")
	snapshot, err := fixture.projection.store.Load(context.Background(), fixture.projection.scope)
	if err != nil || len(snapshot.Layers) == 0 {
		t.Fatalf("reler camadas para diagnóstico: snapshot=%+v err=%v", snapshot.Layers, err)
	}
	firstLayer := snapshot.Layers[0].ID
	active := []string{secondLayer.ID, firstLayer}
	service := newDynamicConflictDiagnosticService(t, fixture, func() []string { return slices.Clone(active) })

	diagnostic, err := service.CheckConflicts(context.Background(), "token", nil)
	if err != nil {
		t.Fatalf("CheckConflicts com ativação dinâmica: %v", err)
	}
	wantActive := []string{firstLayer, secondLayer.ID}
	if !slices.Equal(diagnostic.activeUserLayerIDs, wantActive) {
		t.Fatalf("ativação dinâmica não foi canonizada no diagnóstico: got=%v want=%v", diagnostic.activeUserLayerIDs, wantActive)
	}

	active = []string{firstLayer, secondLayer.ID}
	if err := service.RevalidateConflicts(context.Background(), "token", diagnostic); err != nil {
		t.Fatalf("reordenação semântica da ativação ficou stale: %v", err)
	}
	active = []string{firstLayer}
	if err := service.RevalidateConflicts(context.Background(), "token", diagnostic); !errors.Is(err, ErrStale) {
		t.Fatalf("mudança dinâmica não invalidou diagnóstico: %v", err)
	}
}

func TestConflictDiagnosticRecusaAtivacaoDinamicaInconsistente(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	if err := fixture.projection.db.Where("id = ?", fixture.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	service := newDynamicConflictDiagnosticService(t, fixture, func() []string {
		return []string{"layer-not-in-snapshot"}
	})

	if _, err := service.CheckConflicts(context.Background(), "token", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ativação dinâmica inconsistente aceita ou erro inesperado: %v", err)
	}
}

func TestPreviewActivationPreservaContextualComClaimManualExpirada(t *testing.T) {
	user, layerID := storeTestUUID7(t), storeTestUUID7(t)
	manualRuleID, contextRuleID := storeTestUUID7(t), storeTestUUID7(t)
	expiredAt := time.Now().UTC().Add(-time.Minute)
	snapshot := Snapshot{
		Scope:  Scope{UserID: user},
		Layers: []Layer{{ID: layerID, UserID: user, Source: "user", Enabled: true}},
		ActivationRules: []commandactivation.Rule{
			{ID: manualRuleID, UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: manualRuleID, Mode: commandactivation.ModeManual, Enabled: true},
			{ID: contextRuleID, UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: contextRuleID, Mode: commandactivation.ModeContext, Enabled: true},
		},
		ActivationClaims: []commandactivation.Claim{
			{ActivationID: storeTestUUID7(t), UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: manualRuleID, State: commandactivation.StateActive, ExpiresAt: &expiredAt},
			{ActivationID: storeTestUUID7(t), UserID: user, LayerRefKind: commandactivation.UserRef, LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: contextRuleID, State: commandactivation.StateActive},
		},
	}

	active, err := validatePreviewActiveUserLayerIDs(snapshot, []string{layerID})
	if err != nil || !slices.Equal(active, []string{layerID}) {
		t.Fatalf("claim manual expirada invalidou contexto independente: active=%v err=%v", active, err)
	}
}

func TestCompleteMutationPrepareValidaProvaDinamicaSemEfeitos(t *testing.T) {
	fixture := decisionTestFixtureFor(t, commanddecision.ApplyAction, false)
	if err := fixture.projection.db.Where("id = ?", fixture.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.projection.store.Load(context.Background(), fixture.projection.scope)
	if err != nil || len(snapshot.Layers) != 1 {
		t.Fatalf("reler camada para prepare: layers=%d err=%v", len(snapshot.Layers), err)
	}
	layer := snapshot.Layers[0]
	active := []string{layer.ID}
	service := newDynamicConflictDiagnosticService(t, fixture, func() []string { return slices.Clone(active) })
	updated := layer
	updated.Description = "descrição atualizada"

	prepared, err := service.service.config.Store.PrepareMutation(context.Background(), fixture.projection.scope, MutationIntent{
		Operation: LayerUpdate, ID: layer.ID, Layer: &updated,
	}, service.service.config.Validate)
	if err != nil || prepared == nil {
		t.Fatalf("prepare completo não validou prova dinâmica: prepared=%v err=%v", prepared, err)
	}
	var stored Layer
	if err := fixture.projection.db.Where("id = ?", layer.ID).Take(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Description != layer.Description {
		t.Fatalf("prepare alterou camada sem commit: got=%q want=%q", stored.Description, layer.Description)
	}
}
