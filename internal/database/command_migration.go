package database

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ApplyCommandStorageMigration é uma porta do bootstrap confiável para a v20
// do registro central. Não aceita versão/nome de cliente nem usa banco global.
// A composição e o carimbo pertencem à mesma transação; callbacks não devem
// acessar cofre/UI nem usar outra conexão.
func ApplyCommandStorageMigration(ctx context.Context, db *gorm.DB, apply func(*gorm.DB) error) error {
	if ctx == nil || db == nil || apply == nil {
		return errors.New("migração de comandos inválida")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ensureSchemaMigrationsTable(tx); err != nil {
			return err
		}
		var rows []struct{ Name string }
		if err := tx.Raw("SELECT name FROM schema_migrations WHERE version = 20").Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 1 || (len(rows) == 1 && rows[0].Name != "command_storage_initial") {
			return errors.New("versão de comandos incompatível")
		}
		return runMigrationList(tx, phasePostAutoMigrate, []migration{{
			Version: 20, Name: "command_storage_initial", Phase: phasePostAutoMigrate, Run: apply,
		}})
	})
}
