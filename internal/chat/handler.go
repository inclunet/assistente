package chat

import (
	"context"
	"errors"
	"log"
)

// ErrConversationGone é retornado por SaveAssistantMessage quando a conversa ou
// mensagem pai foi deletada. O chamador deve abortar o processamento silenciosamente.
var ErrConversationGone = errors.New("conversa deletada ou pai ausente")

// SaveAssistantMessage persiste a resposta final do assistant no banco.
// Retorna ("", nil) se content for vazio ou conversationID == "" (noop).
// Retorna ("", ErrConversationGone) se a conversa foi deletada — o chamador deve abortar.
// Retorna ("", err) para outros erros de banco.
func SaveAssistantMessage(ctx context.Context, msgRepo MessageRepository, opts MessageOptions) (string, error) {
	if opts.ConversationID == "" || opts.Content == "" {
		return "", nil
	}

	msg, err := msgRepo.CreateMessage(ctx, opts)
	if err != nil {
		if errors.Is(err, ErrConversationDeleted) || errors.Is(err, ErrParentMessageDeleted) {
			log.Printf("[Chat] conversa %s deletada — abortando processamento", opts.ConversationID)
			return "", ErrConversationGone
		}
		return "", err
	}

	return msg.ID, nil
}
