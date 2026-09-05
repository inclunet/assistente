package database

import (
	"errors"
	"fmt"
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

// TestRunMigrationList_RejectsOutOfOrder valida a checagem de ordem: listas com
// Version não estritamente crescente (fora de ordem ou duplicada) são rejeitadas
// antes de aplicar qualquer migração.
func TestRunMigrationList_RejectsOutOfOrder(t *testing.T) {
	database := newMigratorTestDB(t)

	ran := false
	outOfOrder := []migration{
		{Version: 2, Name: "two", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { ran = true; return nil }},
		{Version: 1, Name: "one", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { ran = true; return nil }},
	}
	if err := runMigrationList(database, phasePreAutoMigrate, outOfOrder); err == nil {
		t.Fatal("esperava erro para lista fora de ordem")
	}
	if ran {
		t.Fatal("nenhuma migração deveria rodar quando a lista é inválida")
	}

	dup := []migration{
		{Version: 1, Name: "one", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { return nil }},
		{Version: 1, Name: "dup", Phase: phasePreAutoMigrate, Run: func(*gorm.DB) error { return nil }},
	}
	if err := runMigrationList(database, phasePreAutoMigrate, dup); err == nil {
		t.Fatal("esperava erro para versão duplicada")
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

// TestRunMigrations_DeferredNotRecordedAndRetries valida o caminho de
// adiamento: uma migração que retorna errMigrationDeferred NÃO aborta o boot,
// NÃO é registrada e as migrações seguintes continuam rodando — e numa
// execução posterior ela é retentada (e registrada quando finalmente sucede).
func TestRunMigrations_DeferredNotRecordedAndRetries(t *testing.T) {
	database := newMigratorTestDB(t)

	attempts := 0
	ranAfter := false
	drop := func(*gorm.DB) error {
		attempts++
		if attempts == 1 {
			return fmt.Errorf("drop falhou (%v): %w", errors.New("no such feature"), errMigrationDeferred)
		}
		return nil
	}
	migs := []migration{
		{Version: 1, Name: "deferred", Phase: phasePostAutoMigrate, Run: drop},
		{Version: 2, Name: "after", Phase: phasePostAutoMigrate, Run: func(*gorm.DB) error { ranAfter = true; return nil }},
	}

	// 1ª execução: v1 adia (não registra) mas não aborta; v2 segue normalmente.
	if err := runMigrationList(database, phasePostAutoMigrate, migs); err != nil {
		t.Fatalf("adiamento não deveria abortar o boot: %v", err)
	}
	if !ranAfter {
		t.Fatal("migração seguinte deveria rodar mesmo com a anterior adiada")
	}
	if got := schemaMigrationRows(t, database); len(got) != 1 || got[0] != 2 {
		t.Fatalf("apenas v2 deveria estar registrada após adiamento, tenho %v", got)
	}
	// Com v1 pendente e v2 registrada há um buraco no prefixo: user_version
	// reflete a maior versão CONTÍGUA (0), não MAX(version)=2.
	if uv := userVersion(t, database); uv != 0 {
		t.Fatalf("user_version deveria ser 0 (v1 pendente cria buraco), tenho %d", uv)
	}

	// 2ª execução: v1 é retentada (agora sucede) e registrada.
	if err := runMigrationList(database, phasePostAutoMigrate, migs); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("v1 deveria ter sido retentada (2 tentativas), tenho %d", attempts)
	}
	if got := schemaMigrationRows(t, database); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("v1 e v2 deveriam estar registradas após o retry, tenho %v", got)
	}
	// Buraco resolvido: prefixo contíguo agora é 1..2, user_version avança p/ 2.
	if uv := userVersion(t, database); uv != 2 {
		t.Fatalf("user_version deveria ser 2 após resolver o adiamento, tenho %d", uv)
	}
}

// TestDedupCredentialEntries_DefersUntilUserIDExists valida que o dedup da v2
// NÃO é dado como aplicado quando a tabela legada ainda não tem a coluna
// user_id (pré-AEP-0052): ele se adia, e só deduplica de fato depois que o
// AutoMigrate adiciona a coluna — destravando o caso em que o índice unique
// falharia por duplicatas recém-expostas.
func TestDedupCredentialEntries_DefersUntilUserIDExists(t *testing.T) {
	prev := db
	database := newMigratorTestDB(t)
	db = database
	t.Cleanup(func() { db = prev })

	// Tabela legada pré-AEP-0052: sem user_id, com pattern duplicado.
	if err := db.Exec(`CREATE TABLE credential_entries (id TEXT PRIMARY KEY, pattern TEXT, updated_at DATETIME)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec(`INSERT INTO credential_entries (id, pattern, updated_at) VALUES ('a','api.openai.com','2026-01-01'),('b','api.openai.com','2026-01-02')`).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Sem user_id → adia (não deduplica e não deve ser registrada).
	if err := dedupCredentialEntriesBeforeMigrate(); !errors.Is(err, errMigrationDeferred) {
		t.Fatalf("esperava errMigrationDeferred sem user_id, tenho %v", err)
	}

	// Simula o AutoMigrate adicionando user_id (todas as linhas no mesmo dono).
	if err := db.Exec(`ALTER TABLE credential_entries ADD COLUMN user_id TEXT DEFAULT ''`).Error; err != nil {
		t.Fatalf("add user_id: %v", err)
	}

	// Agora com user_id → dedup real, sem erro.
	if err := dedupCredentialEntriesBeforeMigrate(); err != nil {
		t.Fatalf("dedup com user_id: %v", err)
	}
	var count int64
	if err := db.Raw(`SELECT count(*) FROM credential_entries`).Scan(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("dedup deveria manter 1 linha por (user_id, pattern), tenho %d", count)
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
		&CredentialEntry{}, &CredentialKeyWrap{}, &LLMProvider{}, &ACPSession{}, &TaskListWorkflow{},
		&TaskList{}, &Task{}, &TaskNote{}, &MCPServer{}, &MCPServerLog{}, &ToolCatalog{},
		&Tag{}, &TagAssignment{}, &JobPipeline{}, &Job{}, &JobTrigger{}, &JobRun{},
		&JobEvent{}, &JobRunEvent{}, &ToolInvocation{}, &SubAgentRun{},
		&ChannelResponsePending{}, &Channel{}, &ChannelContact{}, &ChannelContactConversation{},
	); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
}

func TestMessagePinMigrationAddsColumnAndListingIndex(t *testing.T) {
	database := newMigratorTestDB(t)
	if err := database.AutoMigrate(&Conversation{}, &ChatMessage{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	if !database.Migrator().HasColumn(&ChatMessage{}, "Pinned") {
		t.Fatal("AutoMigrate não criou chat_messages.pinned")
	}

	migration := schemaMigrations[len(schemaMigrations)-1]
	if migration.Version != 13 || migration.Name != "chat_message_pinned_index" {
		t.Fatalf("última migração inesperada: v%d %s", migration.Version, migration.Name)
	}
	if err := migration.Run(database); err != nil {
		t.Fatalf("aplicar migração de pin: %v", err)
	}
	if !database.Migrator().HasIndex(&ChatMessage{}, "idx_chat_messages_conversation_pinned_created") {
		t.Fatal("índice de mensagens fixadas não foi criado")
	}
	if err := migration.Run(database); err != nil {
		t.Fatalf("migração de pin não é idempotente: %v", err)
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
