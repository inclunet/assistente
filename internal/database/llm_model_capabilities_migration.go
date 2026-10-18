package database

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// MigrateLLMModelCapabilities cria a persistência mínima da AEP-0113.
// A migração é idempotente para que o boot possa retomá-la após interrupção.
func MigrateLLMModelCapabilities(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para capabilities de modelos")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable(&LLMProvider{}) {
			return errMigrationDeferred
		}
		if !tx.Migrator().HasColumn(&LLMProvider{}, "CompatibilityRevision") {
			if err := tx.Exec(`ALTER TABLE llm_providers ADD COLUMN compatibility_revision INTEGER NOT NULL DEFAULT 1 CHECK (compatibility_revision > 0)`).Error; err != nil {
				return fmt.Errorf("adicionar revisão de compatibilidade do provedor: %w", err)
			}
		}
		for _, statement := range llmModelCapabilityDDL {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		if err := migrateLLMProviderRevisionGuards(tx); err != nil {
			return fmt.Errorf("proteger revisões do provedor: %w", err)
		}
		return nil
	})
}

var llmModelCapabilityDDL = []string{
	`CREATE TABLE IF NOT EXISTS llm_models (
		id TEXT NOT NULL PRIMARY KEY,
		provider_id TEXT NOT NULL COLLATE BINARY,
		remote_id TEXT NOT NULL COLLATE BINARY
			CHECK (length(remote_id) BETWEEN 1 AND 512 AND trim(remote_id) = remote_id AND instr(remote_id, char(0)) = 0),
		display_name TEXT NOT NULL DEFAULT ''
			CHECK (length(display_name) <= 512 AND instr(display_name, char(0)) = 0),
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		CONSTRAINT fk_llm_models_provider FOREIGN KEY (provider_id)
			REFERENCES llm_providers(id) ON UPDATE CASCADE ON DELETE CASCADE,
		CONSTRAINT ux_llm_models_provider_remote UNIQUE (provider_id, remote_id)
	)`,
	`CREATE TABLE IF NOT EXISTS llm_model_capabilities (
		id TEXT NOT NULL PRIMARY KEY,
		model_id TEXT NOT NULL,
		capability_code TEXT NOT NULL COLLATE BINARY CHECK (
			capability_code IN ('chat.completions','responses','tts','stt')
		),
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		CONSTRAINT fk_llm_model_capabilities_model FOREIGN KEY (model_id)
			REFERENCES llm_models(id) ON UPDATE CASCADE ON DELETE CASCADE,
		CONSTRAINT ux_llm_model_capabilities_model_code UNIQUE (model_id, capability_code)
	)`,
	`CREATE TABLE IF NOT EXISTS llm_model_capability_fields (
		capability_id TEXT NOT NULL,
		field_code TEXT NOT NULL COLLATE BINARY CHECK (
			field_code IN ('max_output_tokens','temperature','top_p','frequency_penalty','presence_penalty','seed','reasoning_effort','max_reasoning_tokens','parallel_tool_calls','voice','speed','audio_format','language')
		),
		compatibility_revision INTEGER NOT NULL CHECK (compatibility_revision > 0),
		recognizer_id TEXT NOT NULL CHECK (length(recognizer_id) BETWEEN 1 AND 128),
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (capability_id, field_code),
		CONSTRAINT fk_llm_model_capability_fields_capability FOREIGN KEY (capability_id)
			REFERENCES llm_model_capabilities(id) ON UPDATE CASCADE ON DELETE CASCADE
	)`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capabilities_identity_immutable
		BEFORE UPDATE OF model_id, capability_code ON llm_model_capabilities
		WHEN NEW.model_id IS NOT OLD.model_id OR NEW.capability_code IS NOT OLD.capability_code
		BEGIN SELECT RAISE(ABORT, 'llm model capability identity is immutable'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capabilities_no_replace
		BEFORE INSERT ON llm_model_capabilities
		WHEN EXISTS (SELECT 1 FROM llm_model_capabilities WHERE id = NEW.id
			OR (model_id = NEW.model_id AND capability_code = NEW.capability_code))
		BEGIN SELECT RAISE(ABORT, 'llm model capability cannot be replaced'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_providers_no_replace
		BEFORE INSERT ON llm_providers
		WHEN EXISTS (SELECT 1 FROM llm_providers WHERE id = NEW.id)
		BEGIN SELECT RAISE(ABORT, 'provider identity cannot be replaced'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_providers_identity_immutable
		BEFORE UPDATE OF id ON llm_providers
		WHEN NEW.id IS NOT OLD.id
		BEGIN SELECT RAISE(ABORT, 'provider identity is immutable'); END`,
	`CREATE INDEX IF NOT EXISTS idx_llm_models_provider ON llm_models(provider_id)`,
	`CREATE INDEX IF NOT EXISTS idx_llm_model_capability_fields_revision
		ON llm_model_capability_fields(capability_id, compatibility_revision)`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_fields_field_insert
		BEFORE INSERT ON llm_model_capability_fields
		WHEN NOT EXISTS (
			SELECT 1 FROM llm_model_capabilities c
			WHERE c.id = NEW.capability_id AND (
				(c.capability_code = 'chat.completions' AND NEW.field_code IN ('max_output_tokens','temperature','top_p','frequency_penalty','presence_penalty','seed','reasoning_effort','max_reasoning_tokens','parallel_tool_calls')) OR
				(c.capability_code = 'responses' AND NEW.field_code IN ('max_output_tokens','temperature','top_p','reasoning_effort','max_reasoning_tokens','parallel_tool_calls')) OR
				(c.capability_code = 'tts' AND NEW.field_code IN ('voice','speed','audio_format')) OR
				(c.capability_code = 'stt' AND NEW.field_code IN ('language','audio_format'))
			)
		)
		BEGIN SELECT RAISE(ABORT, 'unknown model capability field'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_fields_identity_immutable
		BEFORE UPDATE OF capability_id, field_code ON llm_model_capability_fields
		WHEN NEW.capability_id IS NOT OLD.capability_id OR NEW.field_code IS NOT OLD.field_code
		BEGIN SELECT RAISE(ABORT, 'llm model capability field identity is immutable'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_fields_field_update
		BEFORE UPDATE OF capability_id, field_code ON llm_model_capability_fields
		WHEN NOT EXISTS (
			SELECT 1 FROM llm_model_capabilities c
			WHERE c.id = NEW.capability_id AND (
				(c.capability_code = 'chat.completions' AND NEW.field_code IN ('max_output_tokens','temperature','top_p','frequency_penalty','presence_penalty','seed','reasoning_effort','max_reasoning_tokens','parallel_tool_calls')) OR
				(c.capability_code = 'responses' AND NEW.field_code IN ('max_output_tokens','temperature','top_p','reasoning_effort','max_reasoning_tokens','parallel_tool_calls')) OR
				(c.capability_code = 'tts' AND NEW.field_code IN ('voice','speed','audio_format')) OR
				(c.capability_code = 'stt' AND NEW.field_code IN ('language','audio_format'))
			)
		)
		BEGIN SELECT RAISE(ABORT, 'unknown model capability field'); END`, `CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_fields_revision_insert
		BEFORE INSERT ON llm_model_capability_fields
		WHEN NEW.compatibility_revision <> COALESCE((
			SELECT p.compatibility_revision
			FROM llm_model_capabilities c
			JOIN llm_models m ON m.id = c.model_id
			JOIN llm_providers p ON p.id = m.provider_id
			WHERE c.id = NEW.capability_id
		), 0)
		BEGIN SELECT RAISE(ABORT, 'stale model compatibility revision'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_fields_revision_update
		BEFORE UPDATE OF compatibility_revision ON llm_model_capability_fields
		WHEN NEW.compatibility_revision <> COALESCE((
			SELECT p.compatibility_revision
			FROM llm_model_capabilities c
			JOIN llm_models m ON m.id = c.model_id
			JOIN llm_providers p ON p.id = m.provider_id
			WHERE c.id = NEW.capability_id
		), 0)
		BEGIN SELECT RAISE(ABORT, 'stale model compatibility revision'); END`,
	`DROP TRIGGER IF EXISTS trg_llm_models_identity_immutable`,
	`CREATE TRIGGER trg_llm_models_identity_immutable
		BEFORE UPDATE OF provider_id, remote_id ON llm_models
		WHEN NEW.provider_id IS NOT OLD.provider_id OR NEW.remote_id IS NOT OLD.remote_id
		BEGIN SELECT RAISE(ABORT, 'llm model identity is immutable'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_models_identity_no_replace
		BEFORE INSERT ON llm_models
		WHEN EXISTS (SELECT 1 FROM llm_models WHERE id = NEW.id
			OR (provider_id = NEW.provider_id AND remote_id = NEW.remote_id))
		BEGIN SELECT RAISE(ABORT, 'model identity cannot be replaced'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_providers_invalidate_model_fields
		AFTER UPDATE OF compatibility_revision ON llm_providers
		WHEN NEW.compatibility_revision <> OLD.compatibility_revision
		BEGIN
			DELETE FROM llm_model_capability_fields
			WHERE capability_id IN (
				SELECT c.id FROM llm_model_capabilities c
				JOIN llm_models m ON m.id = c.model_id
				WHERE m.provider_id = NEW.id
			);
			DELETE FROM llm_model_capabilities
			WHERE model_id IN (SELECT id FROM llm_models WHERE provider_id = NEW.id);
		END`,
}
