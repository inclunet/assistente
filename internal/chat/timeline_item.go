package chat

import (
	"fmt"

	"assistente/internal/core/ports"
	"assistente/internal/database"
)

// TimelinePosition conserva a chave SQLite sem codificar um cursor que o
// adaptador antigo não utiliza. A codificação opaca pertence à borda wire.
type TimelinePosition struct {
	AnchorCreatedAt string
	AnchorID        string
	Slot            int
	ItemID          string
}

func itemPosition(row database.MessageWindowItem) TimelinePosition {
	slot := 0
	if row.Kind == database.MessageWindowItemKindTurn {
		slot = 1
	}
	return TimelinePosition{row.AnchorCreatedAt, row.AnchorID, slot, timelineWindowItemKey(row)}
}

func projectWindowItems(conversationID string, parentID *string, rows []database.MessageWindowItem, messages []Message, responses map[string]*TimelineItem) ([]*TimelineItem, error) {
	byID := make(map[string]*Message, len(messages))
	for i := range messages {
		byID[messages[i].ID] = &messages[i]
	}
	items := make([]*TimelineItem, 0, len(rows))
	for _, row := range rows {
		var item *TimelineItem
		if row.Kind == database.MessageWindowItemKindTurn {
			item = responses[row.TurnID]
		} else if message := byID[row.MessageID]; message != nil {
			item = &TimelineItem{
				ItemID: ports.MessageItemID(message.ID), ConversationID: conversationID,
				ScopeParentID: parentID, Kind: "message", MessageIDs: []string{message.ID},
				Representative: message, Durability: "persisted", messageCount: 1,
			}
			if message.Reasoning != "" {
				item.Segments = append(item.Segments, responseTextSegment("reasoning", *message, 0, message.Reasoning))
			}
			if message.Content != "" {
				item.Segments = append(item.Segments, responseTextSegment("text", *message, 0, message.Content))
			}
		}
		if item == nil {
			return nil, fmt.Errorf("item da janela sem conteúdo projetado: %s", row.ID)
		}
		item.Position = itemPosition(row)
		items = append(items, item)
	}
	return items, nil
}

// Compatibilidade somente na borda. A ordem e identidade vêm dos itens; este
// adaptador não agrupa mensagens nem escolhe outro representante canônico.
func legacyItemNodes(items []*TimelineItem, rows []database.MessageWindowItem, childCounts map[string]int) []MessageNode {
	nodes := make([]MessageNode, 0, len(items))
	for i, item := range items {
		message := item.Representative
		if message == nil {
			message = item.legacyRepresentative
		}
		if message == nil {
			continue // O DTO antigo não representa resposta sem mensagem persistida.
		}
		node := MessageNode{ItemID: item.ItemID, Message: EnrichMessage(*message), ChildCount: childCounts[message.ID]}
		if item.ScopeParentID != nil {
			node.Level = 1
		}
		node.OriginalIndex = &rows[i].OriginalIndex
		if item.Kind == "response" {
			node.Message.TurnSegments = legacyResponseSegments(item.Segments, item.messageCount, item.callCount)
		}
		nodes = append(nodes, node)
	}
	return nodes
}
