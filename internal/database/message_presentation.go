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
	encodedIDs := EncodeTimelineIDSet(ids)
	repo := NewMessageRepository(tx)
	// CROSS JOIN fixes the loop order: select child questions by parent first,
	// then probe the turn primary key. No conversation-wide turn scan is needed,
	// even before SQLite has collected planner statistics.
	query := repo.timelineItemCTEWithSources("m.parent_id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))", "q.parent_id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))", "chat_messages m",
		"chat_messages q CROSS JOIN chat_turns t ON t.user_message_id=q.id", "") + `
 SELECT parent_id, COUNT(*) AS count FROM timeline_items WHERE parent_id IN (
 SELECT p.id FROM chat_messages p JOIN conversations c ON c.id=p.conversation_id WHERE c.user_id=? AND p.conversation_id=? AND p.id IN (SELECT CAST(unhex(value) AS TEXT) FROM json_each(?))) GROUP BY parent_id`
	rows, err := tx.WithContext(ctx).Raw(query, conversationID, encodedIDs, conversationID, encodedIDs, userID, conversationID, encodedIDs).Rows()
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
