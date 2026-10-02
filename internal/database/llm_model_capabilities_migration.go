package database

import (
	"errors"
	"fmt"

	"assistente/internal/llmcapabilities"
	"gorm.io/gorm"
)

// MigrateLLMModelCapabilities cria o armazenamento estrito da AEP-0113 e
// semeia o vocabulário controlado pela aplicação. É idempotente para retomar
// com segurança após interrupção do boot.
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
		for _, ddl := range llmModelCapabilitiesDDL {
			if err := tx.Exec(ddl).Error; err != nil {
				return err
			}
		}
		for _, ddl := range llmModelCapabilitiesIndexes {
			if err := tx.Exec(ddl).Error; err != nil {
				return err
			}
		}
		if err := seedLLMCapabilityCatalog(tx); err != nil {
			return err
		}
		if err := validateLLMCapabilityCatalog(tx); err != nil {
			return err
		}
		return nil
	})
}

var llmModelCapabilitiesDDL = []string{
	`CREATE TABLE IF NOT EXISTS llm_models (
		id TEXT NOT NULL PRIMARY KEY,
		provider_id TEXT NOT NULL COLLATE BINARY,
		remote_id TEXT NOT NULL COLLATE BINARY CHECK (length(remote_id) > 0 AND trim(remote_id) = remote_id AND instr(remote_id, char(0)) = 0),
		display_name TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		CONSTRAINT fk_llm_models_provider FOREIGN KEY (provider_id) REFERENCES llm_providers(id) ON UPDATE CASCADE ON DELETE CASCADE,
		CONSTRAINT ux_llm_models_provider_remote UNIQUE (provider_id, remote_id)
	)`,
	`CREATE TABLE IF NOT EXISTS llm_capabilities (
		key TEXT NOT NULL PRIMARY KEY CHECK (key IN ('chat','reasoning','tools','input_text','input_image','input_audio','input_video','output_text','output_audio','output_image','output_video','tts','stt','image_generation','music_generation'))
	)`,
	`CREATE TABLE IF NOT EXISTS llm_capability_fields (
		capability_key TEXT NOT NULL COLLATE BINARY,
		key TEXT NOT NULL COLLATE BINARY,
		value_type TEXT NOT NULL CHECK (value_type IN ('number','integer','boolean','string','enum')),
		unit TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (capability_key, key),
		CONSTRAINT fk_llm_capability_fields_capability FOREIGN KEY (capability_key) REFERENCES llm_capabilities(key) ON UPDATE CASCADE ON DELETE RESTRICT,
		CHECK ((capability_key = 'chat' AND key IN ('max_output_tokens','temperature','top_p','frequency_penalty','presence_penalty','seed')) OR
		       (capability_key = 'reasoning' AND key IN ('reasoning_effort','max_reasoning_tokens')) OR
		       (capability_key = 'tools' AND key IN ('parallel_tool_calls')) OR
		       (capability_key = 'tts' AND key IN ('voice','speed','audio_format')) OR
		       (capability_key = 'stt' AND key IN ('language','audio_format')))
	)`,
	`CREATE TABLE IF NOT EXISTS llm_model_catalog_bindings (
		id TEXT NOT NULL PRIMARY KEY,
		model_id TEXT NOT NULL,
		provider_compatibility_revision INTEGER NOT NULL CHECK (provider_compatibility_revision > 0),
		source TEXT NOT NULL CHECK (source IN ('app_curation','official_catalog','third_party_catalog')),
		external_provider_id TEXT NOT NULL COLLATE BINARY CHECK (length(external_provider_id) > 0 AND trim(external_provider_id) = external_provider_id AND instr(external_provider_id, char(0)) = 0),
		external_model_id TEXT NOT NULL COLLATE BINARY CHECK (length(external_model_id) > 0 AND trim(external_model_id) = external_model_id AND instr(external_model_id, char(0)) = 0),
		verified_at DATETIME NOT NULL,
		valid_until DATETIME,
		source_reference TEXT NOT NULL DEFAULT '' CHECK (
			length(source_reference) <= 2048 AND trim(source_reference) = source_reference AND
			instr(source_reference, char(0)) = 0 AND instr(source_reference, char(9)) = 0 AND
			instr(source_reference, char(10)) = 0 AND instr(source_reference, char(13)) = 0 AND
			instr(source_reference, '?') = 0 AND instr(source_reference, '#') = 0 AND instr(source_reference, '@') = 0 AND
			(source_reference = '' OR (lower(substr(source_reference, 1, 7)) = 'http://' AND length(source_reference) > 7 AND substr(source_reference, 8, 1) <> '/') OR
			 (lower(substr(source_reference, 1, 8)) = 'https://' AND length(source_reference) > 8 AND substr(source_reference, 9, 1) <> '/'))),
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		CONSTRAINT fk_llm_catalog_binding_model FOREIGN KEY (model_id) REFERENCES llm_models(id) ON UPDATE CASCADE ON DELETE CASCADE,
		CONSTRAINT ux_llm_catalog_binding_scope UNIQUE (id, model_id, provider_compatibility_revision),
		CONSTRAINT ux_llm_catalog_binding_identity UNIQUE (model_id, provider_compatibility_revision, source, external_provider_id, external_model_id, verified_at),
		CHECK (valid_until IS NULL OR valid_until > verified_at)
	)`,
	`CREATE TABLE IF NOT EXISTS llm_model_capabilities (
		id TEXT NOT NULL PRIMARY KEY,
		model_id TEXT NOT NULL,
		capability_key TEXT NOT NULL,
		support_state TEXT NOT NULL CHECK (support_state IN ('supported','unsupported','unknown')),
		source TEXT NOT NULL CHECK (source IN ('execution_observation','endpoint_discovery','app_curation','official_catalog','third_party_catalog')),
		scope TEXT NOT NULL CHECK (scope IN ('connection','external_binding','generic')),
		provider_compatibility_revision INTEGER NOT NULL CHECK (provider_compatibility_revision > 0),
		binding_id TEXT,
		observed_at DATETIME NOT NULL,
		valid_until DATETIME,
		source_reference TEXT NOT NULL DEFAULT '' CHECK (
			length(source_reference) <= 2048 AND trim(source_reference) = source_reference AND
			instr(source_reference, char(0)) = 0 AND instr(source_reference, char(9)) = 0 AND
			instr(source_reference, char(10)) = 0 AND instr(source_reference, char(13)) = 0 AND
			instr(source_reference, '?') = 0 AND instr(source_reference, '#') = 0 AND instr(source_reference, '@') = 0 AND
			(source_reference = '' OR (lower(substr(source_reference, 1, 7)) = 'http://' AND length(source_reference) > 7 AND substr(source_reference, 8, 1) <> '/') OR
			 (lower(substr(source_reference, 1, 8)) = 'https://' AND length(source_reference) > 8 AND substr(source_reference, 9, 1) <> '/'))),
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		CONSTRAINT fk_llm_model_capability_model FOREIGN KEY (model_id) REFERENCES llm_models(id) ON UPDATE CASCADE ON DELETE CASCADE,
		CONSTRAINT fk_llm_model_capability_key FOREIGN KEY (capability_key) REFERENCES llm_capabilities(key) ON UPDATE CASCADE ON DELETE RESTRICT,
		CONSTRAINT fk_llm_model_capability_binding FOREIGN KEY (binding_id, model_id, provider_compatibility_revision) REFERENCES llm_model_catalog_bindings(id, model_id, provider_compatibility_revision) ON UPDATE CASCADE ON DELETE RESTRICT,
		CHECK (valid_until IS NULL OR valid_until > observed_at),
		CHECK ((scope = 'connection' AND binding_id IS NULL AND source IN ('execution_observation','endpoint_discovery','app_curation')) OR
		       (scope = 'external_binding' AND binding_id IS NOT NULL AND source IN ('app_curation','official_catalog','third_party_catalog')) OR
		       (scope = 'generic' AND binding_id IS NULL AND source IN ('official_catalog','third_party_catalog')))
	)`,
	`CREATE TABLE IF NOT EXISTS llm_model_capability_fields (
		id TEXT NOT NULL PRIMARY KEY,
		model_id TEXT NOT NULL,
		capability_key TEXT NOT NULL,
		field_key TEXT NOT NULL,
		support_state TEXT NOT NULL CHECK (support_state IN ('supported','unsupported','unknown')),
		source TEXT NOT NULL CHECK (source IN ('execution_observation','endpoint_discovery','app_curation','official_catalog','third_party_catalog')),
		scope TEXT NOT NULL CHECK (scope IN ('connection','external_binding','generic')),
		provider_compatibility_revision INTEGER NOT NULL CHECK (provider_compatibility_revision > 0),
		binding_id TEXT,
		minimum REAL,
		maximum REAL,
		step REAL,
		observed_at DATETIME NOT NULL,
		valid_until DATETIME,
		source_reference TEXT NOT NULL DEFAULT '' CHECK (
			length(source_reference) <= 2048 AND trim(source_reference) = source_reference AND
			instr(source_reference, char(0)) = 0 AND instr(source_reference, char(9)) = 0 AND
			instr(source_reference, char(10)) = 0 AND instr(source_reference, char(13)) = 0 AND
			instr(source_reference, '?') = 0 AND instr(source_reference, '#') = 0 AND instr(source_reference, '@') = 0 AND
			(source_reference = '' OR (lower(substr(source_reference, 1, 7)) = 'http://' AND length(source_reference) > 7 AND substr(source_reference, 8, 1) <> '/') OR
			 (lower(substr(source_reference, 1, 8)) = 'https://' AND length(source_reference) > 8 AND substr(source_reference, 9, 1) <> '/'))),
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		CONSTRAINT fk_llm_model_capability_field_model FOREIGN KEY (model_id) REFERENCES llm_models(id) ON UPDATE CASCADE ON DELETE CASCADE,
		CONSTRAINT fk_llm_model_capability_field_catalog FOREIGN KEY (capability_key, field_key) REFERENCES llm_capability_fields(capability_key, key) ON UPDATE CASCADE ON DELETE RESTRICT,
		CONSTRAINT fk_llm_model_capability_field_binding FOREIGN KEY (binding_id, model_id, provider_compatibility_revision) REFERENCES llm_model_catalog_bindings(id, model_id, provider_compatibility_revision) ON UPDATE CASCADE ON DELETE RESTRICT,
		CHECK ((minimum IS NULL OR minimum = minimum) AND (maximum IS NULL OR maximum = maximum) AND (step IS NULL OR (step = step AND step > 0))),
		CHECK (minimum IS NULL OR maximum IS NULL OR minimum <= maximum),
		CHECK (valid_until IS NULL OR valid_until > observed_at),
		CHECK ((scope = 'connection' AND binding_id IS NULL AND source IN ('execution_observation','endpoint_discovery','app_curation')) OR
		       (scope = 'external_binding' AND binding_id IS NOT NULL AND source IN ('app_curation','official_catalog','third_party_catalog')) OR
		       (scope = 'generic' AND binding_id IS NULL AND source IN ('official_catalog','third_party_catalog'))),
		CHECK (support_state = 'supported' OR (minimum IS NULL AND maximum IS NULL AND step IS NULL))
	)`,
	`CREATE TABLE IF NOT EXISTS llm_model_capability_field_options (
		assertion_id TEXT NOT NULL,
		value TEXT NOT NULL COLLATE BINARY CHECK (length(value) > 0 AND length(value) <= 512 AND trim(value) = value AND instr(value, char(0)) = 0),
		label TEXT NOT NULL DEFAULT '' CHECK (length(label) <= 512 AND instr(label, char(0)) = 0),
		support_state TEXT NOT NULL CHECK (support_state IN ('supported','unsupported','unknown')),
		PRIMARY KEY (assertion_id, value),
		CONSTRAINT fk_llm_model_capability_field_option_assertion FOREIGN KEY (assertion_id) REFERENCES llm_model_capability_fields(id) ON UPDATE CASCADE ON DELETE CASCADE
	)`,
}

var llmModelCapabilitiesIndexes = []string{
	`CREATE INDEX IF NOT EXISTS idx_llm_model_capabilities_resolve ON llm_model_capabilities(model_id, provider_compatibility_revision, capability_key, observed_at)`,
	`CREATE INDEX IF NOT EXISTS idx_llm_model_capability_fields_resolve ON llm_model_capability_fields(model_id, provider_compatibility_revision, capability_key, field_key, observed_at)`,
	`CREATE INDEX IF NOT EXISTS idx_llm_model_catalog_bindings_revision ON llm_model_catalog_bindings(model_id, provider_compatibility_revision, valid_until)`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_catalog_bindings_immutable
	 BEFORE UPDATE ON llm_model_catalog_bindings
	 BEGIN SELECT RAISE(ABORT, 'llm_model_catalog_bindings are immutable; insert a new verification'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_binding_insert
	 BEFORE INSERT ON llm_model_capabilities
	 WHEN NEW.scope = 'external_binding' AND NOT EXISTS (
	   SELECT 1 FROM llm_model_catalog_bindings binding
	    WHERE binding.id = NEW.binding_id AND binding.model_id = NEW.model_id
	      AND binding.provider_compatibility_revision = NEW.provider_compatibility_revision
	      AND binding.source = NEW.source AND binding.verified_at <= NEW.observed_at
	      AND (binding.valid_until IS NULL OR binding.valid_until > NEW.observed_at)
	 )
	 BEGIN SELECT RAISE(ABORT, 'llm_model_capabilities requires a matching verified binding'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_field_binding_insert
	 BEFORE INSERT ON llm_model_capability_fields
	 WHEN NEW.scope = 'external_binding' AND NOT EXISTS (
	   SELECT 1 FROM llm_model_catalog_bindings binding
	    WHERE binding.id = NEW.binding_id AND binding.model_id = NEW.model_id
	      AND binding.provider_compatibility_revision = NEW.provider_compatibility_revision
	      AND binding.source = NEW.source AND binding.verified_at <= NEW.observed_at
	      AND (binding.valid_until IS NULL OR binding.valid_until > NEW.observed_at)
	 )
	 BEGIN SELECT RAISE(ABORT, 'llm_model_capability_fields requires a matching verified binding'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_field_typed_insert
	 BEFORE INSERT ON llm_model_capability_fields
	 WHEN NOT EXISTS (
	   SELECT 1 FROM llm_capability_fields f
	    WHERE f.capability_key = NEW.capability_key AND f.key = NEW.field_key
	      AND ((f.value_type IN ('number','integer') AND
	            (NEW.minimum IS NULL OR NEW.minimum BETWEEN -1.7976931348623157e308 AND 1.7976931348623157e308) AND
	            (NEW.maximum IS NULL OR NEW.maximum BETWEEN -1.7976931348623157e308 AND 1.7976931348623157e308) AND
	            (NEW.step IS NULL OR (NEW.step BETWEEN 0 AND 1.7976931348623157e308 AND NEW.step > 0)) AND
	            (f.value_type <> 'integer' OR
	             ((NEW.minimum IS NULL OR NEW.minimum = CAST(NEW.minimum AS INTEGER)) AND
	              (NEW.maximum IS NULL OR NEW.maximum = CAST(NEW.maximum AS INTEGER)) AND
	              (NEW.step IS NULL OR NEW.step = CAST(NEW.step AS INTEGER))))) OR
	           (f.value_type NOT IN ('number','integer') AND NEW.minimum IS NULL AND NEW.maximum IS NULL AND NEW.step IS NULL))
	 )
	 BEGIN SELECT RAISE(ABORT, 'llm_model_capability_fields violates field value type'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_field_typed_update
	 BEFORE UPDATE OF capability_key, field_key, minimum, maximum, step ON llm_model_capability_fields
	 WHEN EXISTS (
	   SELECT 1 FROM llm_capability_fields f
	    WHERE f.capability_key = NEW.capability_key AND f.key = NEW.field_key
	      AND NOT ((f.value_type IN ('number','integer') AND
	                (NEW.minimum IS NULL OR NEW.minimum BETWEEN -1.7976931348623157e308 AND 1.7976931348623157e308) AND
	                (NEW.maximum IS NULL OR NEW.maximum BETWEEN -1.7976931348623157e308 AND 1.7976931348623157e308) AND
	                (NEW.step IS NULL OR (NEW.step BETWEEN 0 AND 1.7976931348623157e308 AND NEW.step > 0)) AND
	                (f.value_type <> 'integer' OR
	                 ((NEW.minimum IS NULL OR NEW.minimum = CAST(NEW.minimum AS INTEGER)) AND
	                  (NEW.maximum IS NULL OR NEW.maximum = CAST(NEW.maximum AS INTEGER)) AND
	                  (NEW.step IS NULL OR NEW.step = CAST(NEW.step AS INTEGER))))) OR
	               (f.value_type NOT IN ('number','integer') AND NEW.minimum IS NULL AND NEW.maximum IS NULL AND NEW.step IS NULL))
	 )
	 BEGIN SELECT RAISE(ABORT, 'llm_model_capability_fields violates field value type'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_field_option_typed_insert
	 BEFORE INSERT ON llm_model_capability_field_options
	 WHEN NOT EXISTS (
	   SELECT 1 FROM llm_model_capability_fields claim
	   JOIN llm_capability_fields field ON field.capability_key = claim.capability_key AND field.key = claim.field_key
	    WHERE claim.id = NEW.assertion_id AND claim.support_state = 'supported' AND field.value_type = 'enum'
	 )
	 BEGIN SELECT RAISE(ABORT, 'llm_model_capability_field_options requires a supported enum field'); END`,
	`CREATE TRIGGER IF NOT EXISTS trg_llm_model_capability_field_option_typed_update
	 BEFORE UPDATE OF assertion_id, value ON llm_model_capability_field_options
	 WHEN NOT EXISTS (
	   SELECT 1 FROM llm_model_capability_fields claim
	   JOIN llm_capability_fields field ON field.capability_key = claim.capability_key AND field.key = claim.field_key
	    WHERE claim.id = NEW.assertion_id AND claim.support_state = 'supported' AND field.value_type = 'enum'
	 )
	 BEGIN SELECT RAISE(ABORT, 'llm_model_capability_field_options requires a supported enum field'); END`,
}

func seedLLMCapabilityCatalog(db *gorm.DB) error {
	for _, capability := range llmcapabilities.Capabilities() {
		if err := db.Exec(`INSERT INTO llm_capabilities(key) VALUES (?) ON CONFLICT(key) DO NOTHING`, string(capability)).Error; err != nil {
			return err
		}
	}
	for _, field := range llmcapabilities.Fields() {
		if err := db.Exec(`INSERT INTO llm_capability_fields(capability_key, key, value_type, unit) VALUES (?, ?, ?, ?) ON CONFLICT(capability_key, key) DO NOTHING`, string(field.Capability), string(field.Key), string(field.ValueType), field.Unit).Error; err != nil {
			return err
		}
	}
	return nil
}

func validateLLMCapabilityCatalog(db *gorm.DB) error {
	var capabilities []string
	if err := db.Raw(`SELECT key FROM llm_capabilities ORDER BY key`).Scan(&capabilities).Error; err != nil {
		return err
	}
	wantCapabilities := llmcapabilities.Capabilities()
	if len(capabilities) != len(wantCapabilities) {
		return errors.New("catálogo de capabilities de modelos divergente do catálogo controlado")
	}
	for i, key := range capabilities {
		if key != string(wantCapabilities[i]) {
			return errors.New("catálogo de capabilities de modelos contém identificador desconhecido")
		}
	}
	var rows []struct {
		CapabilityKey string
		Key           string
		ValueType     string
		Unit          string
	}
	if err := db.Raw(`SELECT capability_key, key, value_type, unit FROM llm_capability_fields ORDER BY capability_key, key`).Scan(&rows).Error; err != nil {
		return err
	}
	wantFields := llmcapabilities.Fields()
	if len(rows) != len(wantFields) {
		return errors.New("catálogo de campos de modelos divergente do catálogo controlado")
	}
	for i, row := range rows {
		want := wantFields[i]
		if row.CapabilityKey != string(want.Capability) || row.Key != string(want.Key) || row.ValueType != string(want.ValueType) || row.Unit != want.Unit {
			return fmt.Errorf("definição divergente para campo de modelo %s.%s", row.CapabilityKey, row.Key)
		}
	}
	return nil
}
