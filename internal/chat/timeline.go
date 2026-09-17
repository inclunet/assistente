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

type canonicalInvocationGroup struct {
	iteration int
	calls     []TurnSegmentToolCall
}

func groupCanonicalInvocations(calls []TurnSegmentToolCall) []canonicalInvocationGroup {
	byIteration := make(map[int][]TurnSegmentToolCall)
	for _, raw := range calls {
		call := normalizeInvocationSummary(raw)
		byIteration[call.Iteration] = append(byIteration[call.Iteration], call)
	}
	groups := make([]canonicalInvocationGroup, 0, len(byIteration))
	for iteration, calls := range byIteration {
		groups = append(groups, canonicalInvocationGroup{iteration: iteration, calls: calls})
	}
	// Iterações são numéricas e começam em zero. Dentro da rodada, preservar
	// a ordem queued_at,id da projeção; call IDs do provedor são opacos.
	sort.Slice(groups, func(i, j int) bool { return groups[i].iteration < groups[j].iteration })
	return groups
}

func timelineMessageWrittenAfter(left, right Message) bool {
	leftTime, rightTime := left.CreatedAt, right.CreatedAt
	if left.UpdatedAt.After(leftTime) {
		leftTime = left.UpdatedAt
	}
	if right.UpdatedAt.After(rightTime) {
		rightTime = right.UpdatedAt
	}
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	if (left.TotalTokens > 0) != (right.TotalTokens > 0) {
		return left.TotalTokens > 0
	}
	if !left.CreatedAt.Equal(right.CreatedAt) {
		return left.CreatedAt.After(right.CreatedAt)
	}
	return left.ID > right.ID
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

	// A fala que originou tools locais é intermediária. Não promovê-la a
	// conclusão se uma edição posterior atualizar seu timestamp. MCP nativo
	// pode apontar para o próprio registro final e não é esse marcador.
	intermediateIDs := make(map[string]bool)
	for _, call := range invocationToolCalls {
		if id := strings.TrimSpace(call.AssistantMessageID); id != "" && call.Origin != "mcp_native" {
			intermediateIDs[id] = true
		}
	}
	hasFinalCandidate := false
	for _, message := range ordered {
		if message.Role == "assistant" && !intermediateIDs[message.ID] {
			hasFinalCandidate = true
			break
		}
	}
	representative := ordered[0]
	hasAssistant := false
	for _, message := range ordered {
		if message.Role == "assistant" && (!hasFinalCandidate || !intermediateIDs[message.ID]) {
			// O registro final pode ser criado antes das rodadas e atualizado
			// no encerramento. Usar a escrita, não só criação ou usage.
			if !hasAssistant || timelineMessageWrittenAfter(message, representative) {
				representative = message
			}
			hasAssistant = true
		}
	}

	groups := groupCanonicalInvocations(invocationToolCalls)
	segments := make([]TurnSegment, 0, len(ordered)+len(groups))
	messageGroup := make(map[string]int)
	finalHasLocalCalls := false
	for index, group := range groups {
		for _, call := range group.calls {
			id := strings.TrimSpace(call.AssistantMessageID)
			if id == "" {
				continue
			}
			if _, found := messageGroup[id]; !found {
				messageGroup[id] = index
			}
			if id == representative.ID && call.Origin != "mcp_native" {
				finalHasLocalCalls = true
			}
		}
	}
	appendText := func(message Message) {
		if strings.TrimSpace(message.Content) != "" {
			segments = append(segments, TurnSegment{Type: "text", Content: message.Content})
		}
	}
	// Vínculos do ledger colocam a fala antes das tools da própria rodada.
	// Texto antigo sem vínculo preserva sua ordem antes da próxima fala ligada;
	// sem nenhum vínculo disponível, mantém-se texto → tools, sem inventar IDs.
	textBuckets := make([][]Message, len(groups)+1)
	messageBuckets := make([]int, len(ordered))
	nextBucket := len(groups)
	if len(messageGroup) == 0 {
		nextBucket = 0
	}
	for index := len(ordered) - 1; index >= 0; index-- {
		message := ordered[index]
		if message.ID == representative.ID && !finalHasLocalCalls {
			continue
		}
		if bucket, found := messageGroup[message.ID]; found {
			nextBucket = bucket
		}
		messageBuckets[index] = nextBucket
	}
	for index, message := range ordered {
		if message.Role == "assistant" && (message.ID != representative.ID || finalHasLocalCalls) {
			bucket := messageBuckets[index]
			textBuckets[bucket] = append(textBuckets[bucket], message)
		}
	}
	for index, texts := range textBuckets {
		for _, message := range texts {
			appendText(message)
		}
		if index < len(groups) {
			segments = append(segments, TurnSegment{Type: "tool_calls", ToolCalls: groups[index].calls})
		}
	}
	if hasAssistant && !finalHasLocalCalls {
		appendText(representative)
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
