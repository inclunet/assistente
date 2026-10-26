package database

import "gorm.io/gorm"

// Permite avançar uma página de chat por usuário sem ordenar ou varrer o histórico.
func ensureToolInvocationOrphanScanIndex(db *gorm.DB) error {
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tool_invocations_orphan_scan ON tool_invocations (user_id, origin_type, id)`).Error; err != nil {
		return err
	}
	// Filhos podem ter outra origem; detach e DELETE por IDs precisam de acesso direto também.
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_tool_invocations_user_id_id ON tool_invocations (user_id, id)`).Error
}
