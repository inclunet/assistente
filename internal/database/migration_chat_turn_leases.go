package database

import "gorm.io/gorm"

// Migração aditiva; v41 ainda não admitia workers em produção. O endereço da
// posse deve acompanhar a tentativa, não depender do ambiente de quem recupera.
func migrateChatTurnLeases(db *gorm.DB) error {
	if db.Migrator().HasColumn("chat_turn_attempts", "lease_path") {
		return nil
	}
	return db.Exec("ALTER TABLE chat_turn_attempts ADD COLUMN lease_path TEXT NOT NULL DEFAULT ''").Error
}
