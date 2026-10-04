package database

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"strings"
)

// migrateLLMProviderAndCatalogGuards protege a raiz do histórico e o catálogo
// canônico também em bancos que já registraram as migrações anteriores.
// Mudanças futuras do vocabulário exigem remover/reinstalar estes guards na
// própria transação da migração versionada, junto da alteração do catálogo Go.
func migrateLLMProviderAndCatalogGuards(db *gorm.DB) error {
	if db == nil {
		return errors.New("banco inválido para guards de provedor e catálogo")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"llm_providers", "llm_capabilities", "llm_capability_fields"} {
			if !tx.Migrator().HasTable(table) {
				return errMigrationDeferred
			}
		}
		if err := validateLLMCapabilityCatalog(tx); err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TRIGGER IF NOT EXISTS trg_llm_providers_no_replace
   BEFORE INSERT ON llm_providers
   WHEN EXISTS (SELECT 1 FROM llm_providers WHERE id = NEW.id)
   BEGIN SELECT RAISE(ABORT, 'provider identity cannot be replaced'); END`).Error; err != nil {
			return err
		}
		if err := tx.Exec(`CREATE TRIGGER IF NOT EXISTS trg_llm_providers_identity_immutable
   BEFORE UPDATE OF id ON llm_providers
   WHEN NEW.id IS NOT OLD.id
   BEGIN SELECT RAISE(ABORT, 'provider identity is immutable'); END`).Error; err != nil {
			return err
		}
		for _, catalog := range []struct{ table, identity string }{
			{"llm_capabilities", "key = NEW.key"},
			{"llm_capability_fields", "capability_key = NEW.capability_key AND key = NEW.key"},
		} {
			for _, operation := range []string{"UPDATE", "DELETE", "INSERT"} {
				condition := ""
				if operation == "INSERT" {
					condition = "WHEN EXISTS (SELECT 1 FROM " + catalog.table + " WHERE " + catalog.identity + ")"
				}
				statement := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS trg_%s_canonical_%s
     BEFORE %s ON %s %s
     BEGIN SELECT RAISE(ABORT, 'canonical model catalog is immutable'); END`, catalog.table, strings.ToLower(operation), operation, catalog.table, condition)
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}
