package database

import (
	"context"
	"fmt"
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
		args := append(repo.timelineItemArgs(conversationID, parentID), ids[start:end])
		var rows []MessageWindowItem
		if err := repo.db.WithContext(ctx).Raw(repo.timelineItemCTE(parentID)+" SELECT "+timelineItemColumns+" FROM timeline_items WHERE item_key IN ? ORDER BY "+timelineOrderAscending, args...).Scan(&rows).Error; err != nil {
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
