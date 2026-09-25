package database

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type externalIdentityMappingAuditTestRow struct {
	ID        string    `gorm:"column:id"`
	Issuer    string    `gorm:"column:issuer"`
	Subject   string    `gorm:"column:subject"`
	UserID    string    `gorm:"column:user_id"`
	Enabled   bool      `gorm:"column:enabled"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func TestMigrateExternalIdentityAdminAuditCreatesCanonicalSchema(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err != nil {
		t.Fatal(err)
	}
	if err := MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatalf("migração canônica: %v", err)
	}
	if err := MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatalf("migração idempotente: %v", err)
	}
	if !db.Migrator().HasTable(&ExternalIdentityAdminAudit{}) {
		t.Fatal("tabela de auditoria ausente")
	}
	var schema string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", "external_identity_admin_audits").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		"actor_subject TEXT NOT NULL COLLATE BINARY",
		"actor_user_id TEXT NOT NULL",
		"action IN ('bootstrap', 'create')",
		"REFERENCES users(id)",
		"actor_subject = subject AND actor_subject = actor_user_id AND actor_user_id = user_id",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("DDL não contém %q: %s", fragment, schema)
		}
	}
	if !db.Migrator().HasIndex(&ExternalIdentityAdminAudit{}, "ux_external_identity_admin_audit_bootstrap_issuer") {
		t.Fatal("índice parcial de bootstrap por issuer ausente")
	}
	if err := externalIdentityAdminAuditHasUserFKs(db); err != nil {
		t.Fatal(err)
	}
	var foreignKeyCount int64
	if err := db.Raw("SELECT COUNT(*) FROM pragma_foreign_key_list('external_identity_admin_audits')").Scan(&foreignKeyCount).Error; err != nil {
		t.Fatal(err)
	}
	if foreignKeyCount != 2 {
		t.Fatalf("quantidade de FKs = %d, esperava actor_user_id e user_id", foreignKeyCount)
	}
}

func TestExternalIdentityAdminAuditConstraintsAndPartialBootstrapUniqueness(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err != nil {
		t.Fatal(err)
	}
	actorID := uuidv7ForExternalIdentityTest(t)
	targetID := uuidv7ForExternalIdentityTest(t)
	for i, user := range []User{
		{UUIDModel: UUIDModel{ID: actorID}, Username: "audit-actor", PasswordHash: "synthetic", Role: UserRoleAdmin, IsActive: true},
		{UUIDModel: UUIDModel{ID: targetID}, Username: "audit-target", PasswordHash: "synthetic", Role: UserRoleUser, IsActive: true},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("criar usuário %d: %v", i, err)
		}
	}
	if err := MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatal(err)
	}
	insert := func(issuer, actorSubject, actorUserID, action, subject, userID string) error {
		row := ExternalIdentityAdminAudit{
			ID: uuidv7ForExternalIdentityTest(t), Issuer: issuer,
			ActorSubject: actorSubject, ActorUserID: actorUserID,
			Action: action, Subject: subject, UserID: userID, CreatedAt: time.Now().UTC(),
		}
		return db.Create(&row).Error
	}
	if err := insert("issuer-a", actorID, actorID, "bootstrap", actorID, actorID); err != nil {
		t.Fatalf("primeiro bootstrap: %v", err)
	}
	if err := insert("issuer-a", actorID, actorID, "bootstrap", actorID, actorID); err == nil {
		t.Fatal("bootstrap duplicado para o mesmo issuer foi aceito")
	}
	if err := insert("issuer-a", actorID, actorID, "create", "external-1", targetID); err != nil {
		t.Fatalf("criação não deve colidir com a unicidade de bootstrap: %v", err)
	}
	if err := insert("issuer-a", actorID, actorID, "create", "external-2", targetID); err != nil {
		t.Fatalf("múltiplas ações create devem ser auditáveis: %v", err)
	}
	if err := insert("issuer-b", actorID, actorID, "bootstrap", actorID, actorID); err != nil {
		t.Fatalf("bootstrap em issuer distinto: %v", err)
	}
	for _, invalid := range []struct {
		issuer, actorSubject, actorUserID, action, subject, userID string
	}{
		{"issuer-c", actorID, actorID, "enable", "external-3", targetID},
		{"issuer-c", actorID, actorID, "bootstrap", "different-subject", actorID},
		{"issuer-c", "arbitrary", actorID, "bootstrap", "arbitrary", actorID},
		{"issuer-c", actorID, uuidv7ForExternalIdentityTest(t), "create", "external-3", targetID},
		{"issuer-c", actorID, actorID, "create", "external-3", uuidv7ForExternalIdentityTest(t)},
		{"issuer-c ", actorID, actorID, "create", "external-3", targetID},
		{"issuer-c", "external-3\x00tail", actorID, "create", "external-3", targetID},
	} {
		if err := insert(invalid.issuer, invalid.actorSubject, invalid.actorUserID, invalid.action, invalid.subject, invalid.userID); err == nil {
			t.Errorf("registro inválido foi aceito: action=%q issuer=%q subject=%q", invalid.action, invalid.issuer, invalid.subject)
		}
	}
	var count int64
	if err := db.Model(&ExternalIdentityAdminAudit{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("linhas de auditoria válidas = %d, esperado 4", count)
	}
}

func TestExternalIdentityAdminAuditRollsBackWithMappingTransaction(t *testing.T) {
	db := newMigratorTestDB(t)
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}); err != nil {
		t.Fatal(err)
	}
	if err := migrateExternalIdentityMapping(db); err != nil {
		t.Fatal(err)
	}
	actorID := uuidv7ForExternalIdentityTest(t)
	if err := db.Create(&User{UUIDModel: UUIDModel{ID: actorID}, Username: "audit-rollback", PasswordHash: "synthetic", Role: UserRoleAdmin, IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateExternalIdentityAdminAudit(db); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("simulate mapping failure")
	err := db.Transaction(func(tx *gorm.DB) error {
		row := ExternalIdentityAdminAudit{
			ID: uuidv7ForExternalIdentityTest(t), Issuer: "issuer-rollback",
			ActorSubject: actorID, ActorUserID: actorID, Action: "create",
			Subject: "external-target", UserID: actorID, CreatedAt: time.Now().UTC(),
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		mapping := externalIdentityMappingAuditTestRow{
			ID: uuidv7ForExternalIdentityTest(t), Issuer: "issuer-rollback",
			Subject: "external-target", UserID: actorID, Enabled: true,
			CreatedAt: row.CreatedAt, UpdatedAt: row.CreatedAt,
		}
		if err := tx.Table("external_identity_mappings").Create(&mapping).Error; err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("erro de rollback = %v, esperado sentinel", err)
	}
	for _, table := range []string{"external_identity_admin_audits", "external_identity_mappings"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rollback deixou %d linhas em %s", count, table)
		}
	}
}

func TestExternalIdentityAdminAuditPublishedUpgradesAndSecondBoot(t *testing.T) {
	for _, release := range []string{"0.1.9", "0.2.0", "0.3.0", "0.4.0", "0.5.0"} {
		t.Run(release, func(t *testing.T) {
			db := loadPublishedReleaseFixture(t, release)
			runCurrentUpgrade(t, db)
			if !db.Migrator().HasTable(&ExternalIdentityAdminAudit{}) {
				t.Fatal("upgrade publicado não criou o armazenamento de auditoria")
			}
			versions, err := appliedMigrationVersions(db)
			if err != nil {
				t.Fatal(err)
			}
			if !versions[31] {
				t.Fatal("upgrade publicado não registrou a v31")
			}
			var before string
			if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", "external_identity_admin_audits").Scan(&before).Error; err != nil {
				t.Fatal(err)
			}
			runCurrentUpgrade(t, db)
			var after string
			if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", "external_identity_admin_audits").Scan(&after).Error; err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("segundo boot alterou o schema de auditoria")
			}
			if err := validateExternalIdentityAdminAuditSchema(db); err != nil {
				t.Fatalf("schema após segundo boot: %v", err)
			}
		})
	}
}
