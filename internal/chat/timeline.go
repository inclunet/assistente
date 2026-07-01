package chat

import (
	"assistente/internal/logging"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"assistente/internal/database"
)

// ToolOnlyTurnPlaceholderSource marks a synthetic assistant placeholder for
// turns that only persisted tool result messages and have no assistant response.
const ToolOnlyTurnPlaceholderSource = "tool_only_turn_placeholder"

const invalidToolCallsLogLimit = 512

var invalidToolCallsLogState = struct {
	sync.Mutex
	seen  map[string]struct{}
	order []string
}{
	seen: make(map[string]struct{}),
}

// MessageHasToolCalls indica se uma mensagem assistant carrega tool calls.
// A detecção é puramente pela string persistida para evitar parse redundante
// durante a montagem dos segmentos de timeline.
func MessageHasToolCalls(m Message) bool {
	switch strings.TrimSpace(m.ToolCalls) {
	case "", "[]", "null":
		return false
	default:
		return true
	}
}

func shouldLogInvalidToolCalls(messageID string) bool {
	invalidToolCallsLogState.Lock()
	defer invalidToolCallsLogState.Unlock()
	if _, ok := invalidToolCallsLogState.seen[messageID]; ok {
		return false
	}
	if len(invalidToolCallsLogState.order) >= invalidToolCallsLogLimit {
		oldest := invalidToolCallsLogState.order[0]
		invalidToolCallsLogState.order[0] = ""
		invalidToolCallsLogState.order = invalidToolCallsLogState.order[1:]
		delete(invalidToolCallsLogState.seen, oldest)
	}
	invalidToolCallsLogState.seen[messageID] = struct{}{}
	invalidToolCallsLogState.order = append(invalidToolCallsLogState.order, messageID)
	return true
}

func ParseToolCalls(messageID string, raw string) []map[string]interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var calls []map[string]interface{}
	arrayErr := json.Unmarshal([]byte(raw), &calls)
	if arrayErr == nil {
		return calls
	}
	var call map[string]interface{}
	singleErr := json.Unmarshal([]byte(raw), &call)
	if singleErr == nil {
		return []map[string]interface{}{call}
	}
	if shouldLogInvalidToolCalls(messageID) {
		logging.Infof(context.Background(), "chat.timeline", "[Chat] tool_calls JSON inválido descartado message_id=%s: array=%v object=%v", messageID, arrayErr, singleErr)
	}
	return nil
}

// ConsolidatedTurnResult agrega o representante do turno (uma única ChatMessage
// canônica para a timeline) e os segmentos cronológicos do turno (Issue #150).
type ConsolidatedTurnResult struct {
	Message  Message
	Segments []TurnSegment
}

func toolCallToTurnSegmentToolCall(call map[string]interface{}) TurnSegmentToolCall {
	id, _ := call["id"].(string)
	tipo, _ := call["type"].(string)
	if tipo == "" {
		tipo = "function"
	}
	name := ""
	args := ""
	if fn, ok := call["function"].(map[string]interface{}); ok {
		if v, ok := fn["name"].(string); ok {
			name = v
		}
		if v, ok := fn["arguments"].(string); ok {
			args = v
		}
	}
	if name == "" {
		if v, ok := call["name"].(string); ok {
			name = v
		}
	}
	if args == "" {
		if v, ok := call["arguments"].(string); ok {
			args = v
		}
	}
	result := ""
	if v, ok := call["result"].(string); ok {
		result = v
	}
	return TurnSegmentToolCall{
		ID:       id,
		Type:     tipo,
		Function: TurnSegmentToolFunction{Name: name, Arguments: args},
		Result:   result,
	}
}

func ConsolidateTimelineTurnMessages(messages []Message, invocationToolResults map[string]string) Message {
	return ConsolidateTimelineTurn(messages, invocationToolResults).Message
}

// ConsolidateTimelineTurn produz a representação canônica de um turno do
// assistente para o timeline do chat: uma Message representativa e a lista
// cronológica de segmentos usada pelo frontend acessível.
func ConsolidateTimelineTurn(messages []Message, invocationToolResults map[string]string) ConsolidatedTurnResult {
	if len(messages) == 0 {
		return ConsolidatedTurnResult{}
	}
	messages = append([]Message(nil), messages...)
	sort.SliceStable(messages, func(i, j int) bool {
		if !messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].CreatedAt.Before(messages[j].CreatedAt)
		}
		return messages[i].ID < messages[j].ID
	})
	toolResults := make(map[string]string)
	for _, message := range messages {
		if message.Role == "tool" && message.ToolCallID != "" {
			toolResults[message.ToolCallID] = message.Content
		}
	}
	// tool_invocations é o caminho canônico novo; usa como fallback quando
	// não existe mais mensagem role=tool persistida.
	for callID, result := range invocationToolResults {
		if callID == "" {
			continue
		}
		if existing, ok := toolResults[callID]; ok && existing != "" {
			continue
		}
		toolResults[callID] = result
	}

	consolidated := messages[0]
	hasAssistant := false
	finalContent := ""
	finalReasoning := ""
	allToolCalls := make([]map[string]interface{}, 0)
	seenToolCallIDs := make(map[string]struct{})
	segments := make([]TurnSegment, 0)
	assistantCount := 0

	hasToolBearingAssistant := false
	for _, message := range messages {
		if message.Role == "assistant" && MessageHasToolCalls(message) {
			hasToolBearingAssistant = true
			break
		}
	}
	finalMsgIdx := -1
	if hasToolBearingAssistant {
		for i, message := range messages {
			if message.Role == "assistant" && strings.TrimSpace(message.Content) != "" && !MessageHasToolCalls(message) {
				finalMsgIdx = i
				break
			}
		}
	}
	var finalTextSegment *TurnSegment
	for i, message := range messages {
		if message.Role != "assistant" {
			continue
		}
		hasAssistant = true
		assistantCount++
		consolidated = message
		if message.Content != "" {
			finalContent = message.Content
		}
		if message.Reasoning != "" {
			finalReasoning = message.Reasoning
		}
		if strings.TrimSpace(message.Content) != "" {
			if i == finalMsgIdx {
				finalTextSegment = &TurnSegment{Type: "text", Content: message.Content}
			} else {
				segments = append(segments, TurnSegment{
					Type:    "text",
					Content: message.Content,
				})
			}
		}
		iterationCalls := make([]TurnSegmentToolCall, 0)
		for _, call := range ParseToolCalls(message.ID, message.ToolCalls) {
			callID, _ := call["id"].(string)
			if callID != "" {
				if _, seen := seenToolCallIDs[callID]; seen {
					continue
				}
				seenToolCallIDs[callID] = struct{}{}
				if existing, ok := call["result"].(string); ok {
					if strings.TrimSpace(existing) != "" {
						allToolCalls = append(allToolCalls, call)
						iterationCalls = append(iterationCalls, toolCallToTurnSegmentToolCall(call))
						continue
					}
				}
				if result, ok := toolResults[callID]; ok {
					call["result"] = result
				}
			}
			allToolCalls = append(allToolCalls, call)
			iterationCalls = append(iterationCalls, toolCallToTurnSegmentToolCall(call))
		}
		if len(iterationCalls) > 0 {
			segments = append(segments, TurnSegment{
				Type:      "tool_calls",
				ToolCalls: iterationCalls,
			})
		}
	}
	if finalMsgIdx >= 0 {
		finalContent = messages[finalMsgIdx].Content
		finalReasoning = messages[finalMsgIdx].Reasoning
		consolidated.PromptTokens = messages[finalMsgIdx].PromptTokens
		consolidated.CompletionTokens = messages[finalMsgIdx].CompletionTokens
		consolidated.TotalTokens = messages[finalMsgIdx].TotalTokens
		consolidated.Model = messages[finalMsgIdx].Model
	}
	if finalTextSegment != nil {
		segments = append(segments, *finalTextSegment)
	}
	if !hasAssistant {
		// Keep the persisted tool message ID as representative so backend pagination anchors
		// still resolve to a real row.
		consolidated.Role = "assistant"
		consolidated.Content = ""
		consolidated.Reasoning = ""
		consolidated.ToolCallID = ""
		consolidated.Source = ToolOnlyTurnPlaceholderSource
		placeholderCalls := make([]map[string]interface{}, 0, len(toolResults))
		segmentToolCalls := make([]TurnSegmentToolCall, 0, len(toolResults))
		for callID, result := range toolResults {
			placeholderCalls = append(placeholderCalls, map[string]interface{}{
				"id":       callID,
				"type":     "function",
				"function": map[string]interface{}{"name": "tool_result", "arguments": ""},
				"result":   result,
			})
		}
		sort.Slice(placeholderCalls, func(i, j int) bool {
			left, _ := placeholderCalls[i]["id"].(string)
			right, _ := placeholderCalls[j]["id"].(string)
			return left < right
		})
		for _, call := range placeholderCalls {
			segmentToolCalls = append(segmentToolCalls, toolCallToTurnSegmentToolCall(call))
		}
		if len(placeholderCalls) > 0 {
			if encoded, err := json.Marshal(placeholderCalls); err == nil {
				consolidated.ToolCalls = string(encoded)
			}
		} else {
			consolidated.ToolCalls = ""
		}
		var placeholderSegments []TurnSegment
		if len(segmentToolCalls) > 0 {
			placeholderSegments = []TurnSegment{{
				Type:      "tool_calls",
				ToolCalls: segmentToolCalls,
			}}
		}
		return ConsolidatedTurnResult{Message: consolidated, Segments: placeholderSegments}
	}
	consolidated.Content = finalContent
	consolidated.Reasoning = finalReasoning
	if len(allToolCalls) > 0 {
		if encoded, err := json.Marshal(allToolCalls); err == nil {
			consolidated.ToolCalls = string(encoded)
		}
	} else {
		consolidated.ToolCalls = ""
	}
	if assistantCount <= 1 && len(allToolCalls) == 0 {
		return ConsolidatedTurnResult{Message: consolidated, Segments: nil}
	}
	return ConsolidatedTurnResult{Message: consolidated, Segments: segments}
}

func MessageTimelineItemKey(message Message) string {
	// Produces the canonical key shared with frontend getTimelineNodeKey.
	// User messages stay standalone even if bad data carries TurnID.
	if message.Role != "user" && message.TurnID != nil && *message.TurnID != "" {
		return "turn:" + *message.TurnID
	}
	return "message:" + message.ID
}

func timelineWindowItemKey(item database.MessageWindowItem) string {
	if item.Kind == database.MessageWindowItemKindTurn {
		return "turn:" + item.TurnID
	}
	return "message:" + item.MessageID
}

func assignMessageNodeTurnSegments(nodes []MessageNode, segmentsByID map[string][]TurnSegment) []MessageNode {
	if len(segmentsByID) == 0 {
		return nodes
	}
	for i := range nodes {
		if segments, ok := segmentsByID[nodes[i].Message.ID]; ok && len(segments) > 0 {
			nodes[i].Message.TurnSegments = segments
		}
		if len(nodes[i].Children) > 0 {
			nodes[i].Children = assignMessageNodeTurnSegments(nodes[i].Children, segmentsByID)
		}
	}
	return nodes
}

func assignMessageNodeOriginalIndexes(nodes []MessageNode, indexesByID map[string]int) []MessageNode {
	if len(indexesByID) == 0 {
		return nodes
	}
	for i := range nodes {
		if index, ok := indexesByID[nodes[i].Message.ID]; ok {
			value := index
			nodes[i].OriginalIndex = &value
		}
		if len(nodes[i].Children) > 0 {
			nodes[i].Children = assignMessageNodeOriginalIndexes(nodes[i].Children, indexesByID)
		}
	}
	return nodes
}

// BuildNodesWithTimelineConsolidation groups persisted messages into canonical
// timeline items and returns nodes ready for rendering.
func BuildNodesWithTimelineConsolidation(messages []Message, parentID *string, childCounts map[string]int, invocationToolResults map[string]map[string]string) []MessageNode {
	if len(messages) == 0 {
		return []MessageNode{}
	}

	grouped := make(map[string][]Message)
	order := make([]string, 0, len(messages))
	seenKey := map[string]struct{}{}
	for _, msg := range messages {
		key := MessageTimelineItemKey(msg)
		grouped[key] = append(grouped[key], msg)
		if _, ok := seenKey[key]; !ok {
			seenKey[key] = struct{}{}
			order = append(order, key)
		}
	}

	representatives := make([]Message, 0, len(order))
	segmentsByMessageID := make(map[string][]TurnSegment)
	for _, key := range order {
		itemMessages := grouped[key]
		if len(itemMessages) == 0 {
			continue
		}
		representative := itemMessages[0]
		if strings.HasPrefix(key, "turn:") && representative.TurnID != nil {
			turnID := strings.TrimSpace(*representative.TurnID)
			result := ConsolidateTimelineTurn(itemMessages, invocationToolResults[turnID])
			representative = result.Message
			if len(result.Segments) > 0 {
				segmentsByMessageID[representative.ID] = result.Segments
			}
		}
		representatives = append(representatives, representative)
	}
	return assignMessageNodeTurnSegments(BuildMessageNodes(representatives, childCounts, parentID), segmentsByMessageID)
}

// BuildTimelineMessageNodes materializes repository timeline items into message
// nodes, preserving original item indexes returned by the canonical window query.
func BuildTimelineMessageNodes(items []database.MessageWindowItem, messages []Message, parentID *string, childCounts map[string]int, invocationToolResults map[string]map[string]string) []MessageNode {
	messagesByItemKey := make(map[string][]Message)
	for _, message := range messages {
		key := MessageTimelineItemKey(message)
		messagesByItemKey[key] = append(messagesByItemKey[key], message)
	}
	representatives := make([]Message, 0, len(items))
	originalIndexesByMessageID := make(map[string]int, len(items))
	segmentsByMessageID := make(map[string][]TurnSegment)
	for _, item := range items {
		itemMessages := messagesByItemKey[timelineWindowItemKey(item)]
		if len(itemMessages) == 0 {
			continue
		}
		representative := itemMessages[0]
		if item.Kind == database.MessageWindowItemKindTurn {
			turnID := strings.TrimSpace(item.TurnID)
			result := ConsolidateTimelineTurn(itemMessages, invocationToolResults[turnID])
			representative = result.Message
			if len(result.Segments) > 0 {
				segmentsByMessageID[representative.ID] = result.Segments
			}
		}
		representatives = append(representatives, representative)
		originalIndexesByMessageID[representative.ID] = item.OriginalIndex
	}
	nodes := assignMessageNodeOriginalIndexes(BuildMessageNodes(representatives, childCounts, parentID), originalIndexesByMessageID)
	return assignMessageNodeTurnSegments(nodes, segmentsByMessageID)
}

func CollectTurnIDsWithToolCalls(messages []Message) []string {
	turnIDs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, msg := range messages {
		if msg.TurnID == nil {
			continue
		}
		shouldHydrate := false
		if msg.Role == "assistant" && MessageHasToolCalls(msg) {
			shouldHydrate = true
		}
		if msg.Role == "tool" && strings.TrimSpace(msg.ToolCallID) != "" {
			shouldHydrate = true
		}
		if !shouldHydrate {
			continue
		}
		turnID := strings.TrimSpace(*msg.TurnID)
		if turnID == "" {
			continue
		}
		if _, ok := seen[turnID]; ok {
			continue
		}
		seen[turnID] = struct{}{}
		turnIDs = append(turnIDs, turnID)
	}
	return turnIDs
}
