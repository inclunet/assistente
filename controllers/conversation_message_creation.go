package controllers

import (
	"context"
	"errors"

	"assistente/internal/database"
)

// Criações avulsas usam o mesmo coordenador das mutações. O pipeline do agente
// já coordena a admissão e continua usando diretamente o repositório de storage.
func (c *ConversationsController) createMessage(ctx context.Context, opts database.MessageOptions) (*database.ChatMessage, error) {
	if c.msgRepo == nil {
		return nil, errors.New("message repository não inicializado")
	}
	var message *database.ChatMessage
	err := c.coordinateMessageMutation(ctx, opts.ConversationID, []string{}, func() ([]string, error) {
		var err error
		message, err = c.msgRepo.CreateMessage(ctx, opts)
		return messageTimelineIDs(message), err
	})
	return message, err
}
