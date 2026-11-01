package database

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// GetTimelineItemsByIDWithContext reconcilia somente identidades materializadas.
// A ausência é significativa neste conjunto explícito; não é paginação.
func (r *MessageRepository) GetTimelineItemsByIDWithContext(ctx context.Context, conversationID string, parentID *string, ids []string) (*MessageWindowResult, error) {
	if len(ids) > 16384 {
		return nil, fmt.Errorf("seleção de timeline excede o limite")
	}
	if _, err := NewConversationRepository(r.db).GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	result := &MessageWindowResult{}
	repo := r.WithTimelineTurns()
	for start := 0; start < len(ids); start += 400 {
		end := min(start+400, len(ids))
		sql, args, err := repo.timelineSelectionQuery(conversationID, parentID, ids[start:end])
		if err != nil {
			return nil, err
		}
		var rows []MessageWindowItem
		if err := repo.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
			return nil, err
		}
		messageRepo := NewMessageRepository(repo.db.Omit("media", "audio", "audio_mime_type").Session(&gorm.Session{}))
		messages, err := messageRepo.fetchMessagesForTimelineItems(conversationID, parentID, rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, rows...)
		result.Messages = append(result.Messages, messages...)
	}
	return result, nil
}

func (r *MessageRepository) timelineSelectionQuery(conversationID string, parentID *string, ids []string) (string, []interface{}, error) {
	var messages, turns []string
	for _, id := range ids {
		kind, value, ok := strings.Cut(id, ":")
		if !ok || value == "" {
			return "", nil, fmt.Errorf("identidade de timeline inválida")
		}
		switch kind {
		case "message":
			messages = append(messages, value)
		case "turn":
			turns = append(turns, value)
		default:
			return "", nil, fmt.Errorf("tipo de item de timeline inválido")
		}
	}
	scopeArgs := []interface{}{conversationID}
	if parentID != nil {
		scopeArgs = append(scopeArgs, *parentID)
	}
	parentPredicate := "parent_id IS NULL"
	if parentID != nil {
		parentPredicate = "parent_id = ?"
	}
	args := []interface{}{messages}
	args = append(args, scopeArgs...)
	args = append(args, turns)
	args = append(args, scopeArgs...)
	if r.includeTimelineTurns {
		args = append(args, scopeArgs...)
		args = append(args, turns)
	}
	args = append(args, ids)
	// CROSS JOIN mantém a seleção à esquerda: o SQLite busca cada mensagem pela
	// PK, sem preferir percorrer todo o índice da conversa antes da classificação.
	cte := r.timelineItemCTEWithPredicates(parentID,
		"(SELECT id FROM chat_messages WHERE id IN ? UNION SELECT id FROM chat_messages WHERE conversation_id=? AND "+parentPredicate+" AND turn_id IN ?) selected CROSS JOIN chat_messages m ON m.id=selected.id",
		" AND t.user_message_id IN ?")
	return cte + " SELECT " + timelineItemColumns + " FROM timeline_items WHERE item_key IN ? ORDER BY " + timelineOrderAscending, args, nil
}
