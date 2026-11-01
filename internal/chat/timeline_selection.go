package chat

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"assistente/internal/database"
	"gorm.io/gorm"
)

type TimelineSelectionSnapshot struct {
	Version database.ChatTimelineVersion
	Items   []*TimelineItem
	Turns   map[string]*database.ChatTurn
}

type TimelineSelectionReader interface {
	ReadSelection(context.Context, string, []string) (*TimelineSelectionSnapshot, error)
}

func (r *DBTimelineReader) ReadSelection(ctx context.Context, conversationID string, refs []string) (*TimelineSelectionSnapshot, error) {
	if len(refs) > 16384 {
		return nil, fmt.Errorf("seleção de timeline excede o limite")
	}
	refs = uniquePublicationIDs(refs)
	result := &TimelineSelectionSnapshot{Turns: make(map[string]*database.ChatTurn)}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		version, err := database.ReadChatTimelineVersion(ctx, tx, conversationID)
		if err != nil {
			return err
		}
		result.Version = version
		byAnchor := make(map[string][]string)
		anchors := make([]string, 0, len(refs))
		for _, ref := range refs {
			kind, id, ok := strings.Cut(ref, ":")
			if !ok || id == "" || (kind != "message" && kind != "turn") {
				return fmt.Errorf("identidade de item inválida")
			}
			if _, exists := byAnchor[id]; !exists {
				anchors = append(anchors, id)
			}
			byAnchor[id] = append(byAnchor[id], ref)
		}
		groups := make(map[string][]string)
		parents := make(map[string]*string)
		var sourceBytes int64
		for start := 0; start < len(anchors); start += 400 {
			batch := anchors[start:min(start+400, len(anchors))]
			size, err := publicationSelectionReadSize(tx, conversationID, batch)
			if err != nil {
				return err
			}
			sourceBytes += size
			if sourceBytes > canonicalSourceBytes {
				return ErrTimelineSourceLimit
			}
			var messages []database.ChatMessage
			if err := tx.Select("id", "parent_id").Where("conversation_id=? AND id IN ?", conversationID, batch).Find(&messages).Error; err != nil {
				return err
			}
			for _, message := range messages {
				key := "root"
				if message.ParentID != nil {
					key = "thread:" + *message.ParentID
				}
				parents[key] = message.ParentID
				groups[key] = append(groups[key], byAnchor[message.ID]...)
			}
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parent := parents[key]
			rows, err := database.NewMessageRepository(tx).GetTimelineItemsByIDWithContext(ctx, conversationID, parent, groups[key])
			if err != nil {
				return err
			}
			ids := make([]string, 0)
			for _, row := range rows.Items {
				if row.Kind == database.MessageWindowItemKindTurn {
					ids = append(ids, row.TurnID)
				}
			}
			facts, err := readTimelineFacts(ctx, tx, conversationID, ids)
			if err != nil {
				return err
			}
			responses, err := projectSnapshotResponses(conversationID, parent, rows.Messages, facts)
			if err != nil {
				return err
			}
			for _, row := range rows.Items {
				if row.Kind == database.MessageWindowItemKindTurn && responses[row.TurnID] == nil {
					responses[row.TurnID], err = ProjectPersistedResponse(conversationID, row.TurnID, parent, nil, facts.calls[row.TurnID], facts.turns[row.TurnID])
					if err != nil {
						return err
					}
				}
			}
			items, err := projectWindowItems(conversationID, parent, rows.Items, rows.Messages, responses)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, items...)
			for id, turn := range facts.turns {
				result.Turns[id] = turn
			}
		}
		return nil
	})
	return result, err
}
