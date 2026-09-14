package chat

import (
	"sort"
	"strings"

	"assistente/internal/database"
)

// ConsolidatedTurnResult agrega a mensagem representante e seus segmentos
// canônicos. Invocações vêm exclusivamente do ledger.
type ConsolidatedTurnResult struct {
	Message  Message
	Segments []TurnSegment
}

func normalizeInvocationSummary(call TurnSegmentToolCall) TurnSegmentToolCall {
	call.InvocationID = strings.TrimSpace(call.InvocationID)
	call.ID = strings.TrimSpace(call.ID)
	call.Name = strings.TrimSpace(call.Name)
	call.Status = strings.TrimSpace(call.Status)
	if call.Name == "" {
		call.Name = "tool"
	}
	if call.Status == "" {
		call.Status = "succeeded"
	}
	if call.ResultAvailability == "" {
		call.ResultAvailability = "missing"
	}
	return call
}

func groupCanonicalInvocations(calls []TurnSegmentToolCall) map[string][]TurnSegmentToolCall {
	groups := make(map[string][]TurnSegmentToolCall)
	for _, raw := range calls {
		call := normalizeInvocationSummary(raw)
		key := strings.TrimSpace(call.AssistantMessageID)
		if key == "" {
			key = "iteration:" + itoa(max(call.Iteration, 1))
		}
		groups[key] = append(groups[key], call)
	}
	for key := range groups {
		sort.SliceStable(groups[key], func(i, j int) bool {
			if groups[key][i].Iteration != groups[key][j].Iteration {
				return groups[key][i].Iteration < groups[key][j].Iteration
			}
			return groups[key][i].ID < groups[key][j].ID
		})
	}
	return groups
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}

// ConsolidateTimelineTurn constrói a timeline apenas com mensagens
// conversacionais e resumos leves do ledger.
func ConsolidateTimelineTurn(messages []Message, invocationToolCalls []TurnSegmentToolCall) ConsolidatedTurnResult {
	if len(messages) == 0 {
		return ConsolidatedTurnResult{}
	}
	ordered := append([]Message(nil), messages...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})

	representative := ordered[0]
	hasFinalUsage := false
	for _, message := range ordered {
		if message.Role == "assistant" {
			if message.TotalTokens > 0 {
				representative = message
				hasFinalUsage = true
			} else if !hasFinalUsage {
				representative = message
			}
		}
	}

	groups := groupCanonicalInvocations(invocationToolCalls)
	segments := make([]TurnSegment, 0, len(ordered)+len(groups))
	consumed := make(map[string]struct{})
	for _, message := range ordered {
		if message.Role != "assistant" {
			continue
		}
		if strings.TrimSpace(message.Content) != "" {
			segments = append(segments, TurnSegment{Type: "text", Content: message.Content})
		}
		if calls := groups[message.ID]; len(calls) > 0 {
			segments = append(segments, TurnSegment{Type: "tool_calls", ToolCalls: calls})
			consumed[message.ID] = struct{}{}
		}
	}

	keys := make([]string, 0, len(groups))
	for key := range groups {
		if _, ok := consumed[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		segments = append(segments, TurnSegment{Type: "tool_calls", ToolCalls: groups[key]})
	}
	if len(invocationToolCalls) == 0 && len(ordered) <= 1 {
		segments = nil
	}
	return ConsolidatedTurnResult{Message: representative, Segments: segments}
}

func MessageTimelineItemKey(message Message) string {
	if message.Role != "user" && message.TurnID != nil && strings.TrimSpace(*message.TurnID) != "" {
		return "turn:" + strings.TrimSpace(*message.TurnID)
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
	for index := range nodes {
		if segments := segmentsByID[nodes[index].Message.ID]; len(segments) > 0 {
			nodes[index].Message.TurnSegments = segments
		}
		if len(nodes[index].Children) > 0 {
			nodes[index].Children = assignMessageNodeTurnSegments(nodes[index].Children, segmentsByID)
		}
	}
	return nodes
}

func assignMessageNodeOriginalIndexes(nodes []MessageNode, indexesByID map[string]int) []MessageNode {
	for index := range nodes {
		if original, ok := indexesByID[nodes[index].Message.ID]; ok {
			value := original
			nodes[index].OriginalIndex = &value
		}
		if len(nodes[index].Children) > 0 {
			nodes[index].Children = assignMessageNodeOriginalIndexes(nodes[index].Children, indexesByID)
		}
	}
	return nodes
}

func BuildNodesWithTimelineConsolidation(
	messages []Message,
	parentID *string,
	childCounts map[string]int,
	invocationToolCalls map[string][]TurnSegmentToolCall,
) []MessageNode {
	grouped := make(map[string][]Message)
	order := make([]string, 0, len(messages))
	for _, message := range messages {
		key := MessageTimelineItemKey(message)
		if _, found := grouped[key]; !found {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], message)
	}
	representatives := make([]Message, 0, len(order))
	segmentsByMessageID := make(map[string][]TurnSegment)
	for _, key := range order {
		itemMessages := grouped[key]
		representative := itemMessages[0]
		if strings.HasPrefix(key, "turn:") {
			turnID := strings.TrimPrefix(key, "turn:")
			result := ConsolidateTimelineTurn(itemMessages, invocationToolCalls[turnID])
			representative = result.Message
			segmentsByMessageID[representative.ID] = result.Segments
		}
		representatives = append(representatives, representative)
	}
	return assignMessageNodeTurnSegments(BuildMessageNodes(representatives, childCounts, parentID), segmentsByMessageID)
}

func BuildTimelineMessageNodes(
	items []database.MessageWindowItem,
	messages []Message,
	parentID *string,
	childCounts map[string]int,
	invocationToolCalls map[string][]TurnSegmentToolCall,
) []MessageNode {
	messagesByKey := make(map[string][]Message)
	for _, message := range messages {
		key := MessageTimelineItemKey(message)
		messagesByKey[key] = append(messagesByKey[key], message)
	}
	representatives := make([]Message, 0, len(items))
	indexes := make(map[string]int, len(items))
	segments := make(map[string][]TurnSegment)
	for _, item := range items {
		itemMessages := messagesByKey[timelineWindowItemKey(item)]
		if len(itemMessages) == 0 {
			continue
		}
		representative := itemMessages[0]
		if item.Kind == database.MessageWindowItemKindTurn {
			result := ConsolidateTimelineTurn(itemMessages, invocationToolCalls[item.TurnID])
			representative = result.Message
			segments[representative.ID] = result.Segments
		}
		representatives = append(representatives, representative)
		indexes[representative.ID] = item.OriginalIndex
	}
	nodes := BuildMessageNodes(representatives, childCounts, parentID)
	return assignMessageNodeTurnSegments(assignMessageNodeOriginalIndexes(nodes, indexes), segments)
}

func CollectTurnIDsWithToolCalls(messages []Message) []string {
	turnIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, message := range messages {
		if message.Role == "user" || message.TurnID == nil {
			continue
		}
		turnID := strings.TrimSpace(*message.TurnID)
		if turnID == "" {
			continue
		}
		if _, found := seen[turnID]; !found {
			seen[turnID] = struct{}{}
			turnIDs = append(turnIDs, turnID)
		}
	}
	return turnIDs
}
