package database

import (
	"database/sql"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupMultiUserTestDB(t *testing.T) *sql.DB {
	t.Helper()

	var err error
	db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&User{},
		&Session{},
		&Conversation{},
		&ChatMessage{},
		&CredentialEntry{},
		&CredentialKeyWrap{},
		&LLMProvider{},
		&TaskListWorkflow{},
		&TaskList{},
		&Task{},
		&TaskNote{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ensureCredentialEntryUserPatternIndex()

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		db = nil
	})
	return sqlDB
}

func TestMultiUserSchemaAddsOwnershipColumns(t *testing.T) {
	sqlDB := setupMultiUserTestDB(t)

	for _, tc := range []struct {
		table  string
		column string
	}{
		{table: "llm_providers", column: "user_id"},
		{table: "conversations", column: "user_id"},
		{table: "credential_entries", column: "user_id"},
		{table: "task_lists", column: "user_id"},
		{table: "sessions", column: "user_id"},
	} {
		if !columnExists(t, sqlDB, tc.table, tc.column) {
			t.Fatalf("%s.%s not found", tc.table, tc.column)
		}
	}
}

func TestCredentialEntriesUniquePerUserPattern(t *testing.T) {
	setupMultiUserTestDB(t)

	firstUser := &User{Username: "ana", PasswordHash: "hash", Role: UserRoleAdmin, IsActive: true}
	secondUser := &User{Username: "leo", PasswordHash: "hash", Role: UserRoleUser, IsActive: true}
	if err := db.Create(firstUser).Error; err != nil {
		t.Fatalf("create first user: %v", err)
	}
	if err := db.Create(secondUser).Error; err != nil {
		t.Fatalf("create second user: %v", err)
	}

	pattern := "api.openai.com"
	if err := db.Create(&CredentialEntry{UserID: firstUser.ID, Pattern: pattern, AuthType: "bearer"}).Error; err != nil {
		t.Fatalf("create first credential: %v", err)
	}
	if err := db.Create(&CredentialEntry{UserID: secondUser.ID, Pattern: pattern, AuthType: "bearer"}).Error; err != nil {
		t.Fatalf("same pattern should be allowed for another user: %v", err)
	}
	if err := db.Create(&CredentialEntry{UserID: firstUser.ID, Pattern: pattern, AuthType: "bearer"}).Error; err == nil {
		t.Fatal("expected duplicate credential pattern for same user to fail")
	}
}

func TestAdoptLegacyDataAssignsBlankOwners(t *testing.T) {
	setupMultiUserTestDB(t)

	user := &User{Username: "admin", PasswordHash: "hash", Role: UserRoleAdmin, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&LLMProvider{ID: "openai", Name: "OpenAI", Type: "openai", BaseURL: "https://api.openai.com/v1"}).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if err := db.Create(&Conversation{Title: "legacy"}).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := db.Create(&CredentialEntry{Pattern: "api.openai.com", AuthType: "bearer"}).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	if err := db.Create(&TaskList{Title: "legacy tasks"}).Error; err != nil {
		t.Fatalf("create task list: %v", err)
	}

	if err := AdoptLegacyData(user.ID); err != nil {
		t.Fatalf("adopt legacy data: %v", err)
	}
	if err := AdoptLegacyData(user.ID); err != nil {
		t.Fatalf("adopt legacy data should be idempotent: %v", err)
	}

	assertOwnedRows(t, "llm_providers", user.ID)
	assertOwnedRows(t, "conversations", user.ID)
	assertOwnedRows(t, "credential_entries", user.ID)
	assertOwnedRows(t, "task_lists", user.ID)
}

// TestAdoptLegacyData_OrphanWithExistingClaim cobre o cenário que travou
// o login do primeiro admin em produção (Wails dev log
// `database.go:208 UNIQUE constraint failed: credential_entries.user_id,
// credential_entries.pattern`):
//
//   - banco contém um par (user_id='', pattern=X) — herdado de boots
//     pré-AEP-0052 que ainda gravavam órfãos depois do AutoMigrate.
//   - o usuário corrente já tem (user_id=X, pattern=X) gravado por um
//     login bem-sucedido anterior.
//   - AdoptLegacyData tenta `UPDATE ... SET user_id = X WHERE user_id =
//     ''` e o índice `ux_credential_entries_user_pattern` aborta a
//     transação inteira.
//
// Resultado observado pelo usuário: CreateAdminUser cria o User no banco,
// mas o Login subsequente falha; a próxima tentativa de criar admin bate
// em "admin inicial já foi criado". A correção descarta a órfã antes do
// UPDATE.
func TestAdoptLegacyData_OrphanWithExistingClaim(t *testing.T) {
	setupMultiUserTestDB(t)

	user := &User{Username: "admin", PasswordHash: "hash", Role: UserRoleAdmin, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	pattern := "api.openai.com"
	if err := db.Create(&CredentialEntry{UserID: user.ID, Pattern: pattern, AuthType: "bearer"}).Error; err != nil {
		t.Fatalf("create claimed credential: %v", err)
	}
	if err := db.Create(&CredentialEntry{Pattern: pattern, AuthType: "bearer"}).Error; err != nil {
		t.Fatalf("create orphan credential: %v", err)
	}

	if err := AdoptLegacyData(user.ID); err != nil {
		t.Fatalf("adopt legacy data should not violate unique index: %v", err)
	}

	var entries []CredentialEntry
	if err := db.Where("pattern = ?", pattern).Find(&entries).Error; err != nil {
		t.Fatalf("list credentials: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected single canonical credential after adopt, got %d: %+v", len(entries), entries)
	}
	if entries[0].UserID != user.ID {
		t.Fatalf("expected canonical entry to belong to admin %q, got %q", user.ID, entries[0].UserID)
	}

	if err := AdoptLegacyData(user.ID); err != nil {
		t.Fatalf("second adopt should remain idempotent: %v", err)
	}
}

func TestAdoptLegacyDataKeepsInternalSecretsInstanceScoped(t *testing.T) {
	setupMultiUserTestDB(t)

	user := &User{Username: "admin", PasswordHash: "hash", Role: UserRoleAdmin, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&CredentialEntry{Pattern: "internal-auth:refresh-token", AuthType: "secret"}).Error; err != nil {
		t.Fatalf("create instance refresh secret: %v", err)
	}
	if err := db.Create(&CredentialEntry{UserID: user.ID, Pattern: "internal-auth:refresh-token", AuthType: "secret"}).Error; err != nil {
		t.Fatalf("create user-scoped duplicate refresh secret: %v", err)
	}

	if err := AdoptLegacyData(user.ID); err != nil {
		t.Fatalf("adopt legacy data: %v", err)
	}

	var entries []CredentialEntry
	if err := db.Where("pattern = ?", "internal-auth:refresh-token").Find(&entries).Error; err != nil {
		t.Fatalf("list internal secrets: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected duplicate internal secret to be collapsed, got %+v", entries)
	}
	if entries[0].UserID != "" {
		t.Fatalf("internal secret should remain instance-scoped, got user_id=%q", entries[0].UserID)
	}
}

func columnExists(t *testing.T, sqlDB *sql.DB, table, column string) bool {
	t.Helper()

	rows, err := sqlDB.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("table info %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan column info: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}

func assertOwnedRows(t *testing.T, table, userID string) {
	t.Helper()

	var count int64
	if err := db.Table(table).Where("user_id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count owned rows in %s: %v", table, err)
	}
	if count != 1 {
		t.Fatalf("%s: expected 1 owned row, got %d", table, count)
	}

	if err := db.Table(table).Where("user_id IS NULL OR user_id = ''").Count(&count).Error; err != nil {
		t.Fatalf("count blank rows in %s: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("%s: expected no blank owners, got %d", table, count)
	}
}
