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
