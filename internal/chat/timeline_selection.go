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
	TurnIDs []string
	Version database.ChatTimelineVersion
	Items   []*TimelineItem
	Turns   map[string]*database.ChatTurn
}

type TimelineSelectionReader interface {
	ReadSelection(context.Context, string, []string) (*TimelineSelectionSnapshot, error)
}

type TimelineLifecycleReader interface {
	ReadTurnLifecycles(context.Context, string, []string) (*TimelineSelectionSnapshot, error)
}

type timelineSelectionScope struct {
	parentID *string
	refs     []string
	sources  []*database.ChatMessage
}

func timelineSelectionScopeKey(parentID *string) string {
	if parentID == nil {
		return "root"
	}
	return "thread:" + *parentID
}

// ReadTurnLifecycles prova execução/ausência sem materializar mensagens ou ledger.
// Turns contém somente UserMessageID, ActiveExecutionID e CurrentExecutionID;
// não substitui registros completos da projeção. A versão acompanha a mesma
// transação autorizada e a cota mede apenas esses campos necessários.
func (r *DBTimelineReader) ReadTurnLifecycles(ctx context.Context, conversationID string, ids []string) (*TimelineSelectionSnapshot, error) {
	if len(ids) > database.MaxMessageWindowRows {
		return nil, ErrTimelineSourceLimit
	}
	if err := database.CheckTimelineIdentityBudget(ids); err != nil {
		return nil, err
	}
	result := &TimelineSelectionSnapshot{TurnIDs: uniquePublicationIDs(ids)}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		version, err := database.ReadChatTimelineVersion(ctx, tx, conversationID)
		if err != nil {
			return err
		}
		result.Version = version
		var size int64
		if err := tx.Model(&database.ChatTurn{}).
			Select(`COALESCE(SUM(1024 + COALESCE(LENGTH(CAST(user_message_id AS BLOB)),0) + COALESCE(LENGTH(CAST(active_execution_id AS BLOB)),0) + COALESCE(LENGTH(CAST(current_execution_id AS BLOB)),0)), 0)`).
			Where("conversation_id=? AND user_message_id IN ?", conversationID, result.TurnIDs).
			Scan(&size).Error; err != nil {
			return err
		}
		if size > canonicalSourceBytes {
			return ErrTimelineSourceLimit
		}
		var turns []database.ChatTurn
		if err := tx.Select("user_message_id", "active_execution_id", "current_execution_id").
			Where("conversation_id=? AND user_message_id IN ?", conversationID, result.TurnIDs).Find(&turns).Error; err != nil {
			return err
		}
		result.Turns = make(map[string]*database.ChatTurn, len(turns))
		for i := range turns {
			result.Turns[turns[i].UserMessageID] = &turns[i]
		}
		return nil
	})
	return result, err
}

func (r *DBTimelineReader) ReadSelection(ctx context.Context, conversationID string, refs []string) (*TimelineSelectionSnapshot, error) {
	if len(refs) > 16384 {
		return nil, fmt.Errorf("seleção de timeline excede o limite")
	}
	if err := database.CheckTimelineIdentityBudget(refs); err != nil {
		return nil, err
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
		var messageIDs, turnIDs []string
		for _, ref := range refs {
			kind, id, ok := strings.Cut(ref, ":")
			if !ok || id == "" || (kind != "message" && kind != "turn") {
				return fmt.Errorf("identidade de item inválida")
			}
			if _, exists := byAnchor[id]; !exists {
				anchors = append(anchors, id)
			}
			byAnchor[id] = append(byAnchor[id], ref)
			if kind == "turn" {
				turnIDs = append(turnIDs, id)
			} else {
				messageIDs = append(messageIDs, id)
			}
		}
		sources, err := database.ReadBoundedTimelineSources(tx, conversationID, messageIDs, turnIDs)
		if err != nil {
			return err
		}

		groups := make(map[string]*timelineSelectionScope)
		for start := 0; start < len(anchors); start += 400 {
			batch := anchors[start:min(start+400, len(anchors))]
			var messages []database.ChatMessage
			if err := tx.Select("id", "parent_id").Where("conversation_id=? AND id IN ?", conversationID, batch).Where(database.CurrentChatMessageSQL("chat_messages")).Find(&messages).Error; err != nil {
				return err
			}
			for _, message := range messages {
				key := timelineSelectionScopeKey(message.ParentID)
				group := groups[key]
				if group == nil {
					group = &timelineSelectionScope{parentID: message.ParentID}
					groups[key] = group
				}
				group.refs = append(group.refs, byAnchor[message.ID]...)
			}
		}
		// Particiona uma vez, sem copiar os corpos das mensagens ou perder a ordem.
		for i := range sources {
			if group := groups[timelineSelectionScopeKey(sources[i].ParentID)]; group != nil {
				group.sources = append(group.sources, &sources[i])
			}
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			group := groups[key]
			parent := group.parentID
			rows, err := database.NewMessageRepository(tx).SelectTimelineItemsByIDWithContext(ctx, conversationID, parent, group.refs)
			if err != nil {
				return err
			}
			rows.ApplyTimelineSources(conversationID, parent, group.sources)
			ids := make([]string, 0)
			for _, row := range rows.Items {
				if row.Kind == database.MessageWindowItemKindTurn {
					ids = append(ids, row.TurnID)
				}
			}
			lifecycleIDs := append([]string(nil), ids...)
			if err := database.ReadMessagePresentation(ctx, tx, rows.Messages); err != nil {
				return err
			}
			for _, message := range rows.Messages {
				if message.Role == "user" {
					lifecycleIDs = append(lifecycleIDs, message.ID)
				}
			}
			lifecycleIDs = uniquePublicationIDs(lifecycleIDs)
			result.TurnIDs = append(result.TurnIDs, lifecycleIDs...)
			facts, err := readTimelineFactsForSelection(ctx, tx, conversationID, lifecycleIDs, ids)
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
