package database

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// MigrateLLMProviderErrorFamilyRevisionGuard atualiza o trigger em bancos que
// já aplicaram a migração 33 antes de error_family participar da revisão.
func MigrateLLMProviderErrorFamilyRevisionGuard(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para revisão da família de erros do provedor")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable(&LLMProvider{}) ||
			!tx.Migrator().HasColumn(&LLMProvider{}, "CompatibilityRevision") ||
			!tx.Migrator().HasColumn(&LLMProvider{}, "ErrorFamily") {
			return errMigrationDeferred
		}
		if err := migrateLLMProviderRevisionGuards(tx); err != nil {
			return fmt.Errorf("atualizar proteção de revisão da família de erros: %w", err)
		}
		return nil
	})
}

// MigrateLLMProviderLegacyErrorFamilies materializes the explicit wizard
// choice for known legacy rows once. The current provider type must still be
// OpenAI-compatible; edited generic providers remain unclassified.
func MigrateLLMProviderLegacyErrorFamilies(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para famílias legadas de erro do provedor")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable(&LLMProvider{}) || !tx.Migrator().HasColumn(&LLMProvider{}, "ErrorFamily") {
			return errMigrationDeferred
		}
		for _, legacy := range []struct{ id, family string }{
			{"openai-default", "openai"},
			{"litellm", "litellm"},
			{"openrouter-default", "openrouter"},
		} {
			if err := tx.Model(&LLMProvider{}).
				Where("id = ? AND type = ? AND (error_family IS NULL OR error_family = '')", legacy.id, "openai").
				Update("error_family", legacy.family).Error; err != nil {
				return fmt.Errorf("classificar provedor legado %q: %w", legacy.id, err)
			}
		}
		return nil
	})
}
