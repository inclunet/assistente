package database

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ApplyCommandStorageMigration é uma porta do bootstrap confiável para a v21
// do registro central. Não aceita versão/nome de cliente nem usa banco global.
// A composição e o carimbo pertencem à mesma transação; callbacks não devem
// acessar cofre/UI nem usar outra conexão.
func ApplyCommandStorageMigration(ctx context.Context, db *gorm.DB, apply func(*gorm.DB) error) error {
	return applyCommandMigration(ctx, db, 21, "command_storage_initial", apply)
}

// ApplyCommandEnvelopeMigration amplia o schema conhecido v21 sem reescrever
// seu carimbo. O bootstrap valida o schema de origem antes de qualquer DDL.
func ApplyCommandEnvelopeMigration(ctx context.Context, db *gorm.DB, apply func(*gorm.DB) error) error {
	return applyCommandMigration(ctx, db, 22, "command_envelope_ownership", apply)
}

func ApplyCommandConfigMigration(ctx context.Context, db *gorm.DB, apply func(*gorm.DB) error) error {
	return applyCommandMigration(ctx, db, 23, "command_config_complete", apply)
}

func ApplyCommandActivationMigration(ctx context.Context, db *gorm.DB, apply func(*gorm.DB) error) error {
	return applyCommandMigration(ctx, db, 24, "command_activation_durable", apply)
}

func ApplyCommandJobActivationMigration(ctx context.Context, db *gorm.DB, apply func(*gorm.DB) error) error {
	return applyCommandMigration(ctx, db, 27, "command_job_activation_consumer", apply)
}

func ApplyCommandImportMigration(ctx context.Context, db *gorm.DB, apply func(*gorm.DB) error) error {
	return applyCommandMigration(ctx, db, 28, "command_config_import_audit", apply)
}

func applyCommandMigration(ctx context.Context, db *gorm.DB, version int, name string, apply func(*gorm.DB) error) error {
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
		if err := tx.Raw("SELECT name FROM schema_migrations WHERE version = ?", version).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 1 || (len(rows) == 1 && rows[0].Name != name) {
			return errors.New("versão de comandos incompatível")
		}
		return runMigrationList(tx, phasePostAutoMigrate, []migration{{
			Version: version, Name: name, Phase: phasePostAutoMigrate, Run: apply,
		}})
	})
}
