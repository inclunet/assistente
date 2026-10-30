package database

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Rebuild the owned three-table hierarchy atomically, with foreign keys enabled.
// No provider or identity is renamed; existing evidence is copied unchanged.
func migrateLLMModelExactIDs(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var schema string
		if err := tx.Raw("SELECT sql FROM sqlite_master WHERE type='table' AND name='llm_models'").Scan(&schema).Error; err != nil {
			return err
		}
		if schema == "" {
			return errMigrationDeferred
		}
		if !strings.Contains(schema, "trim(remote_id) = remote_id") {
			return nil
		}
		tables := []struct{ name, columns string }{
			{"llm_models", "id,provider_id,remote_id,display_name,created_at,updated_at"},
			{"llm_model_capabilities", "id,model_id,capability_code,created_at,updated_at"},
			{"llm_model_capability_fields", "capability_id,field_code,compatibility_revision,recognizer_id,updated_at"},
		}
		for _, table := range tables {
			if err := tx.Exec("CREATE TABLE " + table.name + "_exact_id_backup AS SELECT " + table.columns + " FROM " + table.name).Error; err != nil {
				return err
			}
		}
		for index := len(tables) - 1; index >= 0; index-- {
			if err := tx.Exec("DROP TABLE " + tables[index].name).Error; err != nil {
				return err
			}
		}
		// The first three statements create the hierarchy; restore data before guards
		// so historical restrictions are not reinterpreted against today's revision.
		for _, statement := range llmModelCapabilityDDL[:3] {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		for _, table := range tables {
			if err := tx.Exec("INSERT INTO " + table.name + " (" + table.columns + ") SELECT " + table.columns + " FROM " + table.name + "_exact_id_backup").Error; err != nil {
				return err
			}
		}
		for _, statement := range llmModelCapabilityDDL[3:] {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		for _, table := range tables {
			var violations []struct{ Table string }
			if err := tx.Raw("PRAGMA foreign_key_check(" + table.name + ")").Scan(&violations).Error; err != nil {
				return err
			}
			if len(violations) > 0 {
				return fmt.Errorf("foreign_key_check(%s): %d violações", table.name, len(violations))
			}
			if err := tx.Exec("DROP TABLE " + table.name + "_exact_id_backup").Error; err != nil {
				return err
			}
		}
		return nil
	})
}
