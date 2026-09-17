package database

import (
	"context"
	"fmt"
	"slices"

	"gorm.io/gorm"
)

// LoadHistoryWindowThroughMessageWithContext lê o contexto de retry sem
// materializar as mensagens posteriores nem todo o prefixo da conversa.
// As duas primeiras mensagens e a cauda limitada preservam a política do
// HistoryLoader. Todas as leituras compartilham um snapshot e o escopo do usuário.
func (r *MessageRepository) LoadHistoryWindowThroughMessageWithContext(ctx context.Context, conversationID, messageID string, maxMessages int) (*HistoryWindowResult, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if maxMessages <= 0 {
		return nil, fmt.Errorf("maxMessages deve ser positivo")
	}
	var result *HistoryWindowResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var conv Conversation
		if err := ScopeByUser(ctx, tx, "user_id").
			Select("id", "summary", "summary_up_to_message_id").
			First(&conv, "id = ?", conversationID).Error; err != nil {
			return err
		}
		var target ChatMessage
		if err := tx.Select("id", "created_at").
			Where("id = ? AND conversation_id = ? AND parent_id IS NULL AND role = ?", messageID, conv.ID, "user").
			First(&target).Error; err != nil {
			return err
		}
		// A comparação composta usa o mesmo desempate da ordenação canônica,
		// inclusive para mensagens com o mesmo timestamp.
		query := tx.Model(&ChatMessage{}).
			Where("conversation_id = ? AND parent_id IS NULL", conv.ID).
			Where("(created_at, id) <= (?, ?)", target.CreatedAt, target.ID)
		summary := ""
		boundaryAvailable := false
		if conv.SummaryUpToMessageID != "" {
			var boundary ChatMessage
			found := tx.Select("id", "created_at").
				Where("id = ? AND conversation_id = ? AND parent_id IS NULL", conv.SummaryUpToMessageID, conv.ID).
				Where("(created_at, id) < (?, ?)", target.CreatedAt, target.ID).
				Limit(1).Find(&boundary)
			if found.Error != nil {
				return found.Error
			}
			if found.RowsAffected > 0 {
				boundaryAvailable = true
				summary = conv.Summary
				query = query.Where("(created_at, id) > (?, ?)", boundary.CreatedAt, boundary.ID)
			}
		}
		var first, recent []ChatMessage
		if err := query.Session(&gorm.Session{}).Order("created_at ASC, id ASC").Limit(2).Find(&first).Error; err != nil {
			return err
		}
		if err := query.Session(&gorm.Session{}).Order("created_at DESC, id DESC").Limit(maxMessages).Find(&recent).Error; err != nil {
			return err
		}
		slices.Reverse(recent)
		messages := make([]ChatMessage, 0, len(first)+len(recent))
		messages = append(messages, first...)
		for _, message := range recent {
			if (len(first) > 0 && message.ID == first[0].ID) || (len(first) > 1 && message.ID == first[1].ID) {
				continue
			}
			messages = append(messages, message)
		}
		result = &HistoryWindowResult{
			Messages: messages, Summary: summary,
			SummaryUpToMessageID:     conv.SummaryUpToMessageID,
			SummaryBoundaryAvailable: boundaryAvailable,
		}
		return nil
	})
	return result, err
}
