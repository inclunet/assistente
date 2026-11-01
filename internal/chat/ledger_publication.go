package chat

import (
	"context"

	"assistente/internal/toolinvocations"
	"gorm.io/gorm"
)

// LedgerMutationCoordinator conecta o ledger ao projetor compartilhado. A
// descoberta da origem é uma leitura sem lock de writer; a ordem de locks da
// mutação é sempre projetor -> transação SQLite.
type LedgerMutationCoordinator struct {
	projection *LiveProjection
	db         *gorm.DB
}

func NewLedgerMutationCoordinator(projection *LiveProjection, db *gorm.DB) *LedgerMutationCoordinator {
	return &LedgerMutationCoordinator{projection: projection, db: db}
}

func (c *LedgerMutationCoordinator) MutateInvocation(ctx context.Context, inv *toolinvocations.Invocation, write func() error) error {
	conversationID, turnID := inv.ConversationID, inv.TurnID
	if inv.ID == "" || conversationID == "" || turnID == "" {
		// Chamadores legados informam somente OriginID. Resolver metadados com
		// ownership, sem carregar o conteúdo da mensagem; Create revalida no commit.
		var err error
		conversationID, turnID, err = toolinvocations.NewDBRepository(c.db).ResolveChatOrigin(ctx, inv.OriginID)
		if err != nil {
			return err
		}
	}
	return c.projection.ApplyStorageMutation(ctx, conversationID, []string{"turn:" + turnID}, func() ([]string, error) { return nil, write() })
}
