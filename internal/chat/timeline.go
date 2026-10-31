package chat

import (
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"assistente/internal/core/ports"
	"assistente/internal/database"
)

// ConsolidatedTurnResult agrega a mensagem representante e seus segmentos
// canônicos. Invocações vêm exclusivamente do ledger.
type ConsolidatedTurnResult struct {
	Message  Message
	Segments []TurnSegment
}

func normalizeInvocationSummary(call TurnSegmentToolCall) TurnSegmentToolCall {
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
	iteration   int
	executionID string
	depth       int
	calls       []TurnSegmentToolCall
}

func groupCanonicalInvocations(calls []TurnSegmentToolCall) []canonicalInvocationGroup {
	type groupKey struct {
		executionID      string
		iteration, depth int
	}
	byIteration := make(map[groupKey][]TurnSegmentToolCall)
	for _, raw := range calls {
		call := normalizeInvocationSummary(raw)
		key := groupKey{call.ExecutionID, call.Iteration, call.ExecutionDepth}
		byIteration[key] = append(byIteration[key], call)
	}
	groups := make([]canonicalInvocationGroup, 0, len(byIteration))
	for iteration, calls := range byIteration {
		groups = append(groups, canonicalInvocationGroup{iteration: iteration.iteration, executionID: iteration.executionID, depth: iteration.depth, calls: calls})
	}
	// Iterações são numéricas e começam em zero. Dentro da rodada, preservar
	// a ordem queued_at,id da projeção; call IDs do provedor são opacos.
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].depth != groups[j].depth {
			return groups[i].depth > groups[j].depth
		}
		if groups[i].executionID != groups[j].executionID {
			return groups[i].executionID < groups[j].executionID
		}
		return groups[i].iteration < groups[j].iteration
	})
	return groups
}

// ConsolidateTimelineTurn constrói a timeline apenas com mensagens
// conversacionais e resumos leves do ledger.
func ConsolidateTimelineTurn(messages []Message, invocationToolCalls []TurnSegmentToolCall) ConsolidatedTurnResult {
	if len(messages) == 0 {
		return ConsolidatedTurnResult{}
	}
	ordered := orderedTurnMessages(messages)
	representative := legacyTurnRepresentative(ordered, invocationToolCalls)
	segments := projectResponseSegments(ordered, invocationToolCalls, &representative)
	return ConsolidatedTurnResult{Message: representative, Segments: legacyResponseSegments(segments, len(messages), len(invocationToolCalls))}
}

func orderedTurnMessages(messages []Message) []Message {
	if len(messages) < 2 {
		return messages
	}
	ordered := append([]Message(nil), messages...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})

	return ordered
}

func legacyTurnRepresentative(ordered []Message, invocationToolCalls []TurnSegmentToolCall) Message {
	// A fala que originou tools locais é intermediária. Não promovê-la a
	// conclusão se uma edição posterior atualizar seu timestamp. MCP nativo
	// pode apontar para o próprio registro final e não é esse marcador.
	intermediateIDs := make(map[string]bool)
	for _, call := range invocationToolCalls {
		if id := call.AssistantMessageID; strings.TrimSpace(id) != "" && call.Origin != "mcp_native" && (call.Origin != "acp_agent" || call.ACPTextOffset == nil) {
			intermediateIDs[id] = true
		}
	}
	return database.SelectLegacyTurnRepresentative(ordered, intermediateIDs)

}

func projectResponseSegments(ordered []Message, invocationToolCalls []TurnSegmentToolCall, representative *Message) []ResponseSegment {
	return projectResponseSegmentsWithSources(ordered, invocationToolCalls, representative, nil)
}

func projectResponseSegmentsWithSources(ordered []Message, invocationToolCalls []TurnSegmentToolCall, representative *Message, sources map[string][]ResponseSegment) []ResponseSegment {
	representativeID := ""
	if representative != nil {
		representativeID = representative.ID
	}
	positioned, remaining := positionedInvocations(ordered, invocationToolCalls)
	groups := groupCanonicalInvocations(remaining)
	segments := make([]ResponseSegment, 0, len(ordered)+len(groups))
	messageGroup := make(map[string]int)
	finalHasLocalCalls := false
	for index, group := range groups {
		for _, call := range group.calls {
			id := call.AssistantMessageID
			if strings.TrimSpace(id) == "" {
				continue
			}
			if _, found := messageGroup[id]; !found {
				messageGroup[id] = index
			}
			if id == representativeID && call.Origin != "mcp_native" {
				finalHasLocalCalls = true
			}
		}
	}
	appendText := func(message Message) {
		source, exists := sources[message.ID]
		if !exists {
			source = messageSegments(message)
		}
		segments = append(segments, positionSourceInvocations(source, positioned[message.ID])...)
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
		if message.ID == representativeID && !finalHasLocalCalls {
			continue
		}
		if bucket, found := messageGroup[message.ID]; found {
			nextBucket = bucket
		}
		messageBuckets[index] = nextBucket
	}
	for index, message := range ordered {
		if message.Role == "assistant" && (message.ID != representativeID || finalHasLocalCalls) {
			bucket := messageBuckets[index]
			textBuckets[bucket] = append(textBuckets[bucket], message)
		}
	}
	for index, texts := range textBuckets {
		for _, message := range texts {
			appendText(message)
		}
		if index < len(groups) {
			segments = append(segments, responseToolSegment(groups[index].calls))
		}
	}
	if representative != nil && representative.Role == "assistant" && !finalHasLocalCalls {
		appendText(*representative)
	}
	return segments
}

// Offsets válidos são aplicados por fonte. Uma chamada de outra origem não
// invalida posições já conhecidas, e cada invocação é consumida uma única vez.
func positionedInvocations(messages []Message, calls []TurnSegmentToolCall) (map[string][]TurnSegmentToolCall, []TurnSegmentToolCall) {
	hasPosition := false
	for _, call := range calls {
		if call.Origin == "acp_agent" && call.ACPTextOffset != nil {
			hasPosition = true
			break
		}
	}
	if !hasPosition {
		return nil, calls
	}
	texts := make(map[string]string, len(messages))
	for _, message := range messages {
		if message.Role == "assistant" {
			texts[message.ID] = message.Content
		}
	}
	positioned := make(map[string][]TurnSegmentToolCall)
	remaining := make([]TurnSegmentToolCall, 0, len(calls))
	for _, call := range calls {
		text, exists := texts[call.AssistantMessageID]
		if call.Origin == "acp_agent" && call.ACPTextOffset != nil && exists {
			offset := *call.ACPTextOffset
			if offset >= 0 && offset <= len(text) && (offset == len(text) || utf8.RuneStart(text[offset])) {
				positioned[call.AssistantMessageID] = append(positioned[call.AssistantMessageID], normalizeInvocationSummary(call))
				continue
			}
		}
		remaining = append(remaining, call)
	}
	return positioned, remaining
}

func messageSegments(message Message) []ResponseSegment {
	var segments []ResponseSegment
	if message.Reasoning != "" {
		segments = append(segments, responseTextSegment("reasoning", message, 0, message.Reasoning))
	}
	if message.Content != "" {
		segments = append(segments, responseTextSegment("text", message, 0, message.Content))
	}
	return segments
}

// Preserva fragmentos e suas identidades; apenas offsets conhecidos dividem um
// segmento. Não move raciocínio recebido após um corte para o início do stream.
func positionSourceInvocations(source []ResponseSegment, activity []TurnSegmentToolCall) []ResponseSegment {
	if len(activity) == 0 {
		return source
	}
	sort.SliceStable(activity, func(i, j int) bool { return *activity[i].ACPTextOffset < *activity[j].ACPTextOffset })
	var segments []ResponseSegment
	position, next := 0, 0
	appendCall := func(call TurnSegmentToolCall) {
		if len(segments) > 0 && segments[len(segments)-1].Kind == "tool_invocations" {
			segments[len(segments)-1].Invocations = append(segments[len(segments)-1].Invocations, call)
		} else {
			segments = append(segments, responseToolSegment([]TurnSegmentToolCall{call}))
		}
	}
	for _, segment := range source {
		if segment.Kind != "text" {
			segments = append(segments, segment)
			continue
		}
		local := 0
		appendText := func(end int) {
			if end == local {
				return
			}
			part := segment
			part.Text = segment.Text[local:end]
			if local != 0 {
				if segment.SourceMessageID != "" {
					part = responseTextSegment(segment.Kind, Message{UUIDModel: database.UUIDModel{ID: segment.SourceMessageID}}, local, part.Text)
				} else {
					part.SegmentID = segment.SegmentID + ":" + strconv.Itoa(local)
				}
			}
			segments = append(segments, part)
			local = end
		}
		for next < len(activity) && *activity[next].ACPTextOffset <= position+len(segment.Text) {
			appendText(*activity[next].ACPTextOffset - position)
			appendCall(activity[next])
			next++
		}
		appendText(len(segment.Text))
		position += len(segment.Text)
	}
	for next < len(activity) {
		appendCall(activity[next])
		next++
	}
	return segments
}

func MessageTimelineItemKey(message Message) string {
	if message.Role != "user" && message.TurnID != nil && strings.TrimSpace(*message.TurnID) != "" {
		return ports.ResponseItemID(*message.TurnID)
	}
	return ports.MessageItemID(message.ID)
}

func timelineWindowItemKey(item database.MessageWindowItem) string {
	if item.Kind == database.MessageWindowItemKindTurn {
		return ports.ResponseItemID(item.TurnID)
	}
	return ports.MessageItemID(item.MessageID)
}

func decorateTimelineNodes(nodes []MessageNode, segmentsByID map[string][]TurnSegment) []MessageNode {
	for index := range nodes {
		message := nodes[index].Message
		nodes[index].ItemID = ports.MessageItemID(message.ID)
		if message.Role != "user" && message.TurnID != nil && strings.TrimSpace(*message.TurnID) != "" {
			nodes[index].ItemID = ports.ResponseItemID(*message.TurnID)
		}
		if segments := segmentsByID[nodes[index].Message.ID]; len(segments) > 0 {
			nodes[index].Message.TurnSegments = segments
		}
		if len(nodes[index].Children) > 0 {
			nodes[index].Children = decorateTimelineNodes(nodes[index].Children, segmentsByID)
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
	return buildResponseNodes(messages, parentID, childCounts, invocationToolCalls, nil)
}

func buildResponseNodes(messages []Message, parentID *string, childCounts map[string]int, invocationToolCalls map[string][]TurnSegmentToolCall, responses map[string]*TimelineItem) []MessageNode {
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
			result := responseDisplay(turnID, itemMessages, invocationToolCalls[turnID], responses)
			representative = result.Message
			segmentsByMessageID[representative.ID] = result.Segments
		}
		representatives = append(representatives, representative)
	}
	return decorateTimelineNodes(BuildMessageNodes(representatives, childCounts, parentID), segmentsByMessageID)
}

func BuildTimelineMessageNodes(
	items []database.MessageWindowItem,
	messages []Message,
	parentID *string,
	childCounts map[string]int,
	invocationToolCalls map[string][]TurnSegmentToolCall,
) []MessageNode {
	return buildResponseWindowNodes(items, messages, parentID, childCounts, invocationToolCalls, nil)
}

func buildResponseWindowNodes(items []database.MessageWindowItem, messages []Message, parentID *string, childCounts map[string]int, invocationToolCalls map[string][]TurnSegmentToolCall, responses map[string]*TimelineItem) []MessageNode {
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
			result := responseDisplay(item.TurnID, itemMessages, invocationToolCalls[item.TurnID], responses)
			representative = result.Message
			segments[representative.ID] = result.Segments
		}
		representatives = append(representatives, representative)
		indexes[representative.ID] = item.OriginalIndex
	}
	nodes := BuildMessageNodes(representatives, childCounts, parentID)
	return decorateTimelineNodes(assignMessageNodeOriginalIndexes(nodes, indexes), segments)
}

func CollectTurnIDsWithToolCalls(messages []Message) []string {
	turnIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, message := range messages {
		if message.Role == "user" || message.TurnID == nil {
			continue
		}
		turnID := *message.TurnID
		if strings.TrimSpace(turnID) == "" {
			continue
		}
		if _, found := seen[turnID]; !found {
			seen[turnID] = struct{}{}
			turnIDs = append(turnIDs, turnID)
		}
	}
	return turnIDs
}

// Os leitores legados ainda não carregam metadados. O snapshot produtivo sempre
// fornece as projeções, compartilhadas pelos dois adaptadores.
func responseDisplay(turnID string, messages []Message, calls []TurnSegmentToolCall, responses map[string]*TimelineItem) ConsolidatedTurnResult {
	if response := responses[turnID]; response != nil {
		message, segments := response.legacyMessage()
		return ConsolidatedTurnResult{Message: message, Segments: segments}
	}
	return ConsolidateTimelineTurn(messages, calls)
}
