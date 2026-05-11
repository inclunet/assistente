package database

import (
	"fmt"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestCredentialLossRepro_PreAEP0052BootPreservesAllCredentials reproduz
// o estado exato do banco do incident de 10/05/2026 e garante que o boot
// AEP-0052 + criação do primeiro admin + AdoptLegacyData não perde
// nenhuma credencial.
//
// Estado de partida (forensic, 16:45 do dia do incidente):
//
//   - tabela `credential_entries` com schema pré-AEP-0052 (sem `user_id`,
//     `id TEXT PRIMARY KEY`).
//   - 14 rows com `token_enc`/`client_secret_enc` populados:
//       * 11 patterns user-scoped (api.*, hosts internos, MCP tokens, …);
//       * 3 patterns instance secrets (`internal-auth:*`).
//   - SEM tabela `users` nem `sessions`.
//
// Sequência simulada do primeiro boot AEP-0052:
//
//   1. dedupCredentialEntriesBeforeMigrate() (noop sem user_id).
//   2. db.AutoMigrate (adiciona user_id, cria
//      ux_credential_entries_user_pattern, cria users/sessions).
//   3. ensureCredentialEntryUserPatternIndex().
//   4. CreateAdminUser (apenas insere row em users).
//   5. AdoptLegacyData(userID) — adota órfãs.
//
// Invariantes verificados:
//   - Nenhuma row é deletada.
//   - Após adoção, as 11 rows user-scoped têm `user_id=admin.ID`.
//   - As 3 rows instance secrets continuam com `user_id=''`.
//   - `token_enc`/`client_secret_enc` permanecem intactos
//     (verifica que o caminho não dropou nem zerou colunas — defesa contra
//     o bug histórico do migrateToUUIDv7 que perdeu dados em 27/04).
func TestCredentialLossRepro_PreAEP0052BootPreservesAllCredentials(t *testing.T) {
	openPreAEP0052DB(t)

	type seed struct {
		id              string
		pattern         string
		tokenEnc        string
		clientSecretEnc string
		isInstance      bool
	}
	seeds := []seed{
		{id: "01", pattern: "api.openai.com", tokenEnc: "tok-openai"},
		{id: "02", pattern: "api.anthropic.com", tokenEnc: "tok-anthropic"},
		{id: "03", pattern: "ist-prod-litellm.nullmplatform.com", tokenEnc: "tok-litellm"},
		{id: "04", pattern: "*.github.com", tokenEnc: "tok-gh-wild"},
		{id: "05", pattern: "github.com", tokenEnc: "tok-gh"},
		{id: "06", pattern: "api.gemini.google.com", tokenEnc: "tok-gemini"},
		{id: "07", pattern: "api.deepseek.com", tokenEnc: "tok-deepseek"},
		{id: "08", pattern: "api.groq.com", tokenEnc: "tok-groq"},
		{id: "09", pattern: "mcp-tokens:glean", tokenEnc: "tok-mcp-glean"},
		{id: "10", pattern: "mcp-tokens:custom", tokenEnc: "tok-mcp-custom"},
		{id: "11", pattern: "mcp-client:glean", clientSecretEnc: "secret-mcp-client"},
		{id: "12", pattern: "internal-auth:refresh-token", tokenEnc: "instance-refresh", isInstance: true},
		{id: "13", pattern: "internal-auth:session", tokenEnc: "instance-session", isInstance: true},
		{id: "14", pattern: "internal-tls:ca", tokenEnc: "instance-tls", isInstance: true},
	}

	for _, s := range seeds {
		if err := db.Exec(
			`INSERT INTO credential_entries
			 (id, pattern, auth_type, token_enc, client_secret_enc, created_at, updated_at)
			 VALUES (?, ?, 'bearer', ?, ?, datetime('now'), datetime('now'))`,
			s.id, s.pattern, s.tokenEnc, s.clientSecretEnc,
		).Error; err != nil {
			t.Fatalf("seed %s: %v", s.pattern, err)
		}
	}

	// 1. dedup pré-migrate (deve ser noop sem user_id na tabela).
	dedupCredentialEntriesBeforeMigrate()
	assertCredentialCount(t, len(seeds), "após dedup pré-migrate")

	// 2. AutoMigrate completo (mesmas tabelas que internal/app/db.go inicializa).
	if err := db.AutoMigrate(
		&User{},
		&Session{},
		&CredentialEntry{},
		&CredentialKeyWrap{},
		&LLMProvider{},
		&Conversation{},
		&ChatMessage{},
		&TaskListWorkflow{},
		&TaskList{},
		&Task{},
		&TaskNote{},
	); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	assertCredentialCount(t, len(seeds), "após AutoMigrate")

	// 3. Garante o índice unique (ux_credential_entries_user_pattern).
	ensureCredentialEntryUserPatternIndex()
	assertCredentialCount(t, len(seeds), "após ensureCredentialEntryUserPatternIndex")

	// Confere que os tokens não foram zerados pelo AutoMigrate (defesa
	// contra o bug histórico de migrateToUUIDv7, comm 5d3d7eb9, que
	// dropou colunas durante a migração de PK em 27/04).
	for _, s := range seeds {
		var got struct {
			TokenEnc        string
			ClientSecretEnc string
		}
		if err := db.Raw(
			"SELECT token_enc, client_secret_enc FROM credential_entries WHERE id = ?", s.id,
		).Scan(&got).Error; err != nil {
			t.Fatalf("read seed %s: %v", s.pattern, err)
		}
		if got.TokenEnc != s.tokenEnc {
			t.Fatalf("token_enc de %s perdido após migrate: got %q want %q", s.pattern, got.TokenEnc, s.tokenEnc)
		}
		if got.ClientSecretEnc != s.clientSecretEnc {
			t.Fatalf("client_secret_enc de %s perdido após migrate: got %q want %q", s.pattern, got.ClientSecretEnc, s.clientSecretEnc)
		}
	}

	// 4. Cria admin (passo análogo a CreateAdminUser do app — só insere user).
	admin := &User{Username: "admin", PasswordHash: "h", Role: UserRoleAdmin, IsActive: true}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	// 5. Adoção (chamado pelo primeiro Login).
	if err := AdoptLegacyData(admin.ID); err != nil {
		t.Fatalf("adopt legacy data: %v", err)
	}

	// Invariantes finais:
	assertCredentialCount(t, len(seeds), "após AdoptLegacyData (zero perda)")

	for _, s := range seeds {
		var entry CredentialEntry
		if err := db.Where("id = ?", s.id).First(&entry).Error; err != nil {
			t.Fatalf("credencial perdida no boot AEP-0052: id=%s pattern=%s err=%v", s.id, s.pattern, err)
		}
		if s.isInstance {
			if entry.UserID != "" {
				t.Fatalf("instance secret %q deveria continuar com user_id='', got %q", s.pattern, entry.UserID)
			}
		} else {
			if entry.UserID != admin.ID {
				t.Fatalf("credencial %q deveria ter sido adotada pelo admin, got user_id=%q", s.pattern, entry.UserID)
			}
		}
		if entry.TokenEnc != s.tokenEnc {
			t.Fatalf("token_enc de %q corrompido após boot: got %q want %q", s.pattern, entry.TokenEnc, s.tokenEnc)
		}
		if entry.ClientSecretEnc != s.clientSecretEnc {
			t.Fatalf("client_secret_enc de %q corrompido após boot: got %q want %q", s.pattern, entry.ClientSecretEnc, s.clientSecretEnc)
		}
	}

	// Adoção é idempotente — uma segunda chamada não deve gerar nem
	// duplicar nem apagar nada.
	if err := AdoptLegacyData(admin.ID); err != nil {
		t.Fatalf("adopt segunda passagem: %v", err)
	}
	assertCredentialCount(t, len(seeds), "após segunda passagem de AdoptLegacyData")
}

// TestCredentialLossRepro_AdoptLegacyDoesNotDropClaimedRows blinda o
// invariante "se há órfã + claimed pro mesmo pattern, sobra a claimed".
// Era exatamente o ponto onde AdoptLegacyData abortava antes do fix
// b72060ae — a transação inteira rolava back e a base ficava em estado
// inconsistente (User criado, Login falhando, "admin já existe" no
// retry).
func TestCredentialLossRepro_AdoptLegacyDoesNotDropClaimedRows(t *testing.T) {
	openMultiUserDB(t)

	admin := &User{Username: "admin", PasswordHash: "h", Role: UserRoleAdmin, IsActive: true}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}

	pattern := "api.openai.com"
	if err := db.Create(&CredentialEntry{
		UserID:   admin.ID,
		Pattern:  pattern,
		AuthType: "bearer",
		TokenEnc: "claimed",
	}).Error; err != nil {
		t.Fatalf("seed claimed: %v", err)
	}
	if err := db.Create(&CredentialEntry{
		Pattern:  pattern,
		AuthType: "bearer",
		TokenEnc: "orphan",
	}).Error; err != nil {
		t.Fatalf("seed orphan: %v", err)
	}

	if err := AdoptLegacyData(admin.ID); err != nil {
		t.Fatalf("adopt: %v", err)
	}

	var rows []CredentialEntry
	if err := db.Where("pattern = ?", pattern).Find(&rows).Error; err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("esperava 1 row final (claimed canônica), got %d (%+v)", len(rows), rows)
	}
	if rows[0].TokenEnc != "claimed" {
		t.Fatalf("a row vencedora deveria ser a claimed (token=claimed), got token=%q", rows[0].TokenEnc)
	}
}

// openPreAEP0052DB cria DB in-memory com schema EXATO da base
// pré-AEP-0052 (apenas tabelas existentes naquela versão e
// `credential_entries` sem coluna `user_id`).
func openPreAEP0052DB(t *testing.T) {
	t.Helper()

	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})

	if err := db.Exec(`
		CREATE TABLE credential_entries (
			id TEXT PRIMARY KEY,
			pattern TEXT,
			auth_type TEXT,
			token_enc TEXT,
			username TEXT,
			password_enc TEXT,
			headers_enc TEXT,
			expires_at INTEGER,
			refresh_token_enc TEXT,
			client_id_enc TEXT,
			client_secret_enc TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create credential_entries: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE credential_key_wraps (
			id TEXT PRIMARY KEY,
			kind TEXT UNIQUE,
			salt TEXT,
			wrapped_dek TEXT,
			argon_time INTEGER,
			argon_memory INTEGER,
			argon_threads INTEGER,
			created_at DATETIME,
			updated_at DATETIME
		)
	`).Error; err != nil {
		t.Fatalf("create credential_key_wraps: %v", err)
	}
}

// openMultiUserDB cria DB in-memory já com o schema AEP-0052 aplicado
// (tabelas e índices). Usado por testes que partem de "boot já
// concluído".
func openMultiUserDB(t *testing.T) {
	t.Helper()

	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&User{},
		&Session{},
		&CredentialEntry{},
		&CredentialKeyWrap{},
		&LLMProvider{},
		&Conversation{},
		&ChatMessage{},
		&TaskListWorkflow{},
		&TaskList{},
		&Task{},
		&TaskNote{},
	); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	ensureCredentialEntryUserPatternIndex()

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		db = nil
	})
}

func assertCredentialCount(t *testing.T, want int, msg string) {
	t.Helper()
	var got int64
	if err := db.Table("credential_entries").Count(&got).Error; err != nil {
		t.Fatalf("count credentials (%s): %v", msg, err)
	}
	if int(got) != want {
		t.Fatalf("%s: count credential_entries = %d, want %d", msg, got, want)
	}
	_ = fmt.Sprintf // silence unused if message string formatted later
}
