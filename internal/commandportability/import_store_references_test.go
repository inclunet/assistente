package commandportability

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestApplyPlanImportResolvesCredentialsFromDestinationStore(t *testing.T) {
	for _, scenario := range []struct {
		name                    string
		local, foreign, enabled bool
	}{
		{name: "missing"},
		{name: "foreign", foreign: true},
		{name: "available", local: true, enabled: true},
		{name: "same pattern in both owners", local: true, foreign: true, enabled: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
			if err := f.db.AutoMigrate(&database.CredentialEntry{}); err != nil {
				t.Fatal(err)
			}
			for _, entry := range []struct {
				present bool
				user    string
			}{
				{scenario.local, f.user}, {scenario.foreign, applyImportUUID(t)},
			} {
				if !entry.present {
					continue
				}
				if err := f.db.Create(&database.CredentialEntry{
					UUIDModel: database.UUIDModel{ID: applyImportUUID(t)}, UserID: entry.user,
					Pattern: "*.example.test", TokenEnc: "intentionally-not-decryptable",
				}).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx := database.WithUserID(context.Background(), f.user)
			refs := applyImportRefs(t, f)
			refs.CredentialPattern = NewCredentialPatternResolver(f.db)
			options := PlanOptions{Mode: ReplaceMode, Name: NewStoreLayerName(f.db)}
			owner := NewStoreOwnership(f.db)
			binding := applyImportBinding(t, f.layer.ID, applyImportCommand)
			binding.Arguments = `{"credential":{"kind":"credential","pattern":"*.example.test"}}`
			layers := []LayerExport{applyImportLayer(f, "importada", &binding)}
			plan, err := PlanImport(ctx, layers, options, owner, refs)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Layers) != 1 || plan.Layers[0].Layer.Bindings[0].Enabled != scenario.enabled {
				t.Fatalf("plano não reflete disponibilidade local: %+v", plan)
			}
			if !scenario.enabled && len(plan.Warnings) == 0 {
				t.Fatal("referência indisponível sem aviso")
			}
			if _, err := ApplyPlanImport(ctx, f.service, "token", nil, layers, options, owner, refs); err != nil {
				t.Fatal(err)
			}
			after, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user})
			if err != nil {
				t.Fatal(err)
			}
			if len(after.Bindings) != 1 || after.Bindings[0].Enabled != scenario.enabled {
				t.Fatalf("persistência divergiu do plano: %+v", after.Bindings)
			}
			if f.presenter.calls != 1 {
				t.Fatalf("decisões: %d", f.presenter.calls)
			}
		})
	}
}

func TestApplyPlanImportStoreOwnershipRejectsWholeForeignBatch(t *testing.T) {
	f := newApplyImportFixture(t, func(context.Context, *gorm.DB, commandconfig.MutationDiff) error { return nil })
	foreign := f.layer
	foreign.ID, foreign.UserID, foreign.Name = applyImportUUID(t), applyImportUUID(t), "private-name"
	if err := f.db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	layers := []LayerExport{applyImportLayer(f, "must not be written", nil), applyImportLayer(f, "attempt", nil)}
	layers[1].ID = foreign.ID
	ctx := database.WithUserID(context.Background(), f.user)
	_, err := ApplyPlanImport(ctx, f.service, "token", nil, layers,
		PlanOptions{Mode: ReplaceMode, Name: NewStoreLayerName(f.db)}, NewStoreOwnership(f.db), applyImportRefs(t, f))
	if !errors.Is(err, ErrForeignOwner) {
		t.Fatalf("foreign batch: %v", err)
	}
	after, err := f.store.Load(ctx, commandconfig.Scope{UserID: f.user})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Layers) != 1 || after.Layers[0].Name != f.layer.Name || f.presenter.calls != 0 {
		t.Fatalf("lote recusado teve efeito: %+v, decisões=%d", after.Layers, f.presenter.calls)
	}
}
