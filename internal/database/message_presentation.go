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

// Reuses the same classification as the window, including admitted responses.
func countCanonicalChildren(ctx context.Context, tx *gorm.DB, conversationID string, ids []string) (map[string]int, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	repo := NewMessageRepository(tx).WithTimelineTurns()
	query := repo.timelineItemCTEForScope("m.parent_id IN ?", "q.parent_id IN ?", "chat_messages m", "") + `
 SELECT parent_id, COUNT(*) AS count FROM timeline_items WHERE parent_id IN (
 SELECT p.id FROM chat_messages p JOIN conversations c ON c.id=p.conversation_id WHERE c.user_id=? AND p.conversation_id=? AND p.id IN ?) GROUP BY parent_id`
	var rows []struct {
		ParentID string
		Count    int
	}
	if err := tx.WithContext(ctx).Raw(query, conversationID, ids, conversationID, ids, userID, conversationID, ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		counts[row.ParentID] = row.Count
	}
	return counts, nil
}
