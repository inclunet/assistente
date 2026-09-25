package database

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// MigrateExternalIdentityAdminAudit cria o ledger central de decisões
// administrativas externas. Ele é independente da readiness e da adoção do
// middleware; a unicidade parcial serializa bootstrap por issuer no próprio
// banco. A transação de Auth deve inserir a auditoria antes de alterar o mapa.
func MigrateExternalIdentityAdminAudit(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para auditoria de identidade externa")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable(&User{}) {
			return errors.New("tabela users ausente para auditoria de identidade externa")
		}
		objectType, err := externalIdentityAdminAuditObjectType(tx)
		if err != nil {
			return err
		}
		if objectType != "" && objectType != "table" {
			return errors.New("external_identity_admin_audits não é uma tabela")
		}
		if objectType == "" {
			for _, ddl := range []string{
				externalIdentityAdminAuditTableDDL,
				externalIdentityAdminAuditBootstrapIndexDDL,
				externalIdentityAdminAuditActorIndexDDL,
				externalIdentityAdminAuditUserIndexDDL,
				externalIdentityAdminAuditIdentityIndexDDL,
			} {
				if err := tx.Exec(ddl).Error; err != nil {
					return err
				}
			}
		}
		return validateExternalIdentityAdminAuditSchema(tx)
	})
}

const externalIdentityAdminAuditTableDDL = `
CREATE TABLE external_identity_admin_audits (
    id TEXT NOT NULL PRIMARY KEY
        CHECK (length(id) = 36 AND length(replace(id, '-', '')) = 32
            AND lower(id) = id AND id NOT GLOB '*[^0-9a-f-]*'
            AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-'
            AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
            AND substr(id, 15, 1) = '7'
            AND substr(id, 20, 1) IN ('8', '9', 'a', 'b')),
    issuer TEXT NOT NULL COLLATE BINARY
        CHECK (length(issuer) > 0 AND trim(issuer) = issuer AND instr(issuer, char(0)) = 0),
    actor_subject TEXT NOT NULL COLLATE BINARY
        CHECK (length(actor_subject) > 0 AND trim(actor_subject) = actor_subject AND instr(actor_subject, char(0)) = 0),
    actor_user_id TEXT NOT NULL
        CHECK (length(actor_user_id) = 36 AND length(replace(actor_user_id, '-', '')) = 32
            AND lower(actor_user_id) = actor_user_id AND actor_user_id NOT GLOB '*[^0-9a-f-]*'
            AND substr(actor_user_id, 9, 1) = '-' AND substr(actor_user_id, 14, 1) = '-'
            AND substr(actor_user_id, 19, 1) = '-' AND substr(actor_user_id, 24, 1) = '-'
            AND substr(actor_user_id, 15, 1) = '7'
            AND substr(actor_user_id, 20, 1) IN ('8', '9', 'a', 'b')),
    action TEXT NOT NULL
        CHECK (action IN ('bootstrap', 'create')),
    subject TEXT NOT NULL COLLATE BINARY
        CHECK (length(subject) > 0 AND trim(subject) = subject AND instr(subject, char(0)) = 0),
    user_id TEXT NOT NULL
        CHECK (length(user_id) = 36 AND length(replace(user_id, '-', '')) = 32
            AND lower(user_id) = user_id AND user_id NOT GLOB '*[^0-9a-f-]*'
            AND substr(user_id, 9, 1) = '-' AND substr(user_id, 14, 1) = '-'
            AND substr(user_id, 19, 1) = '-' AND substr(user_id, 24, 1) = '-'
            AND substr(user_id, 15, 1) = '7'
            AND substr(user_id, 20, 1) IN ('8', '9', 'a', 'b')),
    created_at DATETIME NOT NULL,
    CHECK (action <> 'bootstrap' OR (
        actor_subject = subject AND actor_subject = actor_user_id AND actor_user_id = user_id
    )),
    CONSTRAINT fk_external_identity_admin_audit_actor_user
        FOREIGN KEY (actor_user_id) REFERENCES users(id)
        ON UPDATE CASCADE ON DELETE RESTRICT,
    CONSTRAINT fk_external_identity_admin_audit_user
        FOREIGN KEY (user_id) REFERENCES users(id)
        ON UPDATE CASCADE ON DELETE RESTRICT
)`

const externalIdentityAdminAuditBootstrapIndexDDL = `
CREATE UNIQUE INDEX ux_external_identity_admin_audit_bootstrap_issuer
    ON external_identity_admin_audits (issuer COLLATE BINARY)
    WHERE action = 'bootstrap'`

const externalIdentityAdminAuditActorIndexDDL = `
CREATE INDEX idx_external_identity_admin_audit_actor_user
    ON external_identity_admin_audits (actor_user_id)`

const externalIdentityAdminAuditUserIndexDDL = `
CREATE INDEX idx_external_identity_admin_audit_user
    ON external_identity_admin_audits (user_id)`

const externalIdentityAdminAuditIdentityIndexDDL = `
CREATE INDEX idx_external_identity_admin_audit_issuer_subject_created
    ON external_identity_admin_audits (issuer COLLATE BINARY, subject COLLATE BINARY, created_at)`

func externalIdentityAdminAuditObjectType(db *gorm.DB) (string, error) {
	var objectType string
	if err := db.Raw("SELECT type FROM sqlite_master WHERE name = ?", "external_identity_admin_audits").Scan(&objectType).Error; err != nil {
		return "", err
	}
	return objectType, nil
}

func validateExternalIdentityAdminAuditSchema(db *gorm.DB) error {
	var schema string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", "external_identity_admin_audits").Scan(&schema).Error; err != nil {
		return err
	}
	if normalizeExternalIdentityDDL(schema) != normalizeExternalIdentityDDL(externalIdentityAdminAuditTableDDL) {
		return errors.New("DDL de external_identity_admin_audits divergente")
	}
	if err := externalIdentityAdminAuditHasUserFKs(db); err != nil {
		return err
	}
	indexes, err := externalIdentityAdminAuditIndexes(db)
	if err != nil {
		return err
	}
	expected := map[string]struct {
		unique  bool
		partial bool
		cols    []string
		sql     string
	}{
		"ux_external_identity_admin_audit_bootstrap_issuer":        {unique: true, partial: true, cols: []string{"issuer"}, sql: externalIdentityAdminAuditBootstrapIndexDDL},
		"idx_external_identity_admin_audit_actor_user":             {cols: []string{"actor_user_id"}, sql: externalIdentityAdminAuditActorIndexDDL},
		"idx_external_identity_admin_audit_user":                   {cols: []string{"user_id"}, sql: externalIdentityAdminAuditUserIndexDDL},
		"idx_external_identity_admin_audit_issuer_subject_created": {cols: []string{"issuer", "subject", "created_at"}, sql: externalIdentityAdminAuditIdentityIndexDDL},
	}
	if len(indexes) != len(expected)+1 {
		return errors.New("índices de external_identity_admin_audits divergentes")
	}
	autoIndexes := 0
	for name, got := range indexes {
		if strings.HasPrefix(name, "sqlite_autoindex_") {
			autoIndexes++
			if !got.unique || got.partial || len(got.cols) != 1 || got.cols[0] != "id" {
				return errors.New("autoíndice da PK de external_identity_admin_audits divergente")
			}
			continue
		}
		want, ok := expected[name]
		if !ok || got.unique != want.unique || got.partial != want.partial || len(got.cols) != len(want.cols) {
			return fmt.Errorf("índice %s de external_identity_admin_audits divergente", name)
		}
		for i := range want.cols {
			if got.cols[i] != want.cols[i] {
				return fmt.Errorf("colunas do índice %s de external_identity_admin_audits divergentes", name)
			}
		}
		var sql string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&sql).Error; err != nil {
			return err
		}
		if normalizeExternalIdentityDDL(sql) != normalizeExternalIdentityDDL(want.sql) {
			return fmt.Errorf("SQL do índice %s de external_identity_admin_audits divergente", name)
		}
	}
	if autoIndexes != 1 {
		return errors.New("autoíndice da PK de external_identity_admin_audits ausente")
	}
	return nil
}

type externalIdentityAdminAuditIndex struct {
	unique  bool
	partial bool
	cols    []string
}

func externalIdentityAdminAuditIndexes(db *gorm.DB) (map[string]externalIdentityAdminAuditIndex, error) {
	var rows []struct {
		Name    string `gorm:"column:name"`
		Unique  int    `gorm:"column:unique"`
		Partial int    `gorm:"column:partial"`
	}
	if err := db.Raw("PRAGMA index_list('external_identity_admin_audits')").Scan(&rows).Error; err != nil {
		return nil, err
	}
	indexes := make(map[string]externalIdentityAdminAuditIndex, len(rows))
	for _, row := range rows {
		var columns []struct {
			Seq  int    `gorm:"column:seqno"`
			Name string `gorm:"column:name"`
		}
		indexName := strings.ReplaceAll(row.Name, "'", "''")
		if err := db.Raw(fmt.Sprintf("PRAGMA index_info('%s')", indexName)).Scan(&columns).Error; err != nil {
			return nil, err
		}
		cols := make([]string, len(columns))
		for _, column := range columns {
			if column.Seq < 0 || column.Seq >= len(cols) {
				return nil, fmt.Errorf("índice %s com sequência inválida", row.Name)
			}
			cols[column.Seq] = column.Name
		}
		indexes[row.Name] = externalIdentityAdminAuditIndex{unique: row.Unique == 1, partial: row.Partial == 1, cols: cols}
	}
	return indexes, nil
}

func externalIdentityAdminAuditHasUserFKs(db *gorm.DB) error {
	var rows []struct {
		Table    string `gorm:"column:table"`
		From     string `gorm:"column:from"`
		To       string `gorm:"column:to"`
		OnUpdate string `gorm:"column:on_update"`
		OnDelete string `gorm:"column:on_delete"`
	}
	if err := db.Raw("PRAGMA foreign_key_list('external_identity_admin_audits')").Scan(&rows).Error; err != nil {
		return err
	}
	found := map[string]bool{}
	for _, row := range rows {
		if row.Table == "users" && row.To == "id" && row.OnUpdate == "CASCADE" && row.OnDelete == "RESTRICT" {
			if row.From == "actor_user_id" || row.From == "user_id" {
				found[row.From] = true
			}
		}
	}
	if !found["actor_user_id"] || !found["user_id"] {
		return errors.New("external_identity_admin_audits sem FKs para users(id)")
	}
	return nil
}
