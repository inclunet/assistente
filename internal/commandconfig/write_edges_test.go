package commandconfig

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

func TestBindingEnabledWriterRejectsMissingProposalAndCancelledContext(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []*BindingEnabledChange{nil, {}} {
		if err := fixture.store.CommitBindingEnabled(context.Background(), invalid); !errors.Is(err, ErrInvalid) {
			t.Fatal("proposta ausente aceita", err)
		}
	}
	if err := fixture.store.CommitBindingEnabled(nil, change); !errors.Is(err, ErrInvalid) { //nolint:staticcheck // Testa deliberadamente a recusa de contexto nil.
		t.Fatal("contexto nil aceito", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fixture.store.CommitBindingEnabled(ctx, change); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelamento ignorado", err)
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, binding) {
		t.Fatal("cancelamento alterou binding")
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, fixture.snapshot.Generations[0].ID), fixture.snapshot.Generations[0].ID, 1)
}

func TestBindingEnabledCancellationAfterUpdateRollsBackBothRows(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	change, err := fixture.store.PrepareBindingEnabled(ctx, fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	updated := false
	const hook = "fixture:cancel-command-binding-update"
	if err := fixture.db.Callback().Update().After("gorm:update").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_bindings" && tx.Error == nil && tx.RowsAffected == 1 {
			updated = true
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.db.Callback().Update().Remove(hook); err != nil {
			t.Errorf("remover callback %q: %v", hook, err)
		}
	})
	if err := fixture.store.CommitBindingEnabled(ctx, change); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelamento após UPDATE não propagado", err)
	}
	if !updated {
		t.Fatal("teste não alcançou UPDATE do binding")
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, binding) {
		t.Fatal("UPDATE não revertido")
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, fixture.snapshot.Generations[0].ID), fixture.snapshot.Generations[0].ID, 1)
}

func TestBindingEnabledDoesNotImplicitlyRebaseDefaults(t *testing.T) {
	for _, kind := range []string{"semantic", "version"} {
		t.Run(kind, func(t *testing.T) {
			fixture := newProjectionTestFixture(t)
			binding := projectionTestGlobalBinding(t, fixture, "application.defaults", "execute", "active", projectionCommandID, "fp-v1")
			// Baseline válida antes de introduzir exatamente uma mudança de catálogo.
			if _, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options); err != nil {
				t.Fatal(err)
			}
			if kind == "semantic" {
				fixture.options.BuiltinLayers[0].Defaults[0].Fingerprint = "changed"
			} else {
				fixture.options.BuiltinLayers[0].Defaults[0].Version = "2"
			}
			if change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options); !errors.Is(err, ErrInvalid) || change != nil {
				t.Fatal("toggle fez rebase implícito", err)
			}
		})
	}
}
