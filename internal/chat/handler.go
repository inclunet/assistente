package chat

import (
	"context"
	"errors"
	"log"
	"strings"
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

// EnsureAssistantPlaceholder garante que exista uma mensagem root de assistant para o turno
// (turnID aponta para a user message). Retorna o ID da mensagem existente ou recém-criada.
//
// Nota: este placeholder pode começar com Content vazio e será atualizado ao final do streaming.
func EnsureAssistantPlaceholder(ctx context.Context, msgRepo MessageRepository, conversationID string, turnID string) (string, error) {
	conversationID = strings.TrimSpace(conversationID)
	turnID = strings.TrimSpace(turnID)
	if conversationID == "" || turnID == "" || msgRepo == nil {
		return "", nil
	}

	// Reusa uma mensagem assistant root já existente no turno (se houver).
	msgs, err := msgRepo.GetMessagesByTurnID(ctx, conversationID, nil, turnID, 100)
	if err != nil {
		return "", err
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role == "assistant" {
			return m.ID, nil
		}
	}

	// Cria placeholder vazio.
	opts := MessageOptions{
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        "",
		TurnID:         &turnID,
	}
	msg, err := msgRepo.CreateMessage(ctx, opts)
	if err != nil {
		if errors.Is(err, ErrConversationDeleted) || errors.Is(err, ErrParentMessageDeleted) {
			log.Printf("[Chat] conversa %s deletada — abortando placeholder", conversationID)
			return "", ErrConversationGone
		}
		return "", err
	}
	return msg.ID, nil
}

// FinalizeAssistantMessage persiste a resposta final no banco, preferindo atualizar
// um placeholder existente (assistantMessageID) para manter um messageId estável.
func FinalizeAssistantMessage(ctx context.Context, msgRepo MessageRepository, assistantMessageID string, opts MessageOptions) (string, error) {
	assistantMessageID = strings.TrimSpace(assistantMessageID)
	if opts.ConversationID == "" {
		return assistantMessageID, nil
	}
	if opts.Content == "" {
		// Nada para persistir; mantém o placeholder (se existir) como está.
		return assistantMessageID, nil
	}
	if msgRepo == nil {
		return "", nil
	}

	if assistantMessageID != "" {
		if err := msgRepo.UpdateMessageContentAndReasoning(ctx, assistantMessageID, opts.Content, opts.Reasoning, opts.PromptTokens, opts.CompletionTokens, opts.TotalTokens, opts.Model); err != nil {
			return assistantMessageID, err
		}
		return assistantMessageID, nil
	}

	// Fallback: cria uma mensagem nova (comportamento legado).
	return SaveAssistantMessage(ctx, msgRepo, opts)
}
