package database

import "context"

// ==================== Rolling Context (Summary) ====================

// GetConversationSummaryWithContext retorna o resumo e o ID da última mensagem
// resumida de uma conversa do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired — não cabe ler resumo cross-user.
func GetConversationSummaryWithContext(ctx context.Context, conversationID string) (summary string, upToMessageID string, err error) {
	if _, err := RequireUserID(ctx); err != nil {
		return "", "", err
	}
	var conv Conversation
	err = ScopeByUser(ctx, db.WithContext(ctx).Select("summary", "summary_up_to_message_id"), "user_id").First(&conv, "id = ?", conversationID).Error
	if err != nil {
		return "", "", err
	}
	return conv.Summary, conv.SummaryUpToMessageID, nil
}

// UpdateConversationSummaryWithContext atualiza o resumo de uma conversa do
// usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired — escrita global de summary é vetor de poluição
// cross-user.
func UpdateConversationSummaryWithContext(ctx context.Context, conversationID string, summary string, upToMessageID string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").Where("id = ?", conversationID).Updates(map[string]interface{}{
		"summary":                  summary,
		"summary_up_to_message_id": upToMessageID,
		"summarizing_in_progress":  false,
	}).Error
}

// SetSummarizingInProgressWithContext marca se uma sumarização está em
// andamento para o usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired.
func SetSummarizingInProgressWithContext(ctx context.Context, conversationID string, inProgress bool) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").Where("id = ?", conversationID).
		Update("summarizing_in_progress", inProgress).Error
}

// IsSummarizingInProgressWithContext verifica se há sumarização em andamento
// para o usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired.
func IsSummarizingInProgressWithContext(ctx context.Context, conversationID string) (bool, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return false, err
	}
	var conv Conversation
	err := ScopeByUser(ctx, db.WithContext(ctx).Select("summarizing_in_progress"), "user_id").First(&conv, "id = ?", conversationID).Error
	if err != nil {
		return false, err
	}
	return conv.SummarizingInProgress, nil
}
