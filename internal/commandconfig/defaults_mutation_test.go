package commandconfig

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
)

func defaultMutationFixture(t *testing.T, version, fingerprint, rowVersion, rowFingerprint, status string, condition string) (*Store, Scope, CompleteProjection, Binding) {
	t.Helper()
	db := storeTestDB(t)
	user := storeTestUUID7(t)
	storeTestGeneration(t, db, user, nil, 1)
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.BuiltinLayers[0].Defaults[0].Version = version
	options.BuiltinLayers[0].Defaults[0].Fingerprint = fingerprint
	row := completeProjectionBinding(t, user, nil, Layer{}, completeProjectionUUID(t))
	row.LayerRefKind, row.LayerRef = "builtin", "application.defaults"
	row.ReplacesDefaultID = stringPtr("builtin.tab.new")
	row.ReplacesDefaultVersion = stringPtr(rowVersion)
	row.ReplacesDefaultFingerprint = stringPtr(rowFingerprint)
	row.ReviewStatus = status
	row.Condition = condition
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return store, Scope{UserID: user}, options, row
}

func TestPrepareDefaultUpgradeAvancaSomenteVersaoComFingerprintIgual(t *testing.T) {
	store, scope, options, row := defaultMutationFixture(t, "2", "fp-v1", "1", "fp-v1", "active", completeProjectionCondition)
	prepared, err := store.PrepareDefaultUpgrade(context.Background(), scope, options)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.store != store || prepared.Diff().Operation != DefaultUpgrade {
		t.Fatalf("proposta não ligada ao Store/operação: store=%v diff=%+v", prepared.store == store, prepared.Diff())
	}
	diff := prepared.Diff()
	if len(diff.BeforeBindings) != 1 || len(diff.AfterBindings) != 1 || *diff.AfterBindings[0].ReplacesDefaultVersion != "2" || *diff.AfterBindings[0].ReplacesDefaultFingerprint != "fp-v1" || diff.AfterBindings[0].ReviewStatus != "active" {
		t.Fatalf("avanço de versão incorreto: before=%+v after=%+v", diff.BeforeBindings, diff.AfterBindings)
	}
	if got, err := store.Load(context.Background(), scope); err != nil || !reflect.DeepEqual(got.Bindings[0], row) {
		t.Fatalf("preview escreveu ou alterou o banco: got=%+v err=%v", got.Bindings, err)
	}
	diff.AfterBindings[0].ReplacesDefaultVersion = stringPtr("adulterado")
	if *prepared.Diff().AfterBindings[0].ReplacesDefaultVersion != "2" {
		t.Fatal("diff privado foi exposto por alias")
	}
}

func TestPrepareDefaultUpgradeMarcaNeedsReviewSemMudarTrio(t *testing.T) {
	store, scope, options, row := defaultMutationFixture(t, "2", "fp-v2", "1", "fp-v1", "active", completeProjectionCondition)
	prepared, err := store.PrepareDefaultUpgrade(context.Background(), scope, options)
	if err != nil {
		t.Fatal(err)
	}
	after := prepared.Diff().AfterBindings[0]
	if after.ReviewStatus != string(commandbindings.NeedsReview) || *after.ReplacesDefaultVersion != "1" || *after.ReplacesDefaultFingerprint != "fp-v1" {
		t.Fatalf("mudança semântica não falhou fechado: %+v", after)
	}
	if row.ReviewStatus != "active" {
		t.Fatal("fixture alterada por referência")
	}
}

func TestPrepareDefaultUpgradeNoOpFalha(t *testing.T) {
	store, scope, options, _ := defaultMutationFixture(t, "1", "fp-v1", "1", "fp-v1", "active", completeProjectionCondition)
	prepared, err := store.PrepareDefaultUpgrade(context.Background(), scope, options)
	if prepared != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("upgrade sem mudança aceito: prepared=%v err=%v", prepared, err)
	}
}

func TestPrepareDefaultUpgradeNaoAlteraBindingGlobalEmEscopoWorkspace(t *testing.T) {
	store, scope, options, row := defaultMutationFixture(t, "2", "fp-v1", "1", "fp-v1", "active", completeProjectionCondition)
	workspace := "workspace-opaco"
	storeTestGeneration(t, store.db, scope.UserID, &workspace, 1)
	scope.WorkspaceID = &workspace
	prepared, err := store.PrepareDefaultUpgrade(context.Background(), scope, options)
	if prepared != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("upgrade global vazou para escopo workspace: prepared=%v err=%v", prepared, err)
	}
	global, err := store.Load(context.Background(), Scope{UserID: scope.UserID})
	if err != nil || len(global.Bindings) != 1 || !reflect.DeepEqual(global.Bindings[0], row) {
		t.Fatalf("binding global foi alterado: bindings=%+v err=%v", global.Bindings, err)
	}
}

func TestPrepareDefaultRebaseExigeTrioAtualECondicaoEstreita(t *testing.T) {
	condition := `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"dev"}]}`
	store, scope, options, row := defaultMutationFixture(t, "2", "fp-v2", "1", "fp-v1", string(commandbindings.NeedsReview), condition)
	request := DefaultRebaseRequest{BindingID: row.ID,
		Default:   DefaultReference{ID: "builtin.tab.new", Version: "2", Fingerprint: "fp-v2"},
		Condition: commandbindings.Facts{commandbindings.Profile: "dev"}}
	prepared, err := store.PrepareDefaultRebase(context.Background(), scope, request, options)
	if err != nil {
		t.Fatal(err)
	}
	diff := prepared.Diff()
	if diff.Operation != DefaultRebase || len(diff.BeforeBindings) != 1 || len(diff.AfterBindings) != 1 || diff.AfterBindings[0].ReviewStatus != "active" || *diff.AfterBindings[0].ReplacesDefaultVersion != "2" || *diff.AfterBindings[0].ReplacesDefaultFingerprint != "fp-v2" {
		t.Fatalf("rebase não preservou override com trio novo: %+v", diff)
	}
	request.Default.Fingerprint = "fp-outro"
	if prepared, err := store.PrepareDefaultRebase(context.Background(), scope, request, options); prepared != nil || !errors.Is(err, ErrStale) {
		t.Fatalf("trio atual não validado: prepared=%v err=%v", prepared, err)
	}
	request.Default.Fingerprint = "fp-v2"
	request.Condition = commandbindings.Facts{commandbindings.Profile: "prod"}
	if prepared, err := store.PrepareDefaultRebase(context.Background(), scope, request, options); prepared != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("condição ampla/diferente aceita: prepared=%v err=%v", prepared, err)
	}
}

func TestPrepareDefaultRebaseNaoAtingeBindingGlobalEmEscopoWorkspace(t *testing.T) {
	condition := `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"dev"}]}`
	store, scope, options, row := defaultMutationFixture(t, "2", "fp-v2", "1", "fp-v1", string(commandbindings.NeedsReview), condition)
	workspace := "workspace-opaco"
	storeTestGeneration(t, store.db, scope.UserID, &workspace, 1)
	request := DefaultRebaseRequest{BindingID: row.ID,
		Default:   DefaultReference{ID: "builtin.tab.new", Version: "2", Fingerprint: "fp-v2"},
		Condition: commandbindings.Facts{commandbindings.Profile: "dev"}}
	prepared, err := store.PrepareDefaultRebase(context.Background(), Scope{UserID: scope.UserID, WorkspaceID: &workspace}, request, options)
	if prepared != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("rebase de workspace editou binding global: prepared=%v err=%v", prepared, err)
	}
}

func TestPrepareDefaultRebaseRejeitaDefaultIDDiferente(t *testing.T) {
	condition := `{"version":1,"clauses":[{"field":"profile","op":"eq","value":"dev"}]}`
	store, scope, options, row := defaultMutationFixture(t, "2", "fp-v2", "1", "fp-v1", string(commandbindings.NeedsReview), condition)
	request := DefaultRebaseRequest{BindingID: row.ID,
		Default:   DefaultReference{ID: "builtin.outro", Version: "2", Fingerprint: "fp-v2"},
		Condition: commandbindings.Facts{commandbindings.Profile: "dev"}}
	prepared, err := store.PrepareDefaultRebase(context.Background(), scope, request, options)
	if prepared != nil || !errors.Is(err, ErrStale) {
		t.Fatalf("default ID divergente aceito: prepared=%v err=%v", prepared, err)
	}
}

func TestPrepareDefaultRebaseAceitaConjuncaoVaziaExata(t *testing.T) {
	store, scope, options, row := defaultMutationFixture(t, "2", "fp-v2", "1", "fp-v1", string(commandbindings.NeedsReview), completeProjectionCondition)
	request := DefaultRebaseRequest{BindingID: row.ID,
		Default:   DefaultReference{ID: "builtin.tab.new", Version: "2", Fingerprint: "fp-v2"},
		Condition: commandbindings.Facts{}}
	prepared, err := store.PrepareDefaultRebase(context.Background(), scope, request, options)
	if err != nil || prepared == nil {
		t.Fatalf("conjunção vazia exata rejeitada: prepared=%v err=%v", prepared, err)
	}
}
