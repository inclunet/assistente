package commandconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func storeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := t.TempDir() + "/commandconfig.db"
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter conexão sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatalf("migrar schema de teste: %v", err)
	}
	return db
}

func storeTestUUID7(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("gerar UUIDv7: %v", err)
	}
	return id.String()
}

func storeTestPtr(value string) *string { return &value }

func storeTestGeneration(t *testing.T, db *gorm.DB, user string, workspace *string, generation int64) Generation {
	t.Helper()
	row := Generation{ID: storeTestUUID7(t), UserID: user, WorkspaceID: workspace, Generation: generation, UpdatedAt: time.Now().UTC()}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("inserir geração: %v", err)
	}
	return row
}

func storeTestLayer(t *testing.T, db *gorm.DB, user string, workspace *string, name string) Layer {
	t.Helper()
	now := time.Now().UTC()
	row := Layer{ID: storeTestUUID7(t), UserID: user, WorkspaceID: workspace, Name: name, Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 1, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("inserir camada: %v", err)
	}
	return row
}

func storeTestBinding(t *testing.T, db *gorm.DB, user string, workspace *string, layer Layer, status string) Binding {
	t.Helper()
	row := Binding{ID: storeTestUUID7(t), UserID: user, WorkspaceID: workspace, LayerRefKind: "user", LayerRef: layer.ID, TriggerType: "hotkey", TriggerSpec: "{}", Arguments: "{}", Condition: "{}", Effect: "execute", CommandID: storeTestPtr("workspace.tab.new"), Enabled: false, Source: "user", ResolutionPriority: 1, ReviewStatus: status, Presentation: "{}"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("inserir binding: %v", err)
	}
	return row
}

func TestStoreLoadEscopoIsolamentoEStatus(t *testing.T) {
	db := storeTestDB(t)
	store, _ := New(db)
	user1, user2, workspace1, workspace2 := storeTestUUID7(t), storeTestUUID7(t), storeTestUUID7(t), storeTestUUID7(t)
	global := storeTestLayer(t, db, user1, nil, "global")
	local := storeTestLayer(t, db, user1, storeTestPtr(workspace1), "local")
	storeTestLayer(t, db, user1, storeTestPtr(workspace2), "outro-workspace")
	storeTestLayer(t, db, user2, nil, "outro-usuario")
	globalBinding := storeTestBinding(t, db, user1, nil, global, "needs_review")
	localBinding := storeTestBinding(t, db, user1, storeTestPtr(workspace1), local, "active")
	storeTestGeneration(t, db, user1, nil, 7)
	storeTestGeneration(t, db, user1, storeTestPtr(workspace1), 11)
	storeTestGeneration(t, db, user2, nil, 3)

	snapshot, err := store.Load(context.Background(), Scope{UserID: user1, WorkspaceID: storeTestPtr(workspace1)})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(snapshot.Layers) != 2 || len(snapshot.Bindings) != 2 || len(snapshot.Generations) != 2 {
		t.Fatalf("escopo incorreto: camadas=%d bindings=%d gerações=%d", len(snapshot.Layers), len(snapshot.Bindings), len(snapshot.Generations))
	}
	byID := map[string]Binding{}
	for _, binding := range snapshot.Bindings {
		byID[binding.ID] = binding
	}
	if byID[globalBinding.ID].ReviewStatus != "needs_review" || byID[localBinding.ID].ReviewStatus != "active" {
		t.Fatalf("status alterado pelo loader: %#v", byID)
	}
	if byID[globalBinding.ID].Enabled || byID[localBinding.ID].Enabled {
		t.Fatalf("Enabled alterado pelo loader: %#v", byID)
	}
	if byID[globalBinding.ID].Arguments != "{}" || byID[globalBinding.ID].Presentation != "{}" {
		t.Fatal("documentos opacos não foram preservados")
	}

	globalSnapshot, err := store.Load(context.Background(), Scope{UserID: user1})
	if err != nil {
		t.Fatalf("Load global: %v", err)
	}
	if len(globalSnapshot.Layers) != 1 || len(globalSnapshot.Bindings) != 1 || len(globalSnapshot.Generations) != 1 {
		t.Fatalf("escopo global incorreto: %#v", globalSnapshot)
	}
}

func TestStoreLoadRejeitaGeracaoCamadaOuBindingInvalido(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, *gorm.DB, string, string, string)
	}{
		{"sem-geracao-global", func(t *testing.T, db *gorm.DB, user, workspace, layer string) {
			storeTestGeneration(t, db, user, storeTestPtr(workspace), 1)
		}},
		{"sem-geracao-local", func(t *testing.T, db *gorm.DB, user, workspace, layer string) {
			storeTestGeneration(t, db, user, nil, 1)
		}},
		{"camada-de-outro-usuario", func(t *testing.T, db *gorm.DB, user, workspace, layer string) {
			storeTestGeneration(t, db, user, nil, 1)
			storeTestGeneration(t, db, user, storeTestPtr(workspace), 1)
			foreign := storeTestLayer(t, db, storeTestUUID7(t), nil, "foreign")
			storeTestBinding(t, db, user, nil, foreign, "active")
		}},
		{"binding-fora-da-camada", func(t *testing.T, db *gorm.DB, user, workspace, layer string) {
			storeTestGeneration(t, db, user, nil, 1)
			storeTestGeneration(t, db, user, storeTestPtr(workspace), 1)
			global := storeTestLayer(t, db, user, nil, "global")
			storeTestBinding(t, db, user, storeTestPtr(workspace), global, "active")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := storeTestDB(t)
			user, workspace := storeTestUUID7(t), storeTestUUID7(t)
			layer := storeTestUUID7(t)
			tc.setup(t, db, user, workspace, layer)
			store, _ := New(db)
			if _, err := store.Load(context.Background(), Scope{UserID: user, WorkspaceID: storeTestPtr(workspace)}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("erro = %v, esperado ErrInvalid", err)
			}
		})
	}
}

func TestStoreLoadNaoEscreveECheckCurrentDetectaAlteracoes(t *testing.T) {
	db := storeTestDB(t)
	store, _ := New(db)
	user, workspace := storeTestUUID7(t), storeTestUUID7(t)
	storeTestGeneration(t, db, user, nil, 1)
	storeTestGeneration(t, db, user, storeTestPtr(workspace), 1)
	before := countRows(t, db, "command_config_generations")
	snapshot, err := store.Load(context.Background(), Scope{UserID: user, WorkspaceID: storeTestPtr(workspace)})
	if err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, "command_config_generations"); got != before {
		t.Fatalf("Load escreveu no banco: antes=%d depois=%d", before, got)
	}
	if err := store.CheckCurrent(context.Background(), snapshot); err != nil {
		t.Fatalf("snapshot recém carregado está stale: %v", err)
	}

	mutateGeneration := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if err := db.Exec(sql, args...).Error; err != nil {
			t.Fatal(err)
		}
		if err := store.CheckCurrent(context.Background(), snapshot); !errors.Is(err, ErrStale) {
			t.Fatalf("CheckCurrent = %v, esperado ErrStale", err)
		}
	}
	mutateGeneration(t, "UPDATE command_config_generations SET generation = generation + 1 WHERE user_id = ? AND workspace_id = ?", user, workspace)
	var refreshErr error
	snapshot, refreshErr = store.Load(context.Background(), Scope{UserID: user, WorkspaceID: storeTestPtr(workspace)})
	if refreshErr != nil {
		t.Fatalf("refresh após incremento: %v", refreshErr)
	}
	mutateGeneration(t, "UPDATE command_config_generations SET id = ? WHERE user_id = ? AND workspace_id = ?", storeTestUUID7(t), user, workspace)
	// Uma remoção torna o snapshot inválido ou stale; ambos são fail-closed.
	if err := db.Exec("DELETE FROM command_config_generations WHERE user_id = ? AND workspace_id IS NULL", user).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.CheckCurrent(context.Background(), snapshot); !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrStale) {
		t.Fatalf("remoção de geração = %v, esperado ErrInvalid ou ErrStale", err)
	}
}

func TestStoreStampPrivadoStoreDiferenteCancelamentoENil(t *testing.T) {
	db := storeTestDB(t)
	store, _ := New(db)
	user := storeTestUUID7(t)
	storeTestGeneration(t, db, user, nil, 1)
	snapshot, err := store.Load(context.Background(), Scope{UserID: user})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Scope.UserID = storeTestUUID7(t)
	snapshot.Generations[0].Generation = 999
	if err := store.CheckCurrent(context.Background(), snapshot); err != nil {
		t.Fatalf("projetor forjou stamp: %v", err)
	}
	if err := db.Exec("UPDATE command_config_generations SET generation = 999 WHERE user_id = ?", user).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.CheckCurrent(context.Background(), snapshot); !errors.Is(err, ErrStale) {
		t.Fatalf("alteração real após mutação pública = %v, esperado ErrStale", err)
	}
	other, _ := New(db)
	if err := other.CheckCurrent(context.Background(), snapshot); !errors.Is(err, ErrInvalid) {
		t.Fatalf("store diferente: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Load(canceled, Scope{UserID: user}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load cancelado: %v", err)
	}
	if err := store.CheckCurrent(canceled, snapshot); !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckCurrent cancelado: %v", err)
	}
	if _, err := New(nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("New(nil): %v", err)
	}
	var nilStore *Store
	if _, err := nilStore.Load(context.Background(), Scope{UserID: user}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil Store Load: %v", err)
	}
	if err := store.CheckCurrent(context.Background(), Snapshot{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("stamp ausente: %v", err)
	}
	if _, err := store.Load(nil, Scope{UserID: user}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("contexto nil: %v", err)
	}
}

func countRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var count int64
	if err := db.Table(table).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}
