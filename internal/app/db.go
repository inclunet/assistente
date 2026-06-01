package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
)

// Re-exporta tipos do pacote database para manter compatibilidade
type Conversation = database.Conversation
type ChatMessage = database.ChatMessage

// Re-exporta funções que não dependem de App
var (
	InitDatabase  = database.Init
	GenerateTitle = database.GenerateTitle
)

// ==================== Conversation ====================

func (a *App) CreateConversation(title, model string) (*Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.CreateConversationWithContext(ctx, title, model)
}

func (a *App) GetConversations() ([]Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.GetConversationsWithContext(ctx)
}

func (a *App) GetConversation(id string) (*Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.GetConversationWithContext(ctx, id)
}

// EnsureConversation cria ou recicla uma conversa vazia e retorna.
// Usada pelo frontend quando uma aba de chat do workspace é criada sem contentId.
func (a *App) EnsureConversation(title string) (*Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = "Nova Conversa"
	}
	conv, err := database.RecycleOrCreateConversationWithContext(ctx, title)
	if err != nil {
		return nil, fmt.Errorf("erro ao garantir conversa: %w", err)
	}
	return conv, nil
}

// GetMessages retorna mensagens com filtro por parent (API unificada com LAZY LOADING)
func (a *App) GetMessages(conversationID string, parentID *string) ([]chat.MessageNode, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	messages, err := a.msgRepo.GetMessages(ctx, conversationID, parentID)
	if err != nil {
		return nil, err
	}

	return buildMessageNodesWithInvocationFallback(ctx, messages, parentID), nil
}

func buildMessageNodes(ctx context.Context, messages []database.ChatMessage, parentID *string) []chat.MessageNode {
	if len(messages) == 0 {
		return []chat.MessageNode{}
	}
	msgIDs := make([]string, len(messages))
	for i, msg := range messages {
		msgIDs[i] = msg.ID
	}
	childCounts, err := database.CountChildrenWithContext(ctx, msgIDs)
	if err != nil {
		childCounts = make(map[string]int)
	}
	return chat.BuildMessageNodes(messages, childCounts, parentID)
}

func buildMessageNodesWithInvocationFallback(ctx context.Context, messages []database.ChatMessage, parentID *string) []chat.MessageNode {
	if len(messages) == 0 {
		return []chat.MessageNode{}
	}

	grouped := make(map[string][]database.ChatMessage)
	order := make([]string, 0, len(messages))
	seenKey := map[string]struct{}{}
	turnIDs := make([]string, 0)
	seenTurn := map[string]struct{}{}
	for _, msg := range messages {
		key := messageTimelineItemKey(msg)
		grouped[key] = append(grouped[key], msg)
		if _, ok := seenKey[key]; !ok {
			seenKey[key] = struct{}{}
			order = append(order, key)
		}
		// Só hidrata turnos que realmente têm uso de tools: assistant com ToolCalls ou
		// mensagens role=tool com ToolCallID (inclui tool-only turns/placeholder).
		shouldHydrate := false
		if msg.Role == "assistant" && strings.TrimSpace(msg.ToolCalls) != "" {
			shouldHydrate = true
		}
		if msg.Role == "tool" && strings.TrimSpace(msg.ToolCallID) != "" {
			shouldHydrate = true
		}
		if shouldHydrate && msg.TurnID != nil {
			turnID := strings.TrimSpace(*msg.TurnID)
			if turnID != "" {
				if _, ok := seenTurn[turnID]; !ok {
					seenTurn[turnID] = struct{}{}
					turnIDs = append(turnIDs, turnID)
				}
			}
		}
	}

	invocationToolResults := loadChatToolInvocationResultsForTurnIDs(ctx, turnIDs)
	representatives := make([]database.ChatMessage, 0, len(order))
	segmentsByMessageID := make(map[string][]chat.TurnSegment)
	for _, key := range order {
		itemMessages := grouped[key]
		if len(itemMessages) == 0 {
			continue
		}
		representative := itemMessages[0]
		if strings.HasPrefix(key, "turn:") && representative.TurnID != nil {
			turnID := strings.TrimSpace(*representative.TurnID)
			turnResults := invocationToolResults[turnID]
			result := consolidateTimelineTurn(itemMessages, turnResults)
			representative = result.Message
			if len(result.Segments) > 0 {
				segmentsByMessageID[representative.ID] = result.Segments
			}
		}
		representatives = append(representatives, representative)
	}
	return assignMessageNodeTurnSegments(buildMessageNodes(ctx, representatives, parentID), segmentsByMessageID)
}

// assignMessageNodeTurnSegments anexa segmentos canônicos de turno aos nós já
// construídos. Mantemos a atribuição em uma única passagem para evitar mutação
// implícita dentro do builder e para que a árvore (com filhos lazy-loaded)
// também receba os segmentos quando aplicável (Issue #150).
func assignMessageNodeTurnSegments(nodes []chat.MessageNode, segmentsByID map[string][]chat.TurnSegment) []chat.MessageNode {
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

func assignMessageNodeOriginalIndexes(nodes []chat.MessageNode, indexesByID map[string]int) []chat.MessageNode {
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

func messageTimelineItemKey(message database.ChatMessage) string {
	// Produces the canonical key shared with frontend getTimelineNodeKey.
	// User messages stay standalone even if bad data carries TurnID; TurnID only groups assistant/tool responses.
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

// Contract with the frontend renderer: this source marks a synthetic assistant placeholder
// for turns that only persisted tool result messages and have no assistant response.
const toolOnlyTurnPlaceholderSource = "tool_only_turn_placeholder"

var invalidToolCallsLogState = struct {
	sync.Mutex
	seen map[string]struct{}
}{
	seen: make(map[string]struct{}),
}

func shouldLogInvalidToolCalls(messageID string) bool {
	invalidToolCallsLogState.Lock()
	defer invalidToolCallsLogState.Unlock()
	if _, ok := invalidToolCallsLogState.seen[messageID]; ok {
		return false
	}
	invalidToolCallsLogState.seen[messageID] = struct{}{}
	return true
}

func parseToolCalls(messageID string, raw string) []map[string]interface{} {
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
		log.Printf("[Chat] tool_calls JSON inválido descartado message_id=%s: array=%v object=%v", messageID, arrayErr, singleErr)
	}
	return nil
}

// consolidatedTurnResult agrega o representante do turno (uma única ChatMessage
// canônica para a timeline) e os segmentos cronológicos do turno (Issue #150),
// usados pelo frontend para renderizar texto → tools → texto → tools → resposta
// final dentro de UMA única entrada do message history acessível.
type consolidatedTurnResult struct {
	Message  database.ChatMessage
	Segments []chat.TurnSegment
}

func toolCallToTurnSegmentToolCall(call map[string]interface{}) chat.TurnSegmentToolCall {
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
	return chat.TurnSegmentToolCall{
		ID:       id,
		Type:     tipo,
		Function: chat.TurnSegmentToolFunction{Name: name, Arguments: args},
		Result:   result,
	}
}

func consolidateTimelineTurnMessages(messages []database.ChatMessage, invocationToolResults map[string]string) database.ChatMessage {
	return consolidateTimelineTurn(messages, invocationToolResults).Message
}

// consolidateTimelineTurn produz a representação canônica de um turno do
// assistente para o timeline do chat: uma `database.ChatMessage` representativa
// (compatível com a paginação por timeline_items e com a UI legada que lê
// `content` + `toolCalls`) e a lista cronológica de segmentos `chat.TurnSegment`
// (usada pelo frontend acessível para preservar a cadeia de raciocínio em uma
// única entrada do message history — Issue #150).
func consolidateTimelineTurn(messages []database.ChatMessage, invocationToolResults map[string]string) consolidatedTurnResult {
	if len(messages) == 0 {
		return consolidatedTurnResult{}
	}
	messages = append([]database.ChatMessage(nil), messages...)
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
	// callID is immutable within a turn; keep the first assistant emission as canonical
	// and enrich it with the persisted tool result when available.
	seenToolCallIDs := make(map[string]struct{})
	// Segmentos cronológicos do turno: cada iteração do agentic loop produz
	// (texto opcional) seguido de (tool_calls opcional). A última iteração tem
	// só texto (resposta final). Issue #150.
	segments := make([]chat.TurnSegment, 0)
	assistantCount := 0
	for _, message := range messages {
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
		// Texto desta iteração é um segmento próprio quando não vazio. Mantém a
		// ordem cronológica para que NVDA leia a cadeia de raciocínio inteira.
		if strings.TrimSpace(message.Content) != "" {
			segments = append(segments, chat.TurnSegment{
				Type:    "text",
				Content: message.Content,
			})
		}
		iterationCalls := make([]chat.TurnSegmentToolCall, 0)
		for _, call := range parseToolCalls(message.ID, message.ToolCalls) {
			callID, _ := call["id"].(string)
			if callID != "" {
				if _, seen := seenToolCallIDs[callID]; seen {
					continue
				}
				seenToolCallIDs[callID] = struct{}{}
				if existing, ok := call["result"].(string); ok {
					if strings.TrimSpace(existing) != "" {
						// Não sobrescreve resultado já embutido (ex.: import/export).
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
			segments = append(segments, chat.TurnSegment{
				Type:      "tool_calls",
				ToolCalls: iterationCalls,
			})
		}
	}
	if !hasAssistant {
		// Keep the persisted tool message ID as representative so backend pagination anchors
		// still resolve to a real row; frontend delete shortcuts must treat this source as non-deletable.
		consolidated.Role = "assistant"
		consolidated.Content = ""
		consolidated.Reasoning = ""
		consolidated.ToolCallID = ""
		consolidated.Source = toolOnlyTurnPlaceholderSource
		placeholderCalls := make([]map[string]interface{}, 0, len(toolResults))
		segmentToolCalls := make([]chat.TurnSegmentToolCall, 0, len(toolResults))
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
		var placeholderSegments []chat.TurnSegment
		if len(segmentToolCalls) > 0 {
			placeholderSegments = []chat.TurnSegment{{
				Type:      "tool_calls",
				ToolCalls: segmentToolCalls,
			}}
		}
		return consolidatedTurnResult{Message: consolidated, Segments: placeholderSegments}
	}
	consolidated.Content = finalContent
	consolidated.Reasoning = finalReasoning
	if len(allToolCalls) > 0 {
		if encoded, err := json.Marshal(allToolCalls); err == nil {
			consolidated.ToolCalls = string(encoded)
		}
	}
	// Turnos triviais (apenas resposta final, sem ferramentas) não precisam de
	// segments — o renderizador legado de uma única bolha cobre o caso e evita
	// payload extra na timeline.
	if assistantCount <= 1 && len(allToolCalls) == 0 {
		return consolidatedTurnResult{Message: consolidated, Segments: nil}
	}
	return consolidatedTurnResult{Message: consolidated, Segments: segments}
}

func buildTimelineMessageNodes(ctx context.Context, items []database.MessageWindowItem, messages []database.ChatMessage, parentID *string) []chat.MessageNode {
	invocationToolResults := loadChatToolInvocationResultsForTurnIDs(ctx, collectTurnIDsWithToolCalls(messages))
	messagesByItemKey := make(map[string][]database.ChatMessage)
	for _, message := range messages {
		key := messageTimelineItemKey(message)
		messagesByItemKey[key] = append(messagesByItemKey[key], message)
	}
	representatives := make([]database.ChatMessage, 0, len(items))
	originalIndexesByMessageID := make(map[string]int, len(items))
	segmentsByMessageID := make(map[string][]chat.TurnSegment)
	for _, item := range items {
		itemMessages := messagesByItemKey[timelineWindowItemKey(item)]
		if len(itemMessages) == 0 {
			continue
		}
		representative := itemMessages[0]
		if item.Kind == database.MessageWindowItemKindTurn {
			turnResults := invocationToolResults[strings.TrimSpace(item.TurnID)]
			result := consolidateTimelineTurn(itemMessages, turnResults)
			representative = result.Message
			if len(result.Segments) > 0 {
				segmentsByMessageID[representative.ID] = result.Segments
			}
		}
		representatives = append(representatives, representative)
		originalIndexesByMessageID[representative.ID] = item.OriginalIndex
	}
	nodes := assignMessageNodeOriginalIndexes(buildMessageNodes(ctx, representatives, parentID), originalIndexesByMessageID)
	return assignMessageNodeTurnSegments(nodes, segmentsByMessageID)
}

func collectTurnIDsWithToolCalls(messages []database.ChatMessage) []string {
	turnIDs := make([]string, 0)
	seen := map[string]struct{}{}
	for _, msg := range messages {
		if msg.TurnID == nil {
			continue
		}
		shouldHydrate := false
		if msg.Role == "assistant" && strings.TrimSpace(msg.ToolCalls) != "" {
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

func loadChatToolInvocationResultsForTurnIDs(ctx context.Context, turnIDs []string) map[string]map[string]string {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return map[string]map[string]string{}
	}
	return loadChatToolInvocationResultsForTurnIDsWithUser(ctx, userID, turnIDs)
}

func loadChatToolInvocationResultsForTurnIDsWithUser(ctx context.Context, userID string, turnIDs []string) map[string]map[string]string {
	if len(turnIDs) == 0 {
		return map[string]map[string]string{}
	}
	results, err := toolinvocations.LoadChatToolInvocationResultsForTurnIDsWithUser(ctx, userID, turnIDs)
	if err != nil {
		log.Printf("[Chat] load tool_invocations results failed: %v", err)
		return map[string]map[string]string{}
	}
	return results
}

// GetRecentMessages retorna as mensagens raiz mais recentes de uma conversa.
func (a *App) GetRecentMessages(conversationID string, limit int) ([]chat.MessageNode, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []chat.MessageNode{}, nil
	}
	// A API legada pagina por mensagens, mas o renderer colapsa múltiplas linhas do mesmo turno
	// em um único representative. Faz overfetch incremental até preencher o limit.
	rawLimit := limit
	if rawLimit < 10 {
		rawLimit = 10
	}
	if rawLimit > 5000 {
		rawLimit = 5000
	}
	var lastNodes []chat.MessageNode
	for {
		messages, err := database.GetRecentRootMessagesWithContext(ctx, conversationID, rawLimit)
		if err != nil {
			return nil, err
		}
		nodes := buildMessageNodesWithInvocationFallback(ctx, messages, nil)
		lastNodes = nodes
		if len(nodes) >= limit || len(messages) < rawLimit {
			if len(nodes) > limit {
				return nodes[len(nodes)-limit:], nil
			}
			return nodes, nil
		}
		if rawLimit >= 5000 {
			break
		}
		rawLimit *= 2
		if rawLimit > 5000 {
			rawLimit = 5000
		}
	}
	if len(lastNodes) > limit {
		return lastNodes[len(lastNodes)-limit:], nil
	}
	return lastNodes, nil
}

// GetMessagesBefore retorna mensagens raiz anteriores ao cursor informado.
func (a *App) GetMessagesBefore(conversationID string, beforeID string, limit int) ([]chat.MessageNode, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return []chat.MessageNode{}, nil
	}
	rawLimit := limit
	if rawLimit < 10 {
		rawLimit = 10
	}
	if rawLimit > 5000 {
		rawLimit = 5000
	}
	var lastNodes []chat.MessageNode
	for {
		messages, err := database.GetRootMessagesBeforeWithContext(ctx, conversationID, beforeID, rawLimit)
		if err != nil {
			return nil, err
		}
		nodes := buildMessageNodesWithInvocationFallback(ctx, messages, nil)
		lastNodes = nodes
		if len(nodes) >= limit || len(messages) < rawLimit {
			if len(nodes) > limit {
				return nodes[len(nodes)-limit:], nil
			}
			return nodes, nil
		}
		if rawLimit >= 5000 {
			break
		}
		rawLimit *= 2
		if rawLimit > 5000 {
			rawLimit = 5000
		}
	}
	if len(lastNodes) > limit {
		return lastNodes[len(lastNodes)-limit:], nil
	}
	return lastNodes, nil
}

// GetConversationMessageWindow é a API canônica de carregamento incremental de mensagens.
// Ela cobre conversa raiz e filhos diretos de thread com o mesmo contrato total-aware.
func (a *App) GetConversationMessageWindow(req chat.MessageWindowRequest) (*chat.MessageWindow, error) {
	conversationID := strings.TrimSpace(req.ConversationID)
	if conversationID == "" {
		return nil, fmt.Errorf("conversationId é obrigatório")
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = chat.MessageWindowScopeConversation
	}
	if scope != chat.MessageWindowScopeConversation && scope != chat.MessageWindowScopeThread {
		return nil, fmt.Errorf("scope de janela de mensagens inválido: %s", req.Scope)
	}
	if req.Limit <= 0 {
		return nil, fmt.Errorf("limit deve ser maior que zero")
	}
	limit := req.Limit
	if limit > database.MaxMessageWindowRows {
		limit = database.MaxMessageWindowRows
	}

	anchor := strings.TrimSpace(req.Anchor)
	anchorMessageID := strings.TrimSpace(req.AnchorMessageID)
	direction := strings.TrimSpace(req.Direction)
	if direction == "" {
		direction = chat.MessageWindowDirectionBefore
	}
	if direction != chat.MessageWindowDirectionBefore &&
		direction != chat.MessageWindowDirectionAfter &&
		direction != chat.MessageWindowDirectionAround {
		return nil, fmt.Errorf("direction de janela de mensagens inválido: %s", req.Direction)
	}
	if anchor != "" &&
		anchor != chat.MessageWindowAnchorStart &&
		anchor != chat.MessageWindowAnchorEnd {
		return nil, fmt.Errorf("anchor de janela de mensagens inválido: %s", req.Anchor)
	}
	if anchor != "" && anchorMessageID != "" {
		return nil, fmt.Errorf("anchor e anchorMessageId são mutuamente exclusivos")
	}
	if anchor == chat.MessageWindowAnchorStart && direction == chat.MessageWindowDirectionBefore {
		return nil, fmt.Errorf("anchor=start não aceita direction=before")
	}
	if anchor == chat.MessageWindowAnchorEnd && direction == chat.MessageWindowDirectionAfter {
		return nil, fmt.Errorf("anchor=end não aceita direction=after")
	}
	if direction == chat.MessageWindowDirectionAround && anchorMessageID == "" {
		return nil, fmt.Errorf("direction=around exige anchorMessageId")
	}
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}

	var parentID *string
	threadParentID := ""
	if scope == chat.MessageWindowScopeThread {
		threadParentID = strings.TrimSpace(req.ThreadParentID)
		if threadParentID == "" {
			return nil, fmt.Errorf("threadParentId é obrigatório para scope=thread")
		}
		parentMessage, err := database.GetMessageWithContext(ctx, threadParentID)
		if err != nil {
			return nil, fmt.Errorf("threadParentId inválido: %w", err)
		}
		if parentMessage.ConversationID != conversationID {
			return nil, fmt.Errorf("threadParentId não pertence à conversa solicitada")
		}
		if parentMessage.ParentID != nil {
			return nil, fmt.Errorf("threadParentId deve apontar para uma mensagem raiz")
		}
		parentID = &threadParentID
	}

	window, err := database.GetMessageWindowWithContext(ctx, database.MessageWindowQuery{
		ConversationID:  conversationID,
		ParentID:        parentID,
		Anchor:          anchor,
		AnchorMessageID: anchorMessageID,
		Direction:       direction,
		Limit:           limit,
	})
	if err != nil {
		return nil, err
	}
	nodes := buildTimelineMessageNodes(ctx, window.Items, window.Messages, parentID)

	return &chat.MessageWindow{
		Scope:          scope,
		ConversationID: conversationID,
		ThreadParentID: threadParentID,
		Nodes:          nodes,
		TotalCount:     window.TotalCount,
		StartIndex:     window.StartIndex,
		EndIndex:       window.EndIndex,
		HasBefore:      window.HasBefore,
		HasAfter:       window.HasAfter,
	}, nil
}

// GetConversationInfo retorna apenas metadados da conversa (sem mensagens)
func (a *App) GetConversationInfo(id string) (*Conversation, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.GetConversationInfoWithContext(ctx, id)
}

// GetConversationWithThreads retorna conversa com mensagens raiz (lazy loading)
func (a *App) GetConversationWithThreads(id string) (*chat.ConversationWithThreads, error) {
	conv, err := a.GetConversationInfo(id)
	if err != nil {
		return nil, err
	}

	threads, err := a.GetMessages(id, nil)
	if err != nil {
		return nil, err
	}

	return &chat.ConversationWithThreads{
		ID:      conv.ID,
		Title:   conv.Title,
		Threads: threads,
	}, nil
}

// GetMessageChildren retorna os filhos de uma mensagem (lazy loading)
func (a *App) GetMessageChildren(messageID string) ([]chat.MessageNode, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	parent, err := database.GetMessageWithContext(ctx, messageID)
	if err != nil {
		return nil, err
	}
	return a.GetMessages(parent.ConversationID, &messageID)
}

func (a *App) UpdateConversation(id string, title, model string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	if err := database.UpdateConversationWithContext(ctx, id, title, model); err != nil {
		return err
	}

	if title != "" {
		a.emitter.Emit("conversation:renamed", ports.ConversationRenamedEvent{
			ConversationID: id,
			NewTitle:       title,
		})
	}

	return nil
}

func (a *App) DeleteConversation(id string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	if err := database.DeleteConversationWithContext(ctx, id); err != nil {
		return err
	}

	a.emitter.Emit("conversation:deleted", map[string]interface{}{
		"conversation_id": id,
	})

	return nil
}

// DeleteMessage exclui uma mensagem e todas as suas filhas (respostas)
func (a *App) DeleteMessage(messageID string) error {
	// Solicita confirmação via questionário
	ctx := a.ctx
	if a.questionnaireMgr == nil {
		return fmt.Errorf("questionnaire manager não inicializado")
	}

	resp, err := a.questionnaireMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       "Excluir mensagem",
		Description: "Tem certeza que deseja excluir esta mensagem e todas as suas respostas? Esta ação não pode ser desfeita.",
		AllowCancel: true,
		SubmitLabel: "Excluir",
		CancelLabel: "Cancelar",
		Questions: []questionnaire.Question{
			{
				ID:       "confirm",
				Type:     "boolean",
				Prompt:   "Confirmar exclusão?",
				Required: true,
			},
		},
	})
	if err != nil {
		return err
	}
	if resp.Cancelled {
		return fmt.Errorf("exclusão cancelada pelo usuário")
	}
	confirmed, ok := resp.Answers["confirm"].(bool)
	if !ok || !confirmed {
		return fmt.Errorf("exclusão cancelada pelo usuário")
	}

	// Prossegue com a exclusão
	authCtx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	msg, err := database.GetMessageWithContext(authCtx, messageID)
	if err != nil {
		return err
	}
	convID := msg.ConversationID

	if err := database.DeleteMessageWithContext(authCtx, messageID); err != nil {
		return err
	}

	a.emitter.Emit("message:deleted", ports.MessageDeletedEvent{
		ConversationID: convID,
		MessageID:      messageID,
	})

	return nil
}

// UpdateMessage atualiza o conteúdo de uma mensagem existente
func (a *App) UpdateMessage(messageID string, newContent string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	msg, err := database.GetMessageWithContext(ctx, messageID)
	if err != nil {
		return err
	}

	if err := database.UpdateMessageContentWithContext(ctx,
		messageID,
		newContent,
		0, 0, 0, "",
	); err != nil {
		return err
	}

	a.emitter.Emit("message:updated", ports.MessageUpdatedEvent{
		ConversationID: msg.ConversationID,
		MessageID:      messageID,
		Content:        newContent,
	})

	return nil
}

func (a *App) UpdateConversationModel(id string, model string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return database.UpdateConversationWithContext(ctx, id, "", model)
}

// ==================== ChatMessage ====================

func (a *App) CreateMessage(conversationID string, role, content string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

func (a *App) AddMessage(conversationID string, role, content string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{ConversationID: conversationID, Role: role, Content: content})
}

func (a *App) AddMessageWithMedia(conversationID string, role, content, media string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{ConversationID: conversationID, Role: role, Content: content, Media: media})
}

func (a *App) AddMessageWithTokens(conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

func (a *App) AddMessageWithTokensAndMedia(conversationID string, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		Media:            media,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

func (a *App) AddChildMessage(conversationID string, parentID string, role, content, model string) (*ChatMessage, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return a.msgRepo.CreateMessage(ctx, database.MessageOptions{
		ConversationID: conversationID,
		ParentID:       &parentID,
		Role:           role,
		Content:        content,
		Model:          model,
	})
}

func (a *App) GetAllTokenStats() (map[string]int, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.GetAllTokenStatsWithContext(ctx)
}

// ==================== Rolling Context (Summary) ====================

type ConversationSummaryInfo struct {
	Summary               string `json:"summary"`
	SummaryUpToMessageID  string `json:"summary_up_to_message_id"`
	SummarizingInProgress bool   `json:"summarizing_in_progress"`
}

func (a *App) GetConversationSummary(conversationID string) (*ConversationSummaryInfo, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return nil, err
	}
	summary, upToID, err := database.GetConversationSummaryWithContext(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	inProgress, _ := database.IsSummarizingInProgressWithContext(ctx, conversationID)
	return &ConversationSummaryInfo{
		Summary:               summary,
		SummaryUpToMessageID:  upToID,
		SummarizingInProgress: inProgress,
	}, nil
}

func (a *App) RenameConversation(conversationID string, newTitle string) error {
	return a.UpdateConversation(conversationID, newTitle, "")
}

func (a *App) ClearConversation(conversationID string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	if err := database.DeleteAllMessagesWithContext(ctx, conversationID); err != nil {
		return err
	}

	a.emitter.Emit("conversation:cleared", map[string]interface{}{
		"conversation_id": conversationID,
	})

	return nil
}

func (a *App) DeleteMessages(conversationID string, messageIDs []string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	if _, err := database.GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	for _, msgID := range messageIDs {
		msg, err := database.GetMessageWithContext(ctx, msgID)
		if err != nil {
			return fmt.Errorf("erro ao localizar mensagem %s: %w", msgID, err)
		}
		if msg.ConversationID != conversationID {
			return fmt.Errorf("mensagem %s não pertence à conversa solicitada", msgID)
		}
		if err := database.DeleteMessageWithContext(ctx, msgID); err != nil {
			return fmt.Errorf("erro ao deletar mensagem %s: %w", msgID, err)
		}
	}
	return nil
}

// ==================== Search ====================

// MessageSearchResult re-exporta o tipo do database
type MessageSearchResult = database.MessageSearchResult

// SearchConversationHistory busca no conteúdo de todas as mensagens usando FTS5.
// Suporta palavras, "frases exatas", prefixo*, operadores OR/AND/NOT.
func (a *App) SearchConversationHistory(query string, limit int) ([]MessageSearchResult, error) {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return nil, err
	}
	return database.SearchMessageContentWithContext(ctx, query, limit)
}

// RebuildSearchIndex reconstrói o índice de busca full-text.
//
// Operação instance-wide (afeta o índice de todos os usuários), mas o
// binding Wails exige autenticação: AEP-0052 mantém o invariante de que
// nenhum entry point publico ao frontend roda sem sessão ativa, mesmo
// quando a operação interna nao tem escopo de usuário (defesa em
// profundidade contra disparos pré-login).
func (a *App) RebuildSearchIndex() error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return database.RebuildFTSIndex(ctx)
}

// ==================== Model ====================

// SetConversationModel define o modelo para uma conversa
func (a *App) SetConversationModel(conversationID string, model string) error {
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		return err
	}
	return database.UpdateConversationWithContext(ctx, conversationID, "", model)
}

// GetEffectiveModel retorna o modelo efetivo (perfil ativo > config padrão)
func (a *App) GetEffectiveModel() (string, error) {
	// Tenta obter do perfil ativo
	activeProfile, err := a.profileManager.GetActive()
	if err == nil && activeProfile != nil && activeProfile.Chat.Model != "" {
		return activeProfile.Chat.Model, nil
	}

	// Fallback para config
	cfg, err := a.settingsSvc.GetConfig()
	if err != nil {
		return "", err
	}
	return cfg.DefaultModel, nil
}
