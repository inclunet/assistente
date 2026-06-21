package database

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// newMigratorTestDB abre um banco SQLite em memória isolado, SEM tocar na
// global `db`. Usado pelos testes do mecanismo de versionamento que operam
// apenas via parâmetro (runMigrationList recebe o *gorm.DB explicitamente).
func newMigratorTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, derr := database.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})
	return database
}

// schemaMigrationRows lê todas as linhas de schema_migrations ordenadas por
// versão.
func schemaMigrationRows(t *testing.T, database *gorm.DB) []int {
	t.Helper()
	var versions []int
	if err := database.Raw(`SELECT version FROM schema_migrations ORDER BY version`).Scan(&versions).Error; err != nil {
		t.Fatalf("ler schema_migrations: %v", err)
	}
	return versions
}

func userVersion(t *testing.T, database *gorm.DB) int {
	t.Helper()
	var v int
	if err := database.Raw(`PRAGMA user_version`).Scan(&v).Error; err != nil {
		t.Fatalf("ler user_version: %v", err)
	}
	return v
}

// TestRunMigrations_AppliesInOrder valida que as migrações pendentes da fase
// rodam em ordem crescente de versão e são registradas.
func TestRunMigrations_AppliesInOrder(t *testing.T) {
	database := newMigratorTestDB(t)

	var order []int
	migs := []migration{
		{Version: 1, Name: "one", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { order = append(order, 1); return nil }},
		{Version: 2, Name: "two", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { order = append(order, 2); return nil }},
		{Version: 3, Name: "three", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { order = append(order, 3); return nil }},
	}

	if err := runMigrationList(database, phasePreAutoMigrate, migs); err != nil {
		t.Fatalf("runMigrationList: %v", err)
	}

	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("ordem de aplicação inesperada: %v", order)
	}
	got := schemaMigrationRows(t, database)
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("schema_migrations inesperado: %v", got)
	}
	if uv := userVersion(t, database); uv != 3 {
		t.Fatalf("user_version esperado 3, tenho %d", uv)
	}
}

// TestRunMigrations_Idempotent valida que rodar de novo não reexecuta
// migrações já registradas.
func TestRunMigrations_Idempotent(t *testing.T) {
	database := newMigratorTestDB(t)

	runs := map[int]int{}
	migs := []migration{
		{Version: 1, Name: "one", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { runs[1]++; return nil }},
		{Version: 2, Name: "two", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { runs[2]++; return nil }},
	}

	for i := 0; i < 3; i++ {
		if err := runMigrationList(database, phasePreAutoMigrate, migs); err != nil {
			t.Fatalf("runMigrationList (passe %d): %v", i, err)
		}
	}

	if runs[1] != 1 || runs[2] != 1 {
		t.Fatalf("migrações reexecutadas: %v (esperado cada uma 1x)", runs)
	}
	if got := schemaMigrationRows(t, database); len(got) != 2 {
		t.Fatalf("schema_migrations deveria ter 2 linhas, tem %v", got)
	}
}

// TestRunMigrations_PhaseFiltering valida que só a fase pedida roda.
func TestRunMigrations_PhaseFiltering(t *testing.T) {
	database := newMigratorTestDB(t)

	var ran []int
	migs := []migration{
		{Version: 1, Name: "pre", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { ran = append(ran, 1); return nil }},
		{Version: 2, Name: "post", Phase: phasePostAutoMigrate, Run: func(*gorm.DB) error { ran = append(ran, 2); return nil }},
	}

	if err := runMigrationList(database, phasePreAutoMigrate, migs); err != nil {
		t.Fatalf("pre: %v", err)
	}
	if len(ran) != 1 || ran[0] != 1 {
		t.Fatalf("apenas a migração pré deveria rodar, ran=%v", ran)
	}
	if got := schemaMigrationRows(t, database); len(got) != 1 || got[0] != 1 {
		t.Fatalf("schema_migrations após pre: %v", got)
	}

	if err := runMigrationList(database, phasePostAutoMigrate, migs); err != nil {
		t.Fatalf("post: %v", err)
	}
	if len(ran) != 2 || ran[1] != 2 {
		t.Fatalf("migração pós deveria rodar depois, ran=%v", ran)
	}
}

// TestRunMigrations_DetectAppliedStampsWithoutRunning valida o caminho de
// adoção de banco legado: quando DetectApplied retorna true, a migração é
// marcada como aplicada sem executar Run (essencial para migrações pesadas
// como a UUIDv7 em bancos já convertidos).
func TestRunMigrations_DetectAppliedStampsWithoutRunning(t *testing.T) {
	database := newMigratorTestDB(t)

	ranHeavy := false
	migs := []migration{
		{
			Version:       1,
			Name:          "heavy",
			Phase:         phasePreAutoMigrate,
			Run:           func(*gorm.DB) error { ranHeavy = true; return nil },
			DetectApplied: func(*gorm.DB) (bool, error) { return true, nil },
		},
	}

	if err := runMigrationList(database, phasePreAutoMigrate, migs); err != nil {
		t.Fatalf("runMigrationList: %v", err)
	}

	if ranHeavy {
		t.Fatal("migração pesada NÃO deveria ter rodado quando DetectApplied=true")
	}
	if got := schemaMigrationRows(t, database); len(got) != 1 || got[0] != 1 {
		t.Fatalf("migração deveria ter sido registrada mesmo sem rodar: %v", got)
	}
}

// TestRunMigrations_DetectAppliedFalseRuns valida que DetectApplied=false faz
// a migração rodar normalmente.
func TestRunMigrations_DetectAppliedFalseRuns(t *testing.T) {
	database := newMigratorTestDB(t)

	ran := false
	migs := []migration{
		{
			Version:       1,
			Name:          "m",
			Phase:         phasePreAutoMigrate,
			Run:           func(*gorm.DB) error { ran = true; return nil },
			DetectApplied: func(*gorm.DB) (bool, error) { return false, nil },
		},
	}

	if err := runMigrationList(database, phasePreAutoMigrate, migs); err != nil {
		t.Fatalf("runMigrationList: %v", err)
	}
	if !ran {
		t.Fatal("migração deveria ter rodado quando DetectApplied=false")
	}
}

// TestRunMigrations_StopsOnError valida que um erro aborta a sequência: as
// migrações seguintes não rodam e a que falhou não é registrada.
func TestRunMigrations_StopsOnError(t *testing.T) {
	database := newMigratorTestDB(t)

	boom := errors.New("boom")
	ranThird := false
	migs := []migration{
		{Version: 1, Name: "ok", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { return nil }},
		{Version: 2, Name: "fail", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { return boom }},
		{Version: 3, Name: "after", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { ranThird = true; return nil }},
	}

	err := runMigrationList(database, phasePreAutoMigrate, migs)
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("esperava erro encapsulando boom, tenho %v", err)
	}
	if ranThird {
		t.Fatal("migração após a falha não deveria rodar")
	}
	got := schemaMigrationRows(t, database)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("apenas v1 deveria estar registrada, tenho %v", got)
	}
}

// TestSchemaMigrationsRegistry_IsConsistent guarda contra erros de edição da
// lista global: versões estritamente crescentes/únicas, nomes não vazios e Run
// não-nil.
func TestSchemaMigrationsRegistry_IsConsistent(t *testing.T) {
	seen := map[int]bool{}
	prev := 0
	for i, m := range schemaMigrations {
		if m.Version <= 0 {
			t.Fatalf("migração %d com Version inválida: %d", i, m.Version)
		}
		if m.Version <= prev {
			t.Fatalf("versões devem ser estritamente crescentes: v%d após v%d", m.Version, prev)
		}
		if seen[m.Version] {
			t.Fatalf("versão duplicada: %d", m.Version)
		}
		if m.Name == "" {
			t.Fatalf("migração v%d sem Name", m.Version)
		}
		if m.Run == nil {
			t.Fatalf("migração v%d (%s) sem Run", m.Version, m.Name)
		}
		seen[m.Version] = true
		prev = m.Version
	}
}

// TestUUIDMigrationAlreadyApplied cobre os três estados detectados pela v1.
func TestUUIDMigrationAlreadyApplied(t *testing.T) {
	t.Run("banco novo sem tabela → não aplicada (deixa rodar como no-op)", func(t *testing.T) {
		database := newMigratorTestDB(t)
		applied, err := uuidMigrationAlreadyApplied(database)
		if err != nil {
			t.Fatalf("detect: %v", err)
		}
		if applied {
			t.Fatal("sem tabela conversations não deveria ser considerada aplicada")
		}
	})

	t.Run("legado com id INTEGER → não aplicada (precisa converter)", func(t *testing.T) {
		database := newMigratorTestDB(t)
		if err := database.Exec(`CREATE TABLE conversations (id INTEGER PRIMARY KEY, title TEXT)`).Error; err != nil {
			t.Fatalf("create legacy table: %v", err)
		}
		applied, err := uuidMigrationAlreadyApplied(database)
		if err != nil {
			t.Fatalf("detect: %v", err)
		}
		if applied {
			t.Fatal("id INTEGER deveria indicar migração ainda necessária")
		}
	})

	t.Run("já em UUID com id TEXT → aplicada", func(t *testing.T) {
		database := newMigratorTestDB(t)
		if err := database.Exec(`CREATE TABLE conversations (id TEXT PRIMARY KEY, title TEXT)`).Error; err != nil {
			t.Fatalf("create uuid table: %v", err)
		}
		applied, err := uuidMigrationAlreadyApplied(database)
		if err != nil {
			t.Fatalf("detect: %v", err)
		}
		if !applied {
			t.Fatal("id TEXT deveria indicar migração já aplicada")
		}
	})
}

// fullAutoMigrate replica o conjunto de models migrados em Init(), para os
// testes de integração do registro real.
func fullAutoMigrate(t *testing.T, database *gorm.DB) {
	t.Helper()
	if err := database.AutoMigrate(
		&User{}, &Session{}, &Conversation{}, &ChatMessage{}, &MemoryRecord{},
		&CredentialEntry{}, &CredentialKeyWrap{}, &LLMProvider{}, &TaskListWorkflow{},
		&TaskList{}, &Task{}, &TaskNote{}, &MCPServer{}, &MCPServerLog{}, &ToolCatalog{},
		&Tag{}, &TagAssignment{}, &JobPipeline{}, &Job{}, &JobTrigger{}, &JobRun{},
		&JobEvent{}, &JobRunEvent{}, &ToolInvocation{}, &SubAgentRun{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
}

// TestRealRegistry_FreshDBAppliesAllAndIsIdempotent exercita o registro real
// (schemaMigrations) no fluxo de Init() sobre um banco novo: pré-AutoMigrate,
// AutoMigrate, pós-AutoMigrate. Verifica que todas as versões são aplicadas e
// que um segundo boot não reexecuta nada.
//
// As migrações reais usam a global `db`, então este teste a configura (e
// limpa no cleanup) para espelhar o ambiente de Init().
func TestRealRegistry_FreshDBAppliesAllAndIsIdempotent(t *testing.T) {
	prev := db
	database := newMigratorTestDB(t)
	db = database
	t.Cleanup(func() { db = prev })

	if err := runMigrations(db, phasePreAutoMigrate); err != nil {
		t.Fatalf("pré-AutoMigrate: %v", err)
	}
	fullAutoMigrate(t, db)
	if err := runMigrations(db, phasePostAutoMigrate); err != nil {
		t.Fatalf("pós-AutoMigrate: %v", err)
	}

	got := schemaMigrationRows(t, db)
	if len(got) != len(schemaMigrations) {
		t.Fatalf("esperava %d migrações registradas, tenho %d (%v)", len(schemaMigrations), len(got), got)
	}
	for i, m := range schemaMigrations {
		if got[i] != m.Version {
			t.Fatalf("versão registrada na posição %d: esperava %d, tenho %d", i, m.Version, got[i])
		}
	}
	if uv := userVersion(t, db); uv != schemaMigrations[len(schemaMigrations)-1].Version {
		t.Fatalf("user_version esperado %d, tenho %d", schemaMigrations[len(schemaMigrations)-1].Version, uv)
	}

	// Segundo "boot": nada deve ser reexecutado nem duplicado.
	if err := runMigrations(db, phasePreAutoMigrate); err != nil {
		t.Fatalf("pré-AutoMigrate (2º boot): %v", err)
	}
	if err := runMigrations(db, phasePostAutoMigrate); err != nil {
		t.Fatalf("pós-AutoMigrate (2º boot): %v", err)
	}
	if got2 := schemaMigrationRows(t, db); len(got2) != len(schemaMigrations) {
		t.Fatalf("2º boot não deveria adicionar linhas: %v", got2)
	}
}

// TestRealRegistry_PreStampedMigrationDoesNotRerun valida que, num banco onde
// uma versão JÁ consta em schema_migrations, ela não roda de novo — simulando
// a adoção de um banco que já passou pela migração no fluxo antigo.
func TestRealRegistry_PreStampedMigrationDoesNotRerun(t *testing.T) {
	database := newMigratorTestDB(t)

	if err := ensureSchemaMigrationsTable(database); err != nil {
		t.Fatalf("ensure table: %v", err)
	}

	ran := false
	migs := []migration{
		{Version: 1, Name: "already", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { ran = true; return nil }},
	}
	// Pré-carimba a v1 como aplicada.
	if err := recordMigration(database, migs[0]); err != nil {
		t.Fatalf("record: %v", err)
	}

	if err := runMigrationList(database, phasePreAutoMigrate, migs); err != nil {
		t.Fatalf("runMigrationList: %v", err)
	}
	if ran {
		t.Fatal("migração pré-carimbada não deveria reexecutar")
	}
}
