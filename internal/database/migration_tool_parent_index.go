package database

import "gorm.io/gorm"

// O índice por pai isolado compete com os índices por usuário. Sem estatísticas,
// o SQLite pode escolher percorrer todo o ledger do usuário para desassociar
// poucos filhos. As duas chaves juntas atendem ao predicado real da limpeza.
// Só filhos ocupam o novo índice; raízes não precisam dessa busca.
// Não substitui o índice legado: FKs e bancos parcialmente migrados o utilizam.
func ensureToolInvocationParentRetentionIndex(database *gorm.DB) error {
	return database.Exec(`CREATE INDEX IF NOT EXISTS idx_tool_invocations_parent_user
		ON tool_invocations (parent_invocation_id, user_id)
		WHERE parent_invocation_id IS NOT NULL`).Error
}
