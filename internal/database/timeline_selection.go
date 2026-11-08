package database

import (
	"context"
	"fmt"
	"strings"
)

// SelectTimelineItemsByIDWithContext reconcilia somente identidades materializadas.
// A ausência é significativa neste conjunto explícito; não é paginação.
func (r *MessageRepository) SelectTimelineItemsByIDWithContext(ctx context.Context, conversationID string, parentID *string, ids []string) (*MessageWindowResult, error) {
	if len(ids) > 16384 {
		return nil, fmt.Errorf("seleção de timeline excede o limite")
	}
	if _, err := NewConversationRepository(r.db.Select("id")).GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	if err := CheckTimelineIdentityBudget(ids); err != nil {
		return nil, err
	}
	result := &MessageWindowResult{}
	for start := 0; start < len(ids); start += 400 {
		end := min(start+400, len(ids))
		sql, args, err := r.timelineSelectionQuery(conversationID, parentID, ids[start:end])
		if err != nil {
			return nil, err
		}
		rows, err := NewMessageRepository(r.db.WithContext(ctx)).scanTimelineItems(sql, args, timelineOrderAscending)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, rows...)
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
	args = append(args, scopeArgs...)
	args = append(args, turns)
	args = append(args, ids)
	// CROSS JOIN mantém a seleção à esquerda: o SQLite busca cada mensagem pela
	// PK, sem preferir percorrer todo o índice da conversa antes da classificação.
	cte := r.timelineItemCTEWithPredicates(parentID,
		"(SELECT id FROM chat_messages WHERE id IN ? UNION SELECT id FROM chat_messages WHERE conversation_id=? AND "+parentPredicate+" AND turn_id IN ?) selected CROSS JOIN chat_messages m ON m.id=selected.id",
		" AND t.user_message_id IN ?")
	return cte + " SELECT " + timelineItemColumns + " FROM timeline_items WHERE item_key IN ? ORDER BY " + timelineOrderAscending, args, nil
}
