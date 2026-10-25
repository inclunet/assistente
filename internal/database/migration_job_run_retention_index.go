package database

import "gorm.io/gorm"

// A exclusão de um lote usa user_id e id IN (...). Índices separados podem
// levar o SQLite a percorrer todos os runs do usuário enquanto mantém o writer.
// O índice composto atende às duas restrições, sem relaxar o isolamento ou FKs.
func ensureJobRunRetentionIndex(db *gorm.DB) error {
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_job_runs_user_id_id ON job_runs (user_id, id)`).Error
}
