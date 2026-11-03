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
			Select("COALESCE(SUM(1024 + "+publicationLifecycleSizeSQL+"), 0)").
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
			var messageIDs, turnIDs []string
			for _, id := range batch {
				for _, ref := range byAnchor[id] {
					if strings.HasPrefix(ref, "turn:") {
						turnIDs = append(turnIDs, id)
					} else {
						messageIDs = append(messageIDs, id)
					}
				}
			}
			size, err := publicationSelectionReadSize(tx, conversationID, messageIDs, turnIDs)
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
			lifecycleIDs := append([]string(nil), ids...)
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
