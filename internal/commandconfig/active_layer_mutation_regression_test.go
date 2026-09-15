package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandsecurity"
)

func completeServiceWithActiveLayerProof(t *testing.T, fixture activationHookFixture, activeIDs func() []string) *CompleteMutationService {
	t.Helper()
	if err := fixture.db.Where("id = ?", fixture.base.binding.ID).Delete(&Binding{}).Error; err != nil {
		t.Fatalf("remover binding legado da fixture: %v", err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatalf("commandsecurity.NewEpochService: %v", err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	service, err := NewCompleteMutationService(MutationServiceConfig{
		Store: fixture.config,
		Sessions: mutationSessionFixture{principal: auth.LocalSessionPrincipal{
			UserID: fixture.base.epoch.UserID, SessionID: fixture.base.epoch.SessionID,
		}},
		Epochs: epochs, Receipts: fixture.base.receipts,
		Keys:       func(context.Context, string) ([]byte, error) { return bytes.Repeat([]byte{0x42}, 32), nil },
		KeyVersion: "v1", DecisionTTL: time.Minute,
		Authorize:    func(context.Context, auth.LocalSessionPrincipal, Scope, Operation) error { return nil },
		Version:      func(context.Context) (string, error) { return "catalog-v1", nil },
		Render:       func(MutationDiff) (string, error) { return "layer mutation", nil },
		OnMutationTx: fixture.hook(t),
	}, func(context.Context, Scope) (CompleteProjection, error) {
		projected := options
		projected.ActiveUserLayerIDs = append([]string(nil), activeIDs()...)
		return projected, nil
	})
	if err != nil {
		t.Fatalf("NewCompleteMutationService: %v", err)
	}
	return service
}

func TestCompleteServiceLayerDisableAceitaProvaDaCamadaAntesDaMutacao(t *testing.T) {
	fixture := newActivationHookFixture(t)
	service := completeServiceWithActiveLayerProof(t, fixture, func() []string { return []string{fixture.layer.ID} })

	diff, err := service.Apply(context.Background(), "token", nil, MutationIntent{Operation: LayerDisable, ID: fixture.layer.ID})
	if err != nil {
		t.Fatalf("LayerDisable de camada comprovadamente ativa: %v", err)
	}
	found := false
	for _, layer := range diff.AfterLayers {
		if layer.ID == fixture.layer.ID {
			found = true
			if layer.Enabled {
				t.Fatalf("camada desabilitada permaneceu habilitada no after: %+v", layer)
			}
		}
	}
	if !found {
		t.Fatal("camada desabilitada ausente no after")
	}
	var persisted Layer
	if err := fixture.db.Where("id = ?", fixture.layer.ID).Take(&persisted).Error; err != nil {
		t.Fatalf("ler camada desabilitada: %v", err)
	}
	if persisted.Enabled {
		t.Fatal("LayerDisable não foi aplicado")
	}
}

func TestCompleteServiceLayerDeleteAceitaProvaDaCamadaAntesDaMutacao(t *testing.T) {
	fixture := newActivationHookFixture(t)
	service := completeServiceWithActiveLayerProof(t, fixture, func() []string { return []string{fixture.layer.ID} })

	diff, err := service.Apply(context.Background(), "token", nil, MutationIntent{Operation: LayerDelete, ID: fixture.layer.ID})
	if err != nil {
		t.Fatalf("LayerDelete de camada comprovadamente ativa: %v", err)
	}
	for _, layer := range diff.AfterLayers {
		if layer.ID == fixture.layer.ID {
			t.Fatalf("camada apagada permaneceu no diff: %+v", layer)
		}
	}
	var count int64
	if err := fixture.db.Model(&Layer{}).Where("id = ?", fixture.layer.ID).Count(&count).Error; err != nil {
		t.Fatalf("consultar camada apagada: %v", err)
	}
	if count != 0 {
		t.Fatalf("LayerDelete deixou %d linha(s) da camada", count)
	}
}

func TestCompleteServiceRejeitaProvaAtivaAusenteNoSnapshotAtual(t *testing.T) {
	fixture := newActivationHookFixture(t)
	unknownID := storeTestUUID7(t)
	service := completeServiceWithActiveLayerProof(t, fixture, func() []string { return []string{unknownID} })

	if _, err := service.Apply(context.Background(), "token", nil, MutationIntent{Operation: LayerDisable, ID: fixture.layer.ID}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ID ativo ausente no snapshot atual = %v, esperado ErrInvalid", err)
	}
	var persisted Layer
	if err := fixture.db.Where("id = ?", fixture.layer.ID).Take(&persisted).Error; err != nil {
		t.Fatalf("ler camada após rejeição: %v", err)
	}
	if !persisted.Enabled {
		t.Fatal("prova inválida alterou a camada")
	}
	if got := countRows(t, fixture.db, "command_config_mutations"); got != 0 {
		t.Fatalf("prova inválida criou %d auditoria(s)", got)
	}
	if fixture.base.presenter.calls != 0 {
		t.Fatalf("prova inválida chegou ao presenter: %d chamada(s)", fixture.base.presenter.calls)
	}
}
