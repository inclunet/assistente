package database

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
)

// migrateLLMModelHistoryInsertGuards impede REPLACE de reescrever registros
// históricos, inclusive com recursive_triggers desligado. Não altera linhas.
func migrateLLMModelHistoryInsertGuards(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para proteção do histórico de modelos")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, guard := range []struct{ table, conflict string }{
			{"llm_model_catalog_bindings", "id = NEW.id OR (model_id = NEW.model_id AND provider_compatibility_revision = NEW.provider_compatibility_revision AND source = NEW.source AND external_provider_id = NEW.external_provider_id AND external_model_id = NEW.external_model_id AND verified_at = NEW.verified_at)"},
			{"llm_model_capabilities", "id = NEW.id"},
			{"llm_model_capability_fields", "id = NEW.id"},
			{"llm_model_capability_field_options", "assertion_id = NEW.assertion_id AND value = NEW.value"},
		} {
			if !tx.Migrator().HasTable(guard.table) {
				return errMigrationDeferred
			}
			statement := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS trg_%s_no_replace
                BEFORE INSERT ON %s WHEN EXISTS (SELECT 1 FROM %s WHERE %s)
                BEGIN SELECT RAISE(ABORT, 'model history cannot be replaced'); END`,
				guard.table, guard.table, guard.table, guard.conflict)
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
