package database

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// migrateExternalIdentityMapping cria o armazenamento central do vínculo
// administrativo externo. O bootstrap deve registrá-la como v25, depois do
// AutoMigrate. Esta função não publica readiness nem altera middleware.
//
// issuer e subject usam a collation binária padrão: a unicidade é exatamente
// o par persistido, preservando case. A DDL e a camada auth rejeitam valores
// vazios, whitespace de borda e NUL.
func migrateExternalIdentityMapping(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para identidade externa")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable(&User{}) {
			return errors.New("tabela users ausente para identidade externa")
		}
		objectType, err := externalIdentityMappingObjectType(tx)
		if err != nil {
			return err
		}
		if objectType != "" && objectType != "table" {
			return errors.New("external_identity_mappings não é uma tabela")
		}
		if objectType == "" {
			if err := tx.Exec(externalIdentityMappingTableDDL).Error; err != nil {
				return err
			}
			if err := tx.Exec(externalIdentityMappingUniqueIndexDDL).Error; err != nil {
				return err
			}
			if err := tx.Exec(externalIdentityMappingUserIndexDDL).Error; err != nil {
				return err
			}
			if err := tx.Exec(externalIdentityMappingEnabledIndexDDL).Error; err != nil {
				return err
			}
		}
		return validateExternalIdentityMappingSchema(tx)
	})
}

const externalIdentityMappingTableDDL = `
CREATE TABLE external_identity_mappings (
    id TEXT NOT NULL PRIMARY KEY
        CHECK (length(id) = 36 AND length(replace(id, '-', '')) = 32
            AND lower(id) = id AND id NOT GLOB '*[^0-9a-f-]*'
            AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-'
            AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
            AND substr(id, 15, 1) = '7'
            AND substr(id, 20, 1) IN ('8', '9', 'a', 'b')),
    issuer TEXT NOT NULL COLLATE BINARY
        CHECK (length(issuer) > 0 AND trim(issuer) = issuer AND instr(issuer, char(0)) = 0),
    subject TEXT NOT NULL COLLATE BINARY
        CHECK (length(subject) > 0 AND trim(subject) = subject AND instr(subject, char(0)) = 0),
    user_id TEXT NOT NULL
        CHECK (length(user_id) = 36 AND length(replace(user_id, '-', '')) = 32
            AND lower(user_id) = user_id AND user_id NOT GLOB '*[^0-9a-f-]*'
            AND substr(user_id, 9, 1) = '-' AND substr(user_id, 14, 1) = '-'
            AND substr(user_id, 19, 1) = '-' AND substr(user_id, 24, 1) = '-'
            AND substr(user_id, 15, 1) = '7'
            AND substr(user_id, 20, 1) IN ('8', '9', 'a', 'b')),
    enabled BOOLEAN NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    CONSTRAINT fk_external_identity_mappings_user
        FOREIGN KEY (user_id) REFERENCES users(id)
        ON UPDATE CASCADE ON DELETE RESTRICT
)`

const externalIdentityMappingUniqueIndexDDL = `
CREATE UNIQUE INDEX ux_external_identity_issuer_subject
    ON external_identity_mappings (issuer COLLATE BINARY, subject COLLATE BINARY)`

const externalIdentityMappingUserIndexDDL = `
CREATE INDEX idx_external_identity_user_id ON external_identity_mappings (user_id)`

const externalIdentityMappingEnabledIndexDDL = `
CREATE INDEX idx_external_identity_enabled ON external_identity_mappings (enabled)`

func externalIdentityMappingObjectType(db *gorm.DB) (string, error) {
	var objectType string
	if err := db.Raw("SELECT type FROM sqlite_master WHERE name = ?", "external_identity_mappings").Scan(&objectType).Error; err != nil {
		return "", err
	}
	return objectType, nil
}

func validateExternalIdentityMappingSchema(db *gorm.DB) error {
	var schema string
	if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", "external_identity_mappings").Scan(&schema).Error; err != nil {
		return err
	}
	if normalizeExternalIdentityDDL(schema) != normalizeExternalIdentityDDL(externalIdentityMappingTableDDL) {
		return errors.New("DDL de external_identity_mappings divergente")
	}
	if err := externalIdentityMappingHasUserFK(db); err != nil {
		return err
	}
	indexes, err := externalIdentityMappingIndexes(db)
	if err != nil {
		return err
	}
	expected := map[string]struct {
		unique bool
		cols   []string
	}{
		"ux_external_identity_issuer_subject": {unique: true, cols: []string{"issuer", "subject"}},
		"idx_external_identity_user_id":       {cols: []string{"user_id"}},
		"idx_external_identity_enabled":       {cols: []string{"enabled"}},
	}
	if len(indexes) != len(expected)+1 {
		return errors.New("índices de external_identity_mappings divergentes")
	}
	autoIndexes := 0
	for name, got := range indexes {
		if strings.HasPrefix(name, "sqlite_autoindex_") {
			autoIndexes++
			if !got.unique || len(got.cols) != 1 || got.cols[0] != "id" {
				return errors.New("autoíndice da PK de external_identity_mappings divergente")
			}
			continue
		}
		want, ok := expected[name]
		if !ok {
			return fmt.Errorf("índice desconhecido de external_identity_mappings: %s", name)
		}
		if got.unique != want.unique || len(got.cols) != len(want.cols) {
			return fmt.Errorf("índice %s de external_identity_mappings divergente", name)
		}
		for i := range want.cols {
			if got.cols[i] != want.cols[i] {
				return fmt.Errorf("colunas do índice %s de external_identity_mappings divergentes", name)
			}
		}
		var sql string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&sql).Error; err != nil {
			return err
		}
		var expectedSQL string
		switch name {
		case "ux_external_identity_issuer_subject":
			expectedSQL = externalIdentityMappingUniqueIndexDDL
		case "idx_external_identity_user_id":
			expectedSQL = externalIdentityMappingUserIndexDDL
		case "idx_external_identity_enabled":
			expectedSQL = externalIdentityMappingEnabledIndexDDL
		}
		if normalizeExternalIdentityDDL(sql) != normalizeExternalIdentityDDL(expectedSQL) {
			return fmt.Errorf("SQL do índice %s de external_identity_mappings divergente", name)
		}
	}
	if autoIndexes != 1 {
		return errors.New("autoíndice da PK de external_identity_mappings ausente")
	}
	return nil
}

type externalIdentityIndex struct {
	unique bool
	cols   []string
}

func externalIdentityMappingIndexes(db *gorm.DB) (map[string]externalIdentityIndex, error) {
	var rows []struct {
		Name   string `gorm:"column:name"`
		Unique int    `gorm:"column:unique"`
	}
	if err := db.Raw("PRAGMA index_list('external_identity_mappings')").Scan(&rows).Error; err != nil {
		return nil, err
	}
	indexes := make(map[string]externalIdentityIndex, len(rows))
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
		indexes[row.Name] = externalIdentityIndex{unique: row.Unique == 1, cols: cols}
	}
	return indexes, nil
}

func normalizeExternalIdentityDDL(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func externalIdentityMappingHasUserFK(db *gorm.DB) error {
	var rows []struct {
		Table string `gorm:"column:table"`
		From  string `gorm:"column:from"`
		To    string `gorm:"column:to"`
	}
	if err := db.Raw("PRAGMA foreign_key_list('external_identity_mappings')").Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if row.Table == "users" && row.From == "user_id" && row.To == "id" {
			return nil
		}
	}
	return errors.New("external_identity_mappings sem FK users(id)")
}
