package database

import "gorm.io/gorm"

// A exclusão nativa encerra o vínculo independentemente do caminho de acesso
// ao SQLite. Restaurar um backup com o mesmo ID cria um cadastro independente.
func installProviderBindingCleanup(db *gorm.DB) error {
	if !db.Migrator().HasTable(&PluginProviderBinding{}) || !db.Migrator().HasTable(&LLMProvider{}) {
		return errMigrationDeferred
	}
	return db.Exec("CREATE TRIGGER IF NOT EXISTS forget_deleted_plugin_provider AFTER DELETE ON llm_providers BEGIN DELETE FROM plugin_provider_bindings WHERE provider_id=OLD.id; END").Error
}
