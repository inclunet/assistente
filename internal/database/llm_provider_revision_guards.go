package database

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// migrateLLMProviderRevisionGuards cobre também mutações SQL diretas. A revisão
// avança na mesma transação, sem exigir mudanças nos fluxos OAuth existentes.
func migrateLLMProviderRevisionGuards(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para revisões de provedores")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable(&LLMProvider{}) || !tx.Migrator().HasColumn(&LLMProvider{}, "CompatibilityRevision") || !tx.Migrator().HasColumn(&LLMProvider{}, "ConfigRevision") {
			return errMigrationDeferred
		}
		compatibility := []string{"type", "api_format", "base_url", "reasoning_content_mode"}
		configuration := []string{
			"user_id", "type", "api_format", "base_url", "credential_pattern", "auth_mode",
			"reasoning_content_mode", "acp_command", "acp_args", "acp_env", "acp_credential_env",
			"acp_agent_id", "name", "model", "default_model", "timeout",
			"stream_idle_timeout_seconds", "is_default",
		}
		changed := func(columns []string) string {
			conditions := make([]string, len(columns))
			for i, column := range columns {
				conditions[i] = "NEW." + column + " IS NOT OLD." + column
			}
			return strings.Join(conditions, " OR ")
		}
		statements := []string{
			`CREATE TRIGGER IF NOT EXISTS trg_llm_providers_revision_no_regression BEFORE UPDATE ON llm_providers
    WHEN NEW.compatibility_revision < OLD.compatibility_revision OR NEW.config_revision < OLD.config_revision
    BEGIN SELECT RAISE(ABORT, 'provider revision cannot regress'); END`,
			fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS trg_llm_providers_compatibility_revision_update AFTER UPDATE ON llm_providers
    WHEN NEW.compatibility_revision = OLD.compatibility_revision AND (%s)
    BEGIN UPDATE llm_providers SET compatibility_revision = compatibility_revision + 1 WHERE id = NEW.id; END`, changed(compatibility)),
			fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS trg_llm_providers_config_revision_update AFTER UPDATE ON llm_providers
    WHEN NEW.config_revision = OLD.config_revision AND (%s)
    BEGIN UPDATE llm_providers SET config_revision = config_revision + 1 WHERE id = NEW.id; END`, changed(configuration)),
		}
		for _, statement := range statements {
			if err := tx.Exec(statement).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
