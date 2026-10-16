package database

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMigrateExternalIdentityMappingCreatesExactSchema(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	userID := uuidv7ForExternalIdentityTest(t)
	if err := db.Create(&User{UUIDModel: UUIDModel{ID: userID}, Username: "mapping-owner", PasswordHash: "test-password", Role: UserRoleUser, IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err != nil {
		t.Fatalf("migração: %v", err)
	}
	if err := migrateExternalIdentityMapping(db); err != nil {
		t.Fatalf("migração idempotente: %v", err)
	}
	if !db.Migrator().HasTable("external_identity_mappings") {
		t.Fatal("tabela de mapeamento ausente")
	}
	if err := externalIdentityMappingHasUserFK(db); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'external_identity_mappings'").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	if schema == "" || !strings.Contains(schema, "enabled BOOLEAN NOT NULL DEFAULT 1") || !strings.Contains(schema, "REFERENCES users(id)") {
		t.Fatalf("DDL não contém bool/FK esperados: %s", schema)
	}
	if !db.Migrator().HasIndex("external_identity_mappings", "ux_external_identity_issuer_subject") {
		t.Fatal("unicidade issuer/subject ausente")
	}
}

func TestExternalIdentityMappingEnforcesFKAndExactUniqueness(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	userID := uuidv7ForExternalIdentityTest(t)
	if err := db.Create(&User{UUIDModel: UUIDModel{ID: userID}, Username: "mapping-owner-2", PasswordHash: "test-password", Role: UserRoleUser, IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err != nil {
		t.Fatal(err)
	}
	insert := func(id, issuer, subject, owner string) error {
		return db.Exec(`INSERT INTO external_identity_mappings
            (id, issuer, subject, user_id, enabled, created_at, updated_at)
            VALUES (?, ?, ?, ?, TRUE, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id, issuer, subject, owner).Error
	}
	if err := insert(uuidv7ForExternalIdentityTest(t), "https://idp.example", "Alice", userID); err != nil {
		t.Fatal(err)
	}
	if err := insert(uuidv7ForExternalIdentityTest(t), "https://idp.example", "Alice", userID); err == nil {
		t.Fatal("par issuer/subject exato duplicado foi aceito")
	}
	if err := insert(uuidv7ForExternalIdentityTest(t), "https://idp.example", "alice", userID); err != nil {
		t.Fatalf("variação de case colidiu indevidamente: %v", err)
	}
	if err := insert(uuidv7ForExternalIdentityTest(t), "https://idp.example", "Alice ", userID); err == nil {
		t.Fatal("whitespace de borda foi aceito pela DDL")
	}
	if err := insert(uuidv7ForExternalIdentityTest(t), "https://idp.example", "Alice\x00suffix", userID); err == nil {
		t.Fatal("NUL foi aceito pela DDL")
	}
	if err := db.Exec(`INSERT INTO external_identity_mappings
        (id, issuer, subject, user_id, enabled, created_at, updated_at)
        VALUES (?, ?, ?, ?, 2, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, uuidv7ForExternalIdentityTest(t), "https://idp.example", "invalid-enabled", userID).Error; err == nil {
		t.Fatal("enabled fora de 0/1 foi aceito pela DDL")
	}
	if err := db.Exec("UPDATE external_identity_mappings SET enabled = 2 WHERE subject = ?", "Alice").Error; err == nil {
		t.Fatal("CHECK de enabled não protegeu UPDATE")
	}
	if err := insert("not-a-uuid", "https://idp.example", "invalid-id", userID); err == nil {
		t.Fatal("DDL aceitou id que não é UUIDv7")
	}
	if err := insert(uuidv7ForExternalIdentityTest(t), "https://idp.example", "orphan", uuidv7ForExternalIdentityTest(t)); err == nil {
		t.Fatal("FK aceitou user_id órfão")
	}
	var count int64
	if err := db.Table("external_identity_mappings").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("linhas exatas inesperadas: %d", count)
	}
}

func TestMigrateExternalIdentityMappingRejectsCorruptIndex(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP INDEX ux_external_identity_issuer_subject").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX ux_external_identity_issuer_subject ON external_identity_mappings (subject, issuer)").Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err == nil {
		t.Fatal("índice unique com ordem corrompida foi aceito")
	}
}

func TestMigrateExternalIdentityMappingRejectsCollationAndPartialUniqueIndex(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		"CREATE UNIQUE INDEX ux_external_identity_issuer_subject ON external_identity_mappings (issuer COLLATE NOCASE, subject COLLATE BINARY)",
		"CREATE UNIQUE INDEX ux_external_identity_issuer_subject ON external_identity_mappings (issuer COLLATE BINARY, subject COLLATE BINARY) WHERE enabled = 1",
	} {
		if err := db.Exec("DROP INDEX ux_external_identity_issuer_subject").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(ddl).Error; err != nil {
			t.Fatal(err)
		}
		if err := migrateExternalIdentityMapping(db); err == nil {
			t.Fatalf("índice corrompido foi aceito: %s", ddl)
		}
		if err := db.Exec("DROP INDEX ux_external_identity_issuer_subject").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(externalIdentityMappingUniqueIndexDDL).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrateExternalIdentityMappingRejectsCorruptUUIDAndTextSchema(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE external_identity_mappings (
        id INTEGER PRIMARY KEY,
        issuer TEXT,
        subject TEXT,
        user_id TEXT,
        enabled INTEGER,
        created_at DATETIME,
        updated_at DATETIME
    )`).Error; err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err == nil {
		t.Fatal("schema UUID7/text corrompido foi aceito")
	}
	var indexCount int64
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND tbl_name = ?", "external_identity_mappings").Scan(&indexCount).Error; err != nil {
		t.Fatal(err)
	}
	if indexCount != 0 {
		t.Fatalf("migração escreveu índices após detectar schema divergente: %d", indexCount)
	}
}

func uuidv7ForExternalIdentityTest(t *testing.T) string {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	return id.String()
}
