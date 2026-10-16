package commandconfig

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm"
)

func writeTestBaseline(t *testing.T) (projectionTestFixture, Binding) {
	t.Helper()
	fixture := newProjectionTestFixture(t)
	layer := projectionTestLayer(t, fixture, true)
	binding := projectionTestBinding(t, fixture, layer)
	fixture.options.ActiveUserLayerIDs = []string{layer.ID}
	return fixture, binding
}

func writeTestBindingRow(t *testing.T, db *gorm.DB, id string) Binding {
	t.Helper()
	var row Binding
	if err := db.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("ler binding %s: %v", id, err)
	}
	return row
}

func writeTestGenerationRow(t *testing.T, db *gorm.DB, id string) Generation {
	t.Helper()
	var row Generation
	if err := db.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("ler geração %s: %v", id, err)
	}
	return row
}

func requireGeneration(t *testing.T, row Generation, id string, value int64) {
	t.Helper()
	if row.ID != id || row.UserID == "" || row.WorkspaceID != nil || row.Generation != value {
		t.Fatalf("geração inesperada: %#v, esperado id=%s generation=%d", row, id, value)
	}
}

func TestPrepareBindingEnabledNaoEscreveNoBanco(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	beforeBinding := writeTestBindingRow(t, fixture.db, binding.ID)
	beforeGeneration := writeTestGenerationRow(t, fixture.db, generation.ID)
	counts := map[string]int64{}
	for _, table := range []string{"command_layers", "command_bindings", "command_config_generations"} {
		counts[table] = countRows(t, fixture.db, table)
	}

	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatalf("PrepareBindingEnabled: %v", err)
	}
	if change == nil {
		t.Fatal("proposta nil")
	}
	before, after := change.Diff()
	if !reflect.DeepEqual(before, beforeBinding) || after.Enabled || !before.Enabled {
		t.Fatalf("diff inesperado: before=%#v after=%#v", before, after)
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, beforeBinding) {
		t.Fatalf("prepare alterou binding: antes=%#v depois=%#v", beforeBinding, got)
	}
	if got := writeTestGenerationRow(t, fixture.db, generation.ID); !reflect.DeepEqual(got, beforeGeneration) {
		t.Fatalf("prepare alterou geração: antes=%#v depois=%#v", beforeGeneration, got)
	}
	for table, want := range counts {
		if got := countRows(t, fixture.db, table); got != want {
			t.Fatalf("prepare alterou quantidade em %s: antes=%d depois=%d", table, want, got)
		}
	}
}

func TestCommitBindingEnabledAlteraSomenteBindingEgeracaoGlobal(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); err != nil {
		t.Fatalf("commit: %v", err)
	}

	gotBinding := writeTestBindingRow(t, fixture.db, binding.ID)
	wantBinding := binding
	wantBinding.Enabled = false
	if !reflect.DeepEqual(gotBinding, wantBinding) {
		t.Fatalf("binding alterado além de enabled: got=%#v want=%#v", gotBinding, wantBinding)
	}
	if got := countRows(t, fixture.db, "command_bindings"); got != 1 {
		t.Fatalf("quantidade de bindings = %d, esperado 1", got)
	}
	if got := countRows(t, fixture.db, "command_config_generations"); got != 1 {
		t.Fatalf("quantidade de gerações globais = %d, esperado 1", got)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, generation.ID), generation.ID, generation.Generation+1)
}

func TestCommitBindingEnabledReplayStaleSemAlteracao(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	committedBinding := writeTestBindingRow(t, fixture.db, binding.ID)
	committedGeneration := writeTestGenerationRow(t, fixture.db, generation.ID)
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); !errors.Is(err, ErrStale) {
		t.Fatalf("replay = %v, esperado ErrStale", err)
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, committedBinding) {
		t.Fatalf("replay alterou binding: antes=%#v depois=%#v", committedBinding, got)
	}
	if got := writeTestGenerationRow(t, fixture.db, generation.ID); !reflect.DeepEqual(got, committedGeneration) {
		t.Fatalf("replay incrementou geração: antes=%#v depois=%#v", committedGeneration, got)
	}
}

func TestCommitBindingEnabledDuasPropostasConcorrentesUmaVence(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	first, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	// O SQLite continua serializando os writers, mas as duas chamadas de commit
	// competem de fato pelo mesmo CAS quando são iniciadas juntas.
	db, err := fixture.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, proposal := range []*BindingEnabledChange{first, second} {
		wg.Add(1)
		go func(change *BindingEnabledChange) {
			defer wg.Done()
			<-start
			results <- fixture.store.CommitBindingEnabled(context.Background(), change)
		}(proposal)
	}
	close(start)
	wg.Wait()
	close(results)

	wins, stale := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrStale):
			stale++
		default:
			t.Fatalf("erro de commit concorrente: %v", err)
		}
	}
	if wins != 1 || stale != 1 {
		t.Fatalf("resultado concorrente: wins=%d stale=%d", wins, stale)
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); got.Enabled {
		t.Fatal("binding não foi desabilitado pelo commit vencedor")
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, fixture.snapshot.Generations[0].ID), fixture.snapshot.Generations[0].ID, 2)
}

func TestCommitBindingEnabledGeracaoStaleNaoMutaNada(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&Generation{}).Where("id = ?", generation.ID).Update("generation", 2).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); !errors.Is(err, ErrStale) {
		t.Fatalf("geração stale = %v, esperado ErrStale", err)
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, binding) {
		t.Fatalf("binding alterado: %#v", got)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, generation.ID), generation.ID, 2)
}

func TestCommitBindingEnabledDetectaABAMesmoContador(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Delete(&Generation{}, "id = ?", generation.ID).Error; err != nil {
		t.Fatal(err)
	}
	replacement := Generation{ID: storeTestUUID7(t), UserID: fixture.scope.UserID, Generation: generation.Generation}
	if err := fixture.db.Create(&replacement).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); !errors.Is(err, ErrStale) {
		t.Fatalf("ABA = %v, esperado ErrStale", err)
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, binding) {
		t.Fatalf("binding alterado no ABA: %#v", got)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, replacement.ID), replacement.ID, 1)
}

func TestCommitBindingEnabledBindingAlteradoSemGeracaoFazRollback(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&Binding{}).Where("id = ?", binding.ID).Update("resolution_priority", binding.ResolutionPriority+1).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); !errors.Is(err, ErrStale) {
		t.Fatalf("binding alterado = %v, esperado ErrStale", err)
	}
	wantBinding := binding
	wantBinding.ResolutionPriority++
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, wantBinding) {
		t.Fatalf("binding não preservado: got=%#v want=%#v", got, wantBinding)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, generation.ID), generation.ID, 1)
}

func TestCommitBindingEnabledBindingDeletadoFazRollbackDaGeracao(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Delete(&Binding{}, "id = ?", binding.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); !errors.Is(err, ErrStale) {
		t.Fatalf("binding removido = %v, esperado ErrStale", err)
	}
	if got := countRows(t, fixture.db, "command_bindings"); got != 0 {
		t.Fatalf("binding removido foi recriado: %d", got)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, generation.ID), generation.ID, 1)
}

func TestCommitBindingEnabledErroNoUpdateDoBindingFazRollback(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Exec(`CREATE TRIGGER write_test_reject_binding_update BEFORE UPDATE ON command_bindings BEGIN SELECT RAISE(ABORT, 'fixture'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); err == nil {
		t.Fatal("trigger não provocou erro")
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, binding) {
		t.Fatalf("binding alterado após rollback: got=%#v want=%#v", got, binding)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, generation.ID), generation.ID, 1)
}

func TestDiffMutadoNaoAlteraCommitNemTrioDeReplacement(t *testing.T) {
	fixture := newProjectionTestFixture(t)
	binding := projectionTestGlobalBinding(t, fixture, "application.defaults", "execute", "active", projectionCommandID, "fp-v1")
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	before, after := change.Diff()
	if before.CommandID == nil || before.ReplacesDefaultID == nil || before.ReplacesDefaultVersion == nil || before.ReplacesDefaultFingerprint == nil {
		t.Fatalf("fixture não contém ponteiros esperados: %#v", before)
	}
	*before.CommandID = "mutated.before"
	*before.ReplacesDefaultID = "mutated.before"
	*before.ReplacesDefaultVersion = "mutated.before"
	*before.ReplacesDefaultFingerprint = "mutated.before"
	*after.CommandID = "mutated.after"
	*after.ReplacesDefaultID = "mutated.after"
	*after.ReplacesDefaultVersion = "mutated.after"
	*after.ReplacesDefaultFingerprint = "mutated.after"
	unchangedBefore, unchangedAfter := change.Diff()
	if unchangedBefore.CommandID == nil || *unchangedBefore.CommandID != projectionCommandID || *unchangedBefore.ReplacesDefaultFingerprint != "fp-v1" || unchangedAfter.Enabled {
		t.Fatalf("mutação do Diff vazou para a proposta: before=%#v after=%#v", unchangedBefore, unchangedAfter)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	want := binding
	want.Enabled = false
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, want) {
		t.Fatalf("commit aceitou dados mutados do Diff: got=%#v want=%#v", got, want)
	}
}

func TestCommitBindingEnabledRejeitaPropostaDeOutroStore(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	otherStore, err := New(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := otherStore.CommitBindingEnabled(context.Background(), change); !errors.Is(err, ErrInvalid) {
		t.Fatalf("proposta de outro store = %v, esperado ErrInvalid", err)
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, binding) {
		t.Fatalf("proposta estrangeira alterou binding: %#v", got)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, fixture.snapshot.Generations[0].ID), fixture.snapshot.Generations[0].ID, 1)
}

func TestPrepareBindingEnabledRecusasEscopoEOwnerStatusEDados(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*testing.T, *projectionTestFixture, Binding) string
		scope  func(projectionTestFixture) Scope
		enable bool
	}{
		{name: "workspace", scope: func(f projectionTestFixture) Scope {
			workspace := storeTestUUID7(t)
			return Scope{UserID: f.scope.UserID, WorkspaceID: &workspace}
		}, enable: false},
		{name: "binding ausente", setup: func(t *testing.T, _ *projectionTestFixture, _ Binding) string { return storeTestUUID7(t) }, enable: false},
		{name: "binding de outro usuário", setup: func(t *testing.T, f *projectionTestFixture, _ Binding) string {
			foreignUser := storeTestUUID7(t)
			layer := storeTestLayer(t, f.db, foreignUser, nil, "foreign")
			return storeTestBinding(t, f.db, foreignUser, nil, layer, "active").ID
		}, enable: false},
		{name: "no-op", enable: true},
		{name: "needs_review", setup: func(t *testing.T, f *projectionTestFixture, row Binding) string {
			if err := f.db.Model(&Binding{}).Where("id = ?", row.ID).Update("review_status", "needs_review").Error; err != nil {
				t.Fatal(err)
			}
			return row.ID
		}, enable: false},
		{name: "documento inválido", setup: func(t *testing.T, f *projectionTestFixture, row Binding) string {
			if err := f.db.Model(&Binding{}).Where("id = ?", row.ID).Update("trigger_spec", `{"version":2,"code":"KeyK","modifiers":["Control"]}`).Error; err != nil {
				t.Fatal(err)
			}
			return row.ID
		}, enable: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture, row := writeTestBaseline(t)
			bindingID := row.ID
			if tc.setup != nil {
				bindingID = tc.setup(t, &fixture, row)
			}
			scope := fixture.scope
			if tc.scope != nil {
				scope = tc.scope(fixture)
			}
			_, err := fixture.store.PrepareBindingEnabled(context.Background(), scope, bindingID, tc.enable, fixture.options)
			requireInvalid(t, err)
		})
	}
}

func TestCommitBindingEnabledRejeitaGeracaoMaxIntSemOverflow(t *testing.T) {
	fixture, binding := writeTestBaseline(t)
	generation := fixture.snapshot.Generations[0]
	if err := fixture.db.Model(&Generation{}).Where("id = ?", generation.ID).Update("generation", math.MaxInt64).Error; err != nil {
		t.Fatal(err)
	}
	change, err := fixture.store.PrepareBindingEnabled(context.Background(), fixture.scope, binding.ID, false, fixture.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitBindingEnabled(context.Background(), change); !errors.Is(err, ErrInvalid) {
		t.Fatalf("geração máxima = %v, esperado ErrInvalid", err)
	}
	if got := writeTestBindingRow(t, fixture.db, binding.ID); !reflect.DeepEqual(got, binding) {
		t.Fatalf("binding alterado com geração máxima: %#v", got)
	}
	requireGeneration(t, writeTestGenerationRow(t, fixture.db, generation.ID), generation.ID, math.MaxInt64)
}
