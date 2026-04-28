package chat

import (
	"sort"

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

// EnrichMessage converte Message para o DTO do frontend EnrichedMessage
func EnrichMessage(msg Message) EnrichedMessage {
	return EnrichedMessage{
		ID:               msg.ID,
		ConversationID:   msg.ConversationID,
		ParentID:         msg.ParentID,
		TurnID:           msg.TurnID,
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
func BuildMessageTree(messages []Message) []MessageNode {

	childrenMap := make(map[string][]Message)
	var rootMessages []Message

	for _, msg := range messages {
		if msg.ParentID == nil {
			rootMessages = append(rootMessages, msg)
		} else {
			childrenMap[*msg.ParentID] = append(childrenMap[*msg.ParentID], msg)
		}
	}

	sort.Slice(rootMessages, func(i, j int) bool {
		return rootMessages[i].CreatedAt.Before(rootMessages[j].CreatedAt)
	})
	for parentID := range childrenMap {
		sort.Slice(childrenMap[parentID], func(i, j int) bool {
			return childrenMap[parentID][i].CreatedAt.Before(childrenMap[parentID][j].CreatedAt)
		})
	}

	var buildNode func(msg Message, level int) MessageNode
	buildNode = func(msg Message, level int) MessageNode {
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

	return result
}

// BuildMessageNodes constrói uma lista plana de MessageNode a partir de mensagens brutas,
// calculando childCount para lazy loading (sem carregá-los na memória).
// parentID nil = busca raízes (level 0); non-nil = filhos diretos (level 1).
func BuildMessageNodes(messages []Message, childCounts map[string]int, parentID *string) []MessageNode {
	level := 0
	if parentID != nil {
		level = 1
	}
	result := make([]MessageNode, 0, len(messages))
	for _, msg := range messages {
		result = append(result, MessageNode{
			Message:    EnrichMessage(msg),
			Level:      level,
			ChildCount: childCounts[msg.ID],
		})
	}
	return result
}
