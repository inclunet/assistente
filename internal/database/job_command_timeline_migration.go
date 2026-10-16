package database

import (
	"errors"
	"gorm.io/gorm"
)

// Precede AutoMigrate: SQLite não adiciona NOT NULL a tabela populada sem
// default. O backfill histórico usa o início já registrado, nunca inventa
// root_origin/proveniência ou eventos elegíveis retroativos.
func migrateCommandJobQueuedAt(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable("job_runs") {
			return nil
		}
		if !tx.Migrator().HasColumn("job_runs", "started_at") {
			return errors.New("job_runs sem started_at histórico")
		}
		if !tx.Migrator().HasColumn("job_runs", "queued_at") {
			if err := tx.Exec("ALTER TABLE job_runs ADD COLUMN queued_at datetime").Error; err != nil {
				return err
			}
		}
		if err := tx.Exec("UPDATE job_runs SET queued_at = started_at WHERE queued_at IS NULL AND started_at IS NOT NULL").Error; err != nil {
			return err
		}
		var missing int64
		if err := tx.Table("job_runs").Where("queued_at IS NULL").Count(&missing).Error; err != nil {
			return err
		}
		if missing != 0 {
			return errors.New("job_runs sem timestamp autoritativo para queued_at")
		}
		return nil
	})
}
