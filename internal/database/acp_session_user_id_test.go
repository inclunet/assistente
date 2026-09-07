package database

import (
	"testing"

	"gorm.io/gorm"
)

// criaAcpSessionsLegado reproduz o `acp_sessions` como o AutoMigrate o criava
// antes de `user_id` virar NOT NULL DEFAULT '' (AEP-0084 D12): coluna nula e um
// índice unique que, por causa disso, não vale para as linhas sem dono — no
// SQLite dois NULL não se comparam iguais.
func criaAcpSessionsLegado(t *testing.T, database *gorm.DB) {
	t.Helper()
	ddl := []string{
		"CREATE TABLE `acp_sessions` (`id` text,`created_at` datetime,`updated_at` datetime,`user_id` text,`conversation_id` text NOT NULL,`provider_id` text NOT NULL,`session_id` text NOT NULL,`prompt_prefix_hash` text,`cwd` text,PRIMARY KEY (`id`))",
		"CREATE UNIQUE INDEX `idx_acp_sessions_scope` ON `acp_sessions`(`user_id`,`conversation_id`,`provider_id`)",
		"CREATE INDEX `idx_acp_sessions_conversation_id` ON `acp_sessions`(`conversation_id`)",
	}
	for _, stmt := range ddl {
		if err := database.Exec(stmt).Error; err != nil {
			t.Fatalf("criar schema legado de acp_sessions: %v", err)
		}
	}
}

// insereSessaoLegada grava direto no schema antigo. `dono` é `nil` para a linha
// com `user_id` NULL, que é o caso que a migração precisa resolver.
func insereSessaoLegada(t *testing.T, database *gorm.DB, id string, dono any, conversa, provider, sessao, atualizadaEm string) {
	t.Helper()
	err := database.Exec(
		`INSERT INTO acp_sessions (id, user_id, conversation_id, provider_id, session_id, prompt_prefix_hash, cwd, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, '', '/projeto', ?, ?)`,
		id, dono, conversa, provider, sessao, atualizadaEm, atualizadaEm,
	).Error
	if err != nil {
		t.Fatalf("inserir sessão legada %q: %v", id, err)
	}
}

// bootaSobre roda o mesmo trio do Init(): migrações pré-AutoMigrate,
// AutoMigrate de todos os models e migrações pós-AutoMigrate. Várias migrações
// falam com a global `db`, então ela é apontada para o banco do teste e
// restaurada depois.
func bootaSobre(t *testing.T, database *gorm.DB) error {
	t.Helper()
	anterior := db
	db = database
	t.Cleanup(func() { db = anterior })

	if err := runMigrations(database, phasePreAutoMigrate); err != nil {
		return err
	}
	fullAutoMigrate(t, database)
	return runMigrations(database, phasePostAutoMigrate)
}

type sessaoLida struct {
	ID        string
	UserID    string
	SessionID string
}

func leSessoes(t *testing.T, database *gorm.DB) []sessaoLida {
	t.Helper()
	var linhas []sessaoLida
	if err := database.Raw(`SELECT id, user_id, session_id FROM acp_sessions ORDER BY id`).Scan(&linhas).Error; err != nil {
		t.Fatalf("ler acp_sessions: %v", err)
	}
	return linhas
}

func contaSessoesNulas(t *testing.T, database *gorm.DB) int64 {
	t.Helper()
	var total int64
	if err := database.Raw(`SELECT count(*) FROM acp_sessions WHERE user_id IS NULL`).Scan(&total).Error; err != nil {
		t.Fatalf("contar sessões com user_id nulo: %v", err)
	}
	return total
}

// TestBaseAntigaComSessoesSemDonoContinuaAbrindo é o caso que a migração
// existe para atender: um banco criado antes da mudança, com `user_id` nulo,
// precisa passar pelo boot inteiro. Sem a normalização prévia o AutoMigrate
// recria a tabela para aplicar o NOT NULL e a cópia das linhas morre em
// "NOT NULL constraint failed: acp_sessions__temp.user_id" — o app não sobe.
func TestBaseAntigaComSessoesSemDonoContinuaAbrindo(t *testing.T) {
	database := newMigratorTestDB(t)
	criaAcpSessionsLegado(t, database)

	// Duas linhas nulas para a mesma conversa e provider: hoje o índice unique
	// deixa passar, porque NULL não colide com NULL. Vence a mais recente.
	insereSessaoLegada(t, database, "a1", nil, "conv-1", "cursor", "sess-velha", "2026-01-01")
	insereSessaoLegada(t, database, "a2", nil, "conv-1", "cursor", "sess-nova", "2026-02-01")
	// Nula disputando com uma que já está em string vazia: a vazia manda, por
	// ser a forma que o app grava hoje.
	insereSessaoLegada(t, database, "b1", "", "conv-2", "cursor", "sess-vazia", "2026-01-01")
	insereSessaoLegada(t, database, "b2", nil, "conv-2", "cursor", "sess-nula", "2026-03-01")
	// Nula sem disputa: sobrevive e só troca de valor.
	insereSessaoLegada(t, database, "c1", nil, "conv-3", "cursor", "sess-sozinha", "2026-01-01")
	// Sessão de gente de verdade: a migração não pode encostar.
	insereSessaoLegada(t, database, "d1", "user-ana", "conv-1", "cursor", "sess-da-ana", "2026-01-01")

	if err := bootaSobre(t, database); err != nil {
		t.Fatalf("boot sobre base antiga: %v", err)
	}

	if nulas := contaSessoesNulas(t, database); nulas != 0 {
		t.Fatalf("%d linhas continuaram com user_id nulo", nulas)
	}

	esperado := []sessaoLida{
		{ID: "a2", UserID: "", SessionID: "sess-nova"},
		{ID: "b1", UserID: "", SessionID: "sess-vazia"},
		{ID: "c1", UserID: "", SessionID: "sess-sozinha"},
		{ID: "d1", UserID: "user-ana", SessionID: "sess-da-ana"},
	}
	lidas := leSessoes(t, database)
	if len(lidas) != len(esperado) {
		t.Fatalf("sobraram %d linhas, esperado %d: %+v", len(lidas), len(esperado), lidas)
	}
	for i, quero := range esperado {
		if lidas[i] != quero {
			t.Errorf("linha %d = %+v, esperado %+v", i, lidas[i], quero)
		}
	}

	// A base migrada precisa continuar utilizável, não apenas abrir.
	nova := ACPSession{
		UserID:         "user-ana",
		ConversationID: "conv-9",
		ProviderID:     "cursor",
		SessionID:      "sess-depois-da-migracao",
		Cwd:            "/projeto",
	}
	if err := database.Create(&nova).Error; err != nil {
		t.Fatalf("gravar sessão depois da migração: %v", err)
	}
	var relida ACPSession
	if err := database.Where("conversation_id = ? AND provider_id = ?", "conv-9", "cursor").First(&relida).Error; err != nil {
		t.Fatalf("ler sessão depois da migração: %v", err)
	}
	if relida.SessionID != nova.SessionID {
		t.Fatalf("sessão relida = %q", relida.SessionID)
	}
}

// TestMigracaoDeSessaoSemDonoNaoRefazTrabalhoNoSegundoBoot guarda a
// idempotência: a v10 é registrada em schema_migrations e o boot seguinte não
// pode mexer nas linhas de novo nem falhar.
func TestMigracaoDeSessaoSemDonoNaoRefazTrabalhoNoSegundoBoot(t *testing.T) {
	database := newMigratorTestDB(t)
	criaAcpSessionsLegado(t, database)
	insereSessaoLegada(t, database, "a1", nil, "conv-1", "cursor", "sess-1", "2026-01-01")

	if err := bootaSobre(t, database); err != nil {
		t.Fatalf("primeiro boot: %v", err)
	}
	primeiro := leSessoes(t, database)

	if err := bootaSobre(t, database); err != nil {
		t.Fatalf("segundo boot: %v", err)
	}
	segundo := leSessoes(t, database)

	if len(primeiro) != len(segundo) {
		t.Fatalf("segundo boot mexeu nas linhas: %+v → %+v", primeiro, segundo)
	}
	for i := range primeiro {
		if primeiro[i] != segundo[i] {
			t.Fatalf("linha %d mudou no segundo boot: %+v → %+v", i, primeiro[i], segundo[i])
		}
	}

	registradas := schemaMigrationRows(t, database)
	vezes := 0
	for _, v := range registradas {
		if v == 10 {
			vezes++
		}
	}
	if vezes != 1 {
		t.Fatalf("v10 registrada %d vezes em %v, esperado 1", vezes, registradas)
	}
}

// TestBancoNovoNaoPrecisaDaMigracaoDeSessaoSemDono cobre o outro extremo: sem
// a tabela, a migração é noop e ainda assim fica registrada — um banco novo já
// nasce no formato certo.
func TestBancoNovoNaoPrecisaDaMigracaoDeSessaoSemDono(t *testing.T) {
	database := newMigratorTestDB(t)
	if err := normalizeACPSessionUserID(database); err != nil {
		t.Fatalf("migração sem a tabela deveria ser noop: %v", err)
	}

	if err := bootaSobre(t, database); err != nil {
		t.Fatalf("boot em banco novo: %v", err)
	}
	if linhas := leSessoes(t, database); len(linhas) != 0 {
		t.Fatalf("banco novo nasceu com linhas: %+v", linhas)
	}
}

// TestSessaoSemDonoGravaStringVaziaEmVezDeNulo descreve o que a coluna passa a
// garantir: sem dono é string vazia, e string vazia disputa o índice unique
// como qualquer outro valor — dois vínculos vivos para a mesma conversa e
// provider deixariam o app sem saber qual sessão é a do agente.
func TestSessaoSemDonoGravaStringVaziaEmVezDeNulo(t *testing.T) {
	database := newMigratorTestDB(t)
	if err := database.AutoMigrate(&ACPSession{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	semDono := ACPSession{ConversationID: "conv-1", ProviderID: "cursor", SessionID: "sess-1"}
	if err := database.Create(&semDono).Error; err != nil {
		t.Fatalf("gravar sessão sem dono: %v", err)
	}
	if nulas := contaSessoesNulas(t, database); nulas != 0 {
		t.Fatalf("sessão sem dono gravou %d linhas com user_id nulo", nulas)
	}

	repetida := ACPSession{ConversationID: "conv-1", ProviderID: "cursor", SessionID: "sess-2"}
	if err := database.Create(&repetida).Error; err == nil {
		t.Fatal("segundo vínculo sem dono para a mesma conversa e provider foi aceito")
	}

	// E o NULL deixa de entrar pela porta dos fundos do SQL cru.
	err := database.Exec(
		`INSERT INTO acp_sessions (id, user_id, conversation_id, provider_id, session_id, created_at, updated_at)
		 VALUES ('cru', NULL, 'conv-2', 'cursor', 'sess-3', '2026-01-01', '2026-01-01')`,
	).Error
	if err == nil {
		t.Fatal("a coluna user_id ainda aceita NULL")
	}
}
