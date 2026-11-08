package database

import (
	"context"
	"gorm.io/gorm"
)

// MessagePresentation holds only transient visual metadata.
type MessagePresentation struct{ ChildCount int }

func ReadMessagePresentation(ctx context.Context, tx *gorm.DB, messages []ChatMessage) error {
	for start := 0; start < len(messages); start += 400 {
		end := min(start+400, len(messages))
		ids := make([]string, 0, end-start)
		for _, message := range messages[start:end] {
			ids = append(ids, message.ID)
		}
		counts, err := countCanonicalChildren(ctx, tx, messages[start].ConversationID, ids)
		if err != nil {
			return err
		}
		for i := start; i < end; i++ {
			messages[i].Presentation = &MessagePresentation{ChildCount: counts[messages[i].ID]}
		}
	}
	return nil
}

// A consulta de contagem projeta apenas identidade/escopo. UNION deduplica os
// mesmos itens da janela; a pergunta/execução usa os predicados compartilhados.
var canonicalChildCountSQL = `WITH authorized_parents AS MATERIALIZED (
 SELECT p.id FROM (SELECT CAST(unhex(value) AS TEXT) AS id FROM json_each(?)) selected_parent
 CROSS JOIN chat_messages p ON p.id=selected_parent.id
 JOIN conversations c ON c.id=p.conversation_id WHERE c.user_id=? AND p.conversation_id=?
), child_items AS (
 SELECT ` + timelineItemKindSQL + ` AS kind, ` + timelineItemIDSQL + ` AS id, m.parent_id
 FROM chat_messages m ` + timelineQuestionJoinSQL + `
 WHERE m.conversation_id=? AND m.parent_id IN (SELECT id FROM authorized_parents) AND ` + CurrentChatMessageSQL("m") + `
 UNION
 SELECT 'turn',t.user_message_id,q.parent_id FROM chat_messages q
 CROSS JOIN chat_turns t ON t.user_message_id=q.id
 WHERE t.conversation_id=? AND q.conversation_id=t.conversation_id AND q.role='user'
 AND q.parent_id IN (SELECT id FROM authorized_parents)
) SELECT parent_id, COUNT(*) AS count FROM child_items GROUP BY parent_id`

// Reuses the same classification as the window, including admitted responses.
func countCanonicalChildren(ctx context.Context, tx *gorm.DB, conversationID string, ids []string) (map[string]int, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	encodedIDs := EncodeTimelineIDSet(ids)
	rows, err := tx.WithContext(ctx).Raw(canonicalChildCountSQL, encodedIDs, userID, conversationID, conversationID, conversationID).Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	counts := make(map[string]int)
	for rows.Next() {
		var parentID string
		var count int
		if err := rows.Scan(&parentID, &count); err != nil {
			return nil, err
		}
		counts[parentID] = count
	}
	return counts, rows.Err()
}
