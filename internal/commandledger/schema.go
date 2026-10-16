package commandledger

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Migrate cria somente o schema do ledger. Constraints condicionais dos
// grupos avançados de D11 (surface, conversa/turno, delegation, job, policy,
// actor/provenance e replay de eventos) ainda não são habilitadas pela API
// interna local_session; a validação fica para a fase que expuser esses fluxos.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if db == nil || ctx == nil {
		return fmt.Errorf("commandledger: banco nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&ledgerRow{}, &invocationRow{}, &closedGenerationRow{}); err != nil {
			return err
		}
		if err := tx.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS ux_command_idempotency_keys_source_event_id
ON command_idempotency_keys (source_event_id)
WHERE source_event_id IS NOT NULL
`).Error; err != nil {
			return err
		}
		// NULL não participa de UNIQUE no SQLite; índices parciais separados
		// tornam o ciclo system e os ciclos de usuário idempotentes sem FK.
		if err := tx.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS ux_command_closed_generation_user
ON command_closed_generations (user_id, auth_context_type, auth_context_id, security_generation)
WHERE user_id IS NOT NULL
`).Error; err != nil {
			return err
		}
		return tx.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS ux_command_closed_generation_system
ON command_closed_generations (auth_context_type, auth_context_id, security_generation)
WHERE user_id IS NULL
`).Error
	})
}
