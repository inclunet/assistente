package database

import "context"

// SelectHistoricalTimelineWithContext mantém os limites aceitos da janela.
// Não repete Anchor=end nem troca uma âncora removida por outra página.
func (r *MessageRepository) SelectHistoricalTimelineWithContext(ctx context.Context, query MessageWindowQuery, first, last string) (*MessageWindowResult, error) {
	repo := NewMessageRepository(r.db.WithContext(ctx)).WithTimelineTurns()
	result, err := repo.selectMessageWindow(ctx, MessageWindowQuery{ConversationID: query.ConversationID, ParentID: query.ParentID})
	if err != nil {
		return nil, err
	}
	if first == "" && last == "" {
		return result, nil
	}
	if first == "" || last == "" || normalizeMessageWindowLimit(query.Limit) == 0 {
		return nil, ErrTimelineCursorInvalid
	}
	bounds := make([]MessageWindowItem, 0, 2)
	for _, value := range []string{first, last} {
		cursor, err := decodeTimelineCursor(MessageWindowQuery{ConversationID: query.ConversationID, ParentID: query.ParentID, Cursor: value})
		if err != nil {
			return nil, err
		}
		item, err := repo.getAnchorTimelineItem(MessageWindowQuery{ConversationID: query.ConversationID, ParentID: query.ParentID, AnchorItemID: cursor.ItemID, Cursor: value})
		if err != nil {
			return nil, err
		}
		if !cursor.matches(*item) {
			return nil, ErrTimelineCursorStale
		}
		bounds = append(bounds, *item)
	}
	where := "(created_at, anchor_id COLLATE BINARY, slot, item_key COLLATE BINARY) >= (?, ?, ?, ?) AND (created_at, anchor_id COLLATE BINARY, slot, item_key COLLATE BINARY) <= (?, ?, ?, ?)"
	args := append(timelineKeyArgs(bounds[0]), timelineKeyArgs(bounds[1])...)
	limit := normalizeMessageWindowLimit(query.Limit)
	items, err := repo.queryTimelineItems(query.ConversationID, query.ParentID, where, timelineOrderAscending, limit+1, args...)
	if err != nil {
		return nil, err
	}
	if len(items) > limit {
		return nil, ErrTimelineSourceLimit
	}
	if len(items) == 0 {
		return nil, ErrTimelineCursorInvalid
	}
	start, err := repo.countTimelineItemsBefore(query.ConversationID, query.ParentID, bounds[0])
	if err != nil {
		return nil, err
	}
	result.Items = items
	result.StartIndex = start
	result.EndIndex = start + len(items) - 1
	result.HasBefore = start > 0
	result.HasAfter = result.EndIndex < result.TotalCount-1
	for i := range result.Items {
		result.Items[i].OriginalIndex = start + i
		if result.Items[i].InvalidTurnReference {
			result.InvalidTurnReferences++
		}
	}
	return result, nil
}
