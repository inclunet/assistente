package database

import (
	"database/sql"
	"gorm.io/gorm"
)

// A leitura da timeline não usa hooks nem associações. Escaneia diretamente
// a projeção escalar, sem construir uma segunda ChatMessage por reflexão.
// COALESCE conserva a semântica de zero do GORM para colunas legadas nulas.
func scanTimelineMessages(query *gorm.DB, capacity int) ([]ChatMessage, error) {
	rows, err := query.Select(`chat_messages.id, chat_messages.created_at, chat_messages.updated_at,
        COALESCE(chat_messages.conversation_id,''), chat_messages.parent_id, chat_messages.turn_id,
        chat_messages.chat_execution_id, COALESCE(chat_messages.role,''), COALESCE(chat_messages.content,''),
        COALESCE(chat_messages.reasoning,''), COALESCE(chat_messages.prompt_tokens,0),
        COALESCE(chat_messages.completion_tokens,0), COALESCE(chat_messages.total_tokens,0),
        COALESCE(chat_messages.cache_read_tokens,0), COALESCE(chat_messages.cache_write_tokens,0),
        COALESCE(chat_messages.cache_miss_tokens,0), COALESCE(chat_messages.model,''),
        COALESCE(chat_messages.source,''), COALESCE(chat_messages.pinned,0)`).Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	messages := make([]ChatMessage, 0, capacity)
	for rows.Next() {
		messages = append(messages, ChatMessage{})
		message := &messages[len(messages)-1]
		var created, updated sql.NullTime
		if err := rows.Scan(&message.ID, &created, &updated, &message.ConversationID,
			&message.ParentID, &message.TurnID, &message.ChatExecutionID, &message.Role,
			&message.Content, &message.Reasoning, &message.PromptTokens, &message.CompletionTokens,
			&message.TotalTokens, &message.CacheReadTokens, &message.CacheWriteTokens,
			&message.CacheMissTokens, &message.Model, &message.Source, &message.Pinned); err != nil {
			return nil, err
		}
		message.CreatedAt, message.UpdatedAt = created.Time, updated.Time
	}
	return messages, rows.Err()
}
