package commandconfig

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"gorm.io/gorm"
)

func prepareConfirmedImportBatchTest(t *testing.T, f activationHookFixture, scope Scope, name string) *ConfirmedMutation {
	t.Helper()
	before, err := f.config.Load(context.Background(), scope)
	if err != nil {
		t.Fatalf("Load %v: %v", name, err)
	}
	after := cloneConfigSnapshot(before)
	if len(after.Bindings) == 0 {
		t.Fatalf("snapshot %v sem binding local", name)
	}
	for i := range after.Bindings {
		if sameWorkspace(after.Bindings[i].WorkspaceID, scope.WorkspaceID) {
			after.Bindings[i].Arguments = `{"batch":"` + name + `"}`
			p, err := f.config.prepareImportedSnapshot(context.Background(), scope, before, ImportedSnapshot{
				Snapshot: after, TouchedBindingIDs: []string{after.Bindings[i].ID},
			}, completeTestValidator)
			if err != nil {
				t.Fatalf("PrepareImportedSnapshot %v: %v", name, err)
			}
			confirmed, err := f.config.ConfirmMutation(context.Background(), p, f.base.epoch, f.base.receipts, "v1", func(context.Context, string) ([]byte, error) {
				return bytes.Repeat([]byte{0x42}, 32), nil
			}, time.Now().Add(time.Minute), func(MutationDiff) (string, error) { return "batch import", nil })
			if err != nil {
				t.Fatalf("ConfirmMutation %v: %v", name, err)
			}
			return confirmed
		}
	}
	t.Fatalf("snapshot %v sem binding no escopo exato", name)
	return nil
}

func batchImportScopes(t *testing.T, f activationHookFixture) (Scope, Scope, *ConfirmedMutation, *ConfirmedMutation) {
	t.Helper()
	workspace := storeTestUUID7(t)
	if err := f.config.EnsureScope(context.Background(), Scope{UserID: f.base.epoch.UserID, WorkspaceID: &workspace}); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	if _, err := f.config.Load(context.Background(), Scope{UserID: f.base.epoch.UserID, WorkspaceID: &workspace}); err != nil {
		t.Fatalf("Load workspace: %v", err)
	}
	localLayer := storeTestLayer(t, f.db, f.base.epoch.UserID, &workspace, "workspace-original")
	storeTestBinding(t, f.db, f.base.epoch.UserID, &workspace, localLayer, "active")
	global := Scope{UserID: f.base.epoch.UserID}
	local := Scope{UserID: f.base.epoch.UserID, WorkspaceID: &workspace}
	return global, local, prepareConfirmedImportBatchTest(t, f, global, "global-imported"), prepareConfirmedImportBatchTest(t, f, local, "workspace-imported")
}

func TestCommitConfirmedImportBatchGlobalWorkspaceComposto(t *testing.T) {
	f := newActivationHookFixture(t)
	global, local, globalConfirmed, localConfirmed := batchImportScopes(t, f)
	if err := f.config.CommitConfirmedImportBatch(context.Background(), []*ConfirmedMutation{globalConfirmed, localConfirmed}, f.base.epoch, func(_ context.Context, _ *gorm.DB, _ MutationDiff) error { return nil }); err != nil {
		t.Fatalf("CommitConfirmedImportBatch: %v", err)
	}
	globalAfter, err := f.config.Load(context.Background(), global)
	if err != nil {
		t.Fatal(err)
	}
	localAfter, err := f.config.Load(context.Background(), local)
	if err != nil {
		t.Fatal(err)
	}
	if !hasBindingArguments(globalAfter, `{"batch":"global-imported"}`) || !hasBindingArguments(localAfter, `{"batch":"workspace-imported"}`) {
		t.Fatalf("lote não aplicou os dois escopos: global=%v local=%v", globalAfter.Bindings, localAfter.Bindings)
	}
	for _, c := range []*ConfirmedMutation{globalConfirmed, localConfirmed} {
		if got := loadDecisionReceiptProbe(t, f.db, c.request.DecisionID); got.ConsumedAt.Valid == false {
			t.Fatalf("receipt não consumido: %s", c.request.DecisionID)
		}
	}
	var audits int64
	if err := f.db.Table("command_config_mutations").Where("operation = ?", string(ConfigImport)).Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if audits != 2 {
		t.Fatalf("auditorias=%d, esperado 2", audits)
	}
}

func TestCommitConfirmedImportBatchHookTardioFazRollbackCompleto(t *testing.T) {
	f := newActivationHookFixture(t)
	_, _, globalConfirmed, localConfirmed := batchImportScopes(t, f)
	beforeRows := map[string]int64{}
	for _, table := range []string{"command_layers", "command_config_generations", "command_config_mutations"} {
		beforeRows[table] = countRows(t, f.db, table)
	}
	beforeGlobal, err := f.config.Load(context.Background(), Scope{UserID: f.base.epoch.UserID})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	err = f.config.CommitConfirmedImportBatch(context.Background(), []*ConfirmedMutation{globalConfirmed, localConfirmed}, f.base.epoch, func(_ context.Context, tx *gorm.DB, _ MutationDiff) error {
		calls++
		if calls == 2 {
			if result := tx.Model(&Binding{}).Where("workspace_id IS NOT NULL").Update("arguments", `{"tampered":true}`); result.Error != nil {
				return result.Error
			}
			return errors.New("hook tardio")
		}
		return nil
	})
	if err == nil || calls != 2 {
		t.Fatalf("hook tardio: err=%v calls=%d", err, calls)
	}
	for table, want := range beforeRows {
		if got := countRows(t, f.db, table); got != want {
			t.Fatalf("rollback %s: got=%d want=%d", table, got, want)
		}
	}
	afterGlobal, err := f.config.Load(context.Background(), Scope{UserID: f.base.epoch.UserID})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeGlobal, afterGlobal) {
		t.Fatalf("global alterado após rollback: before=%#v after=%#v", beforeGlobal, afterGlobal)
	}
	for _, c := range []*ConfirmedMutation{globalConfirmed, localConfirmed} {
		if got := loadDecisionReceiptProbe(t, f.db, c.request.DecisionID); got.ConsumedAt.Valid {
			t.Fatalf("receipt consumido apesar do rollback: %s", c.request.DecisionID)
		}
	}
}

func TestCommitConfirmedImportBatchVerificaEscopoAnteriorAlteradoPeloUltimoHook(t *testing.T) {
	f := newActivationHookFixture(t)
	_, local, globalConfirmed, localConfirmed := batchImportScopes(t, f)
	var beforeLayer Layer
	if err := f.db.Where("user_id = ? AND workspace_id = ?", f.base.epoch.UserID, *local.WorkspaceID).Take(&beforeLayer).Error; err != nil {
		t.Fatal(err)
	}
	beforeRows := map[string]int64{}
	for _, table := range []string{"command_layers", "command_config_generations", "command_config_mutations"} {
		beforeRows[table] = countRows(t, f.db, table)
	}
	calls := 0
	err := f.config.CommitConfirmedImportBatch(context.Background(), []*ConfirmedMutation{globalConfirmed, localConfirmed}, f.base.epoch, func(_ context.Context, tx *gorm.DB, _ MutationDiff) error {
		calls++
		if calls != 2 {
			return nil
		}
		result := tx.Model(&Layer{}).Where("id = ? AND user_id = ? AND workspace_id = ?", beforeLayer.ID, beforeLayer.UserID, *beforeLayer.WorkspaceID).Update("name", "hook-tampered")
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			t.Fatalf("hook não alterou exatamente uma camada: %d", result.RowsAffected)
		}
		return nil
	})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("alteração tardia não foi recusada: %v", err)
	}
	if calls != 2 {
		t.Fatalf("hooks executados=%d, esperado 2", calls)
	}
	var afterLayer Layer
	if err := f.db.Where("id = ?", beforeLayer.ID).Take(&afterLayer).Error; err != nil {
		t.Fatal(err)
	}
	if afterLayer.Name != beforeLayer.Name {
		t.Fatalf("alteração tardia não sofreu rollback: got=%q want=%q", afterLayer.Name, beforeLayer.Name)
	}
	for table, want := range beforeRows {
		if got := countRows(t, f.db, table); got != want {
			t.Fatalf("rollback %s: got=%d want=%d", table, got, want)
		}
	}
	for _, c := range []*ConfirmedMutation{globalConfirmed, localConfirmed} {
		if loadDecisionReceiptProbe(t, f.db, c.request.DecisionID).ConsumedAt.Valid {
			t.Fatalf("receipt consumido apesar de ErrStale: %s", c.request.DecisionID)
		}
	}
}

func TestCommitConfirmedImportBatchRejeitaReplayEStaleEmQualquerEscopo(t *testing.T) {
	f := newActivationHookFixture(t)
	_, _, globalConfirmed, localConfirmed := batchImportScopes(t, f)
	if err := f.config.CommitConfirmedImportBatch(context.Background(), []*ConfirmedMutation{globalConfirmed, localConfirmed}, f.base.epoch, func(_ context.Context, _ *gorm.DB, _ MutationDiff) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := f.config.CommitConfirmedImportBatch(context.Background(), []*ConfirmedMutation{globalConfirmed, localConfirmed}, f.base.epoch, func(_ context.Context, _ *gorm.DB, _ MutationDiff) error { return nil }); !errors.Is(err, commanddecision.ErrStale) && !errors.Is(err, ErrStale) {
		t.Fatalf("replay=%v, esperado stale", err)
	}

	f2 := newActivationHookFixture(t)
	_, local2, global2, workspace2 := batchImportScopes(t, f2)
	var generation Generation
	if err := f2.db.Where("user_id = ? AND workspace_id = ?", f2.base.epoch.UserID, *local2.WorkspaceID).Take(&generation).Error; err != nil {
		t.Fatal(err)
	}
	if err := f2.db.Model(&Generation{}).Where("id = ?", generation.ID).Update("generation", generation.Generation+1).Error; err != nil {
		t.Fatal(err)
	}
	if err := f2.config.CommitConfirmedImportBatch(context.Background(), []*ConfirmedMutation{global2, workspace2}, f2.base.epoch, func(_ context.Context, _ *gorm.DB, _ MutationDiff) error { return nil }); !errors.Is(err, ErrStale) && !errors.Is(err, commanddecision.ErrStale) {
		t.Fatalf("stale workspace=%v, esperado stale", err)
	}
	if got := loadDecisionReceiptProbe(t, f2.db, global2.request.DecisionID); got.ConsumedAt.Valid {
		t.Fatal("receipt global consumido em lote stale")
	}
}

func hasBindingArguments(snapshot Snapshot, want string) bool {
	for _, binding := range snapshot.Bindings {
		if binding.Arguments == want {
			return true
		}
	}
	return false
}

func TestCommitConfirmedImportBatchRejectsLateGenerationChange(t *testing.T) {
	f := newActivationHookFixture(t)
	_, local, globalConfirmed, localConfirmed := batchImportScopes(t, f)
	ctx := context.Background()
	before, err := f.config.Load(ctx, local)
	if err != nil {
		t.Fatal(err)
	}
	err = f.config.CommitConfirmedImportBatch(ctx, []*ConfirmedMutation{globalConfirmed, localConfirmed}, f.base.epoch, func(_ context.Context, tx *gorm.DB, diff MutationDiff) error {
		if diff.Scope.WorkspaceID != nil {
			return nil
		}
		result := tx.Model(&Generation{}).Where("user_id = ? AND workspace_id = ?", local.UserID, *local.WorkspaceID).Update("generation", gorm.Expr("generation + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			t.Fatalf("geração alvo ausente: %d", result.RowsAffected)
		}
		return nil
	})
	if !errors.Is(err, ErrStale) {
		t.Fatalf("geração divergente aceita: %v", err)
	}
	after, err := f.config.Load(ctx, local)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rollback não restaurou configuração e gerações")
	}
	if countRows(t, f.db, "command_config_mutations") != 0 {
		t.Fatal("auditoria sobreviveu ao rollback")
	}
	for _, c := range []*ConfirmedMutation{globalConfirmed, localConfirmed} {
		if loadDecisionReceiptProbe(t, f.db, c.request.DecisionID).ConsumedAt.Valid {
			t.Fatal("receipt consumido apesar de rollback")
		}
	}
}
