package chat

import (
	"fmt"
	"sort"
	"strconv"

	"assistente/internal/database"
	"assistente/internal/events"
)

// ConversationService orquestra consultas e modificações em conversas e mensagens.
// Ele delega a persistência para repositórios e emite eventos de domínio para o frontend.
type ConversationService struct {
	cr      ConversationRepository
	mr      MessageRepository
	emitter events.Emitter
}

func NewConversationService(cr ConversationRepository, mr MessageRepository, em events.Emitter) *ConversationService {
	return &ConversationService{cr: cr, mr: mr, emitter: em}
}

// EnrichMessage converte database.ChatMessage para o DTO do frontend EnrichedMessage
func EnrichMessage(msg database.ChatMessage) EnrichedMessage {
	var parentIDStr *string
	if msg.ParentID != nil {
		s := strconv.FormatUint(uint64(*msg.ParentID), 10)
		parentIDStr = &s
	}

	var turnID *uint
	if msg.TurnID != nil {
		v := uint(*msg.TurnID) // assumindo que database.ChatMessage.TurnID é inteiro
		turnID = &v
	}

	return EnrichedMessage{
		ID:               strconv.FormatUint(uint64(msg.ID), 10),
		ConversationID:   msg.ConversationID,
		ParentID:         parentIDStr,
		TurnID:           turnID,
		Role:             msg.Role,
		Content:          msg.Content,
		Reasoning:        msg.Reasoning,
		Media:            msg.Media,
		ToolCalls:        msg.ToolCalls,
		ToolCallID:       msg.ToolCallID,
		PromptTokens:     msg.PromptTokens,
		CompletionTokens: msg.CompletionTokens,
		TotalTokens:      msg.TotalTokens,
		Model:            msg.Model,
		Source:           msg.Source,
		CreatedAt:        msg.CreatedAt,
		Timestamp:        msg.CreatedAt.UnixMilli(),
		IsStreaming:      false,
		Internal:         msg.ParentID != nil || msg.Role == "tool",
	}
}

// BuildMessageTree organiza mensagens planas em uma árvore hierárquica
func BuildMessageTree(messages []database.ChatMessage) []MessageNode {
	fmt.Printf("[TREE] Construindo árvore com %d mensagens\n", len(messages))

	childrenMap := make(map[uint][]database.ChatMessage)
	var rootMessages []database.ChatMessage

	for _, msg := range messages {
		if msg.ParentID == nil {
			rootMessages = append(rootMessages, msg)
		} else {
			childrenMap[*msg.ParentID] = append(childrenMap[*msg.ParentID], msg)
		}
	}

	sort.Slice(rootMessages, func(i, j int) bool {
		return rootMessages[i].ID < rootMessages[j].ID
	})
	for parentID := range childrenMap {
		sort.Slice(childrenMap[parentID], func(i, j int) bool {
			return childrenMap[parentID][i].ID < childrenMap[parentID][j].ID
		})
	}

	var buildNode func(msg database.ChatMessage, level int) MessageNode
	buildNode = func(msg database.ChatMessage, level int) MessageNode {
		node := MessageNode{
			Message:  EnrichMessage(msg),
			Children: []MessageNode{},
			Level:    level,
		}

		children := childrenMap[msg.ID]
		node.ChildCount = len(children)

		for _, child := range children {
			childNode := buildNode(child, level+1)
			node.Children = append(node.Children, childNode)
		}

		return node
	}

	result := make([]MessageNode, 0, len(rootMessages))
	for _, rootMsg := range rootMessages {
		node := buildNode(rootMsg, 0)
		result = append(result, node)
	}

	fmt.Printf("[TREE] Resultado: %d raízes\n", len(result))
	return result
}
