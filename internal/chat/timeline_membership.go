package chat

import (
	"context"
	"fmt"

	"assistente/internal/database"
	"gorm.io/gorm"
)

// TimelineMembershipSnapshot preserva a versão que autoriza o conjunto e seus
// índices. Selection.Messages é vazio: participação não materializa conteúdo.
// Contrato interno, ainda sem binding/evento de transporte.
type TimelineMembershipSnapshot struct {
	Version   database.ChatTimelineVersion
	Selection *database.MessageWindowResult
}

type TimelineMembershipReader interface {
	ReadWindowMembership(context.Context, database.MessageWindowQuery) (*TimelineMembershipSnapshot, error)
}

func (r *DBTimelineReader) ReadWindowMembership(ctx context.Context, query database.MessageWindowQuery) (*TimelineMembershipSnapshot, error) {
	result := &TimelineMembershipSnapshot{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		version, err := database.ReadChatTimelineVersion(ctx, tx, query.ConversationID)
		if err != nil {
			return err
		}
		result.Version = version
		repo := database.NewMessageRepository(tx)
		if err := validateTimelineParent(ctx, repo, query); err != nil {
			return err
		}
		result.Selection, err = repo.SelectTimelineWindowWithContext(ctx, query)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validateTimelineParent(ctx context.Context, repo *database.MessageRepository, query database.MessageWindowQuery) error {
	if query.ParentID == nil {
		return nil
	}
	parent, err := repo.GetMessageScopeWithContext(ctx, *query.ParentID)
	if err != nil {
		return fmt.Errorf("threadParentId inválido: %w", err)
	}
	if parent.ConversationID != query.ConversationID {
		return fmt.Errorf("threadParentId não pertence à conversa solicitada")
	}
	if parent.HasParent {
		return fmt.Errorf("threadParentId deve apontar para uma mensagem raiz")
	}
	return nil
}
