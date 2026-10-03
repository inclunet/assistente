package database

import (
	"errors"
	"gorm.io/gorm"
)

// migrateLLMModelIdentityAndOptionSeals preserva a identidade dos fatos e fecha
// listas de opções na mesma transação que publica a afirmação de campo.
func migrateLLMModelIdentityAndOptionSeals(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para identidade e opções de modelos")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"llm_models", "llm_model_capability_fields", "llm_model_capability_field_options"} {
			if !tx.Migrator().HasTable(table) {
				return errMigrationDeferred
			}
		}
		legacy := !tx.Migrator().HasTable("llm_model_capability_field_seals")
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS llm_model_capability_field_seals (
            assertion_id TEXT NOT NULL PRIMARY KEY,
            FOREIGN KEY (assertion_id) REFERENCES llm_model_capability_fields(id) ON DELETE CASCADE
        )`).Error; err != nil {
			return err
		}
		if legacy {
			if err := tx.Exec(`INSERT INTO llm_model_capability_field_seals (assertion_id)
                SELECT id FROM llm_model_capability_fields`).Error; err != nil {
				return err
			}
		}
		for _, statement := range []string{
			`CREATE TRIGGER IF NOT EXISTS trg_llm_models_identity_no_replace
             BEFORE INSERT ON llm_models
             WHEN EXISTS (SELECT 1 FROM llm_models WHERE id = NEW.id
                 OR (provider_id = NEW.provider_id AND remote_id = NEW.remote_id))
             BEGIN SELECT RAISE(ABORT, 'model identity cannot be replaced'); END`,
			`CREATE TRIGGER IF NOT EXISTS trg_llm_models_identity_immutable
             BEFORE UPDATE OF provider_id,remote_id ON llm_models
             WHEN NEW.provider_id IS NOT OLD.provider_id OR NEW.remote_id IS NOT OLD.remote_id
             BEGIN SELECT RAISE(ABORT, 'model identity is immutable'); END`,
			`CREATE TRIGGER IF NOT EXISTS trg_llm_model_field_options_sealed
             BEFORE INSERT ON llm_model_capability_field_options
             WHEN EXISTS (SELECT 1 FROM llm_model_capability_field_seals WHERE assertion_id = NEW.assertion_id)
             BEGIN SELECT RAISE(ABORT, 'field option set is sealed'); END`,
			`CREATE TRIGGER IF NOT EXISTS trg_llm_model_field_seals_immutable
             BEFORE UPDATE ON llm_model_capability_field_seals
             BEGIN SELECT RAISE(ABORT, 'field option seal is immutable'); END`,
			`CREATE TRIGGER IF NOT EXISTS trg_llm_model_field_seals_append_only
             BEFORE DELETE ON llm_model_capability_field_seals
             WHEN EXISTS (SELECT 1 FROM llm_model_capability_fields WHERE id = OLD.assertion_id)
             BEGIN SELECT RAISE(ABORT, 'field option seal is append-only'); END`,
			`CREATE TRIGGER IF NOT EXISTS trg_llm_model_field_seals_no_replace
             BEFORE INSERT ON llm_model_capability_field_seals
             WHEN EXISTS (SELECT 1 FROM llm_model_capability_field_seals WHERE assertion_id = NEW.assertion_id)
             BEGIN SELECT RAISE(ABORT, 'field option seal cannot be replaced'); END`,
		} {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
