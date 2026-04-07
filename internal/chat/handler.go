package chat

import (
	"errors"
	"fmt"

	"assistente/internal/database"
)

// ErrConversationGone é retornado por SaveAssistantMessage quando a conversa ou
// mensagem pai foi deletada. O chamador deve abortar o processamento silenciosamente.
var ErrConversationGone = errors.New("conversa deletada ou pai ausente")

// SaveAssistantMessage persiste a resposta final do assistant no banco.
// Retorna (0, nil) se content for vazio ou conversationID == 0 (noop).
// Retorna (0, ErrConversationGone) se a conversa foi deletada — o chamador deve abortar.
// Retorna (0, err) para outros erros de banco.
func SaveAssistantMessage(msgRepo MessageRepository, opts MessageOptions) (uint, error) {
	if opts.ConversationID == 0 || opts.Content == "" {
		return 0, nil
	}

	msg, err := msgRepo.CreateMessage(opts)
	if err != nil {
		if errors.Is(err, database.ErrConversationDeleted) || errors.Is(err, database.ErrParentMessageDeleted) {
			fmt.Printf("🛑 Conversa %d foi deletada/limpa — abortando processamento\n", opts.ConversationID)
			return 0, ErrConversationGone
		}
		return 0, err
	}

	return msg.ID, nil
}
