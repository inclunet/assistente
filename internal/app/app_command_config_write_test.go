package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

// Usa o mesmo App autenticado e SQLite temporário da fixture de execução;
// nunca abre banco do usuário, diálogo nativo ou store de credenciais real.
func exerciseCommandBindingWrites(t *testing.T, app *App, db *gorm.DB, store *commandconfig.Store,
	state *commandexecution.HostState, token, userID, bindingID string, options commandconfig.LocalReadProjection,
) {
	t.Helper()
	ctx := context.Background()
	authorize := func(_ context.Context, principal auth.LocalSessionPrincipal) error {
		if principal.UserID != userID {
			t.Fatal("política recebeu outro usuário")
		}
		return nil // Política exclusivamente de fixture, não default de produto.
	}
	load := func() (commandconfig.Binding, int64) {
		t.Helper()
		snapshot, err := store.Load(ctx, commandconfig.Scope{UserID: userID})
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range snapshot.Bindings {
			if row.ID == bindingID {
				return row, snapshot.Generations[0].Generation
			}
		}
		t.Fatal("binding da fixture ausente")
		return commandconfig.Binding{}, 0
	}
	before, generation := load()
	mapBefore, _, err := state.UserConfiguration(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	denied := errors.New("decisão negada de fixture")
	if err := app.changeCommandBindingEnabled(ctx, token, store, bindingID, false, options, authorize,
		func(_ context.Context, old, next commandconfig.Binding) error {
			if old.ID != bindingID || !old.Enabled || next.Enabled || old.UserID != userID || next.UserID != userID {
				t.Fatal("diff incorreto")
			}
			return denied
		}); !errors.Is(err, denied) {
		t.Fatal("negação não respeitada", err)
	}
	after, currentGeneration := load()
	mapAfter, _, err := state.UserConfiguration(ctx, userID)
	if after.Enabled != before.Enabled || currentGeneration != generation || err != nil || mapBefore != mapAfter {
		t.Fatal("negação alterou dados ou mapa", err)
	}
	neverConfirm := func(context.Context, commandconfig.Binding, commandconfig.Binding) error {
		t.Fatal("confirm sem autenticação")
		return nil
	}
	if err := app.changeCommandBindingEnabled(ctx, "invalid-token", store, bindingID, false, options, authorize, neverConfirm); err == nil {
		t.Fatal("JWT inválido aceito")
	}
	if err := app.changeCommandBindingEnabled(ctx, token, store, bindingID, false, options, authorize, nil); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatal("confirmação ausente aceita", err)
	}
	if err := app.changeCommandBindingEnabled(ctx, token, store, bindingID, false, options, nil, neverConfirm); !errors.Is(err, commandexecution.ErrInvalidConfiguration) {
		t.Fatal("política ausente aceita", err)
	}
	revoked := false
	if err := app.changeCommandBindingEnabled(ctx, token, store, bindingID, false, options,
		func(context.Context, auth.LocalSessionPrincipal) error {
			if revoked {
				return denied
			}
			return nil
		}, func(context.Context, commandconfig.Binding, commandconfig.Binding) error { revoked = true; return nil }); !errors.Is(err, denied) {
		t.Fatal("política revogada durante confirmação foi ignorada", err)
	}
	after, currentGeneration = load()
	if !after.Enabled || currentGeneration != generation {
		t.Fatal("revogação de política permitiu escrita")
	}
	if err := app.changeCommandBindingEnabled(ctx, token, store, bindingID, false, options, authorize,
		func(_ context.Context, old, next commandconfig.Binding) error {
			// Alterar a cópia exibida não pode modificar a proposta privada.
			*old.CommandID = "forged.command"
			next.Enabled = true
			return nil
		}); err != nil {
		t.Fatal(err)
	}
	after, currentGeneration = load()
	if after.Enabled || *after.CommandID != *before.CommandID || currentGeneration != generation+1 {
		t.Fatal("commit não corresponde à proposta")
	}
	if _, _, err := state.UserConfiguration(ctx, userID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatal("commit conservou mapa velho", err)
	}
	rebuild := func() {
		t.Helper()
		if err := app.rebuildPersistedLocalReadConfiguration(ctx, token, store, options); err != nil {
			t.Fatal(err)
		}
	}
	rebuild()
	// Uma gravação concorrente durante a decisão invalida a proposta. O mapa
	// permanece removido mesmo quando o commit CAS falha: não há rollback de cache.
	if err := app.changeCommandBindingEnabled(ctx, token, store, bindingID, true, options, authorize,
		func(context.Context, commandconfig.Binding, commandconfig.Binding) error {
			return db.Model(&commandconfig.Generation{}).Where("user_id = ? AND workspace_id IS NULL", userID).Update("generation", currentGeneration+1).Error
		}); !errors.Is(err, commandconfig.ErrStale) {
		t.Fatal("proposta concorrente aceita", err)
	}
	after, generation = load()
	if after.Enabled || generation != currentGeneration+1 {
		t.Fatal("stale mudou binding ou geração")
	}
	if _, _, err := state.UserConfiguration(ctx, userID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatal("stale manteve mapa", err)
	}
	rebuild()
	// A decisão pode demorar sem segurar o gate. Uma observação de lock nesse
	// intervalo deve invalidar a operação, ainda que o callback aprove depois.
	if err := app.changeCommandBindingEnabled(ctx, token, store, bindingID, true, options, authorize,
		func(context.Context, commandconfig.Binding, commandconfig.Binding) error {
			return state.SetOSSessionState(ctx, true, true)
		}); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatal("decisão sobreviveu ao lock", err)
	}
	after, currentGeneration = load()
	if after.Enabled || currentGeneration != generation {
		t.Fatal("lock não impediu escrita")
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	rebuild()
}
