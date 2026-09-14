package toolinvocations

import (
	"context"

	"assistente/internal/database"
)

// LegacyReadPolicy informa quais conversas ainda dependem de L1/L3 durante o
// backfill. Ausência de checkpoint significa ledger-only: conversas sem legado
// nunca recebem estado, e todo recurso legado detectado recebe pending.
type LegacyReadPolicy = database.ToolLedgerLegacyReadPolicy

// LoadLegacyReadPolicyWithUser resolve o gate em lote para evitar uma consulta
// por turno/conversa. Somente estado explicitamente pending autoriza legado.
func LoadLegacyReadPolicyWithUser(ctx context.Context, userID string, conversationIDs []string) (LegacyReadPolicy, error) {
	return database.LoadToolLedgerLegacyReadPolicyWithUser(ctx, userID, conversationIDs)
}
