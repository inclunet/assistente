package chat

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/database"
)

// TimelineItem é a projeção interna de mensagens e respostas. O envelope wire
// é publicado pelo projetor, que mantém época e revisão próprias.
type TimelineItem struct {
	Thinking          bool
	TerminalReason    string
	ExecutionID       string
	RecoveryReference *TimelineRecoveryReference
	messages          []Message
	invocations       []TurnSegmentToolCall
	ConversationID    string
	ScopeParentID     *string
	Kind              string
	Position          TimelinePosition
	Durability        string
	ItemID            string
	TurnID            string
	Representative    *Message
	State             string
	Origin            string
	Segments          []ResponseSegment
	messageCount      int
	callCount         int
}

type ResponseSegment struct {
	SourceMessageID string
	SegmentID       string
	Kind            string
	Text            string
	Invocations     []TurnSegmentToolCall
}

func responseTextSegment(kind string, message Message, start int, text string) ResponseSegment {
	return ResponseSegment{SourceMessageID: message.ID, SegmentID: kind + ":" + base64.RawURLEncoding.EncodeToString([]byte(message.ID)) + ":" + strconv.Itoa(start), Kind: kind, Text: text}
}

func responseToolSegment(calls []TurnSegmentToolCall) ResponseSegment {
	// O primeiro membro identifica o grupo. Crescer o grupo preserva a identidade.
	id := calls[0].InvocationID
	return ResponseSegment{SegmentID: "tools:" + base64.RawURLEncoding.EncodeToString([]byte(id)), Kind: "tool_invocations", Invocations: calls}
}

// ProjectPersistedResponse valida vínculos e projeta o conteúdo confirmado do turno.
// Fontes e turno devem vir da mesma leitura transacional, filtrada pela seleção
// de execução do banco. Continue pode incluir bases explícitas; retry não as herda.
func ProjectPersistedResponse(conversationID, turnID string, parentID *string, messages []Message, calls []TurnSegmentToolCall, turn *database.ChatTurn) (*TimelineItem, error) {
	result := &TimelineItem{ItemID: ports.ResponseItemID(turnID), TurnID: turnID, State: database.TurnHistoricalUnknown, Origin: "legacy", ConversationID: conversationID, ScopeParentID: parentID, Kind: "response", Durability: "persisted"}
	for _, message := range messages {
		if message.ConversationID != conversationID || message.TurnID == nil || *message.TurnID != turnID || !sameScope(message.ParentID, parentID) || message.Role == "user" {
			return nil, fmt.Errorf("mensagem fora do escopo da resposta %s", turnID)
		}
	}
	for _, call := range calls {
		if call.InvocationID == "" {
			return nil, fmt.Errorf("invocação sem identidade persistida no turno %s", turnID)
		}
	}
	ordered := orderedTurnMessages(messages)
	if turn == nil {
		if len(ordered) > 0 {
			message := legacyTurnRepresentative(ordered, calls)
			result.Representative = &message
		}
	} else {
		if turn.UserMessageID != turnID || turn.ConversationID != conversationID {
			return nil, database.ErrChatTurnInvalid
		}
		result.State, result.Origin = turn.State, turn.Origin
		result.TerminalReason = turn.TerminalReason
		if turn.CurrentExecutionID != nil {
			result.ExecutionID = *turn.CurrentExecutionID
		}
		if turn.ResponseMessageID != nil {
			for _, message := range ordered {
				if message.ID == *turn.ResponseMessageID && message.Role == "assistant" {
					result.Representative = &message
					break
				}
			}
			if result.Representative == nil {
				return nil, fmt.Errorf("representante persistido fora da resposta %s", turnID)
			}
		}
	}
	result.messages, result.invocations = ordered, calls
	result.Segments = projectResponseSegments(ordered, calls, result.Representative)
	result.messageCount, result.callCount = len(messages), len(calls)
	return result, nil
}

func sameScope(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// TerminalPatch preserva o evento terminal ainda consumido por integrações.
// Somente o representante explícito comprova a resposta confirmada no storage.
func (r *TimelineItem) TerminalPatch() *ports.TurnPatchEvent {
	if r == nil || r.Representative == nil {
		return nil
	}
	message := r.Representative
	patch := &ports.TurnPatchEvent{ItemID: r.ItemID, Message: ports.TurnPatchMessage{
		ID: message.ID, ConversationID: message.ConversationID, ParentID: message.ParentID,
		TurnID: r.TurnID, Content: message.Content, Reasoning: message.Reasoning,
		PromptTokens: message.PromptTokens, CompletionTokens: message.CompletionTokens,
		TotalTokens: message.TotalTokens, CacheReadTokens: message.CacheReadTokens,
		CacheWriteTokens: message.CacheWriteTokens, CacheMissTokens: message.CacheMissTokens,
		Model: message.Model, CreatedAt: message.CreatedAt.Format(time.RFC3339Nano), Timestamp: message.CreatedAt.UnixMilli(),
	}}
	if r.callCount == 0 && r.messageCount <= 1 {
		return patch
	}
	for _, segment := range r.Segments {
		target := ports.TurnPatchSegment{}
		switch segment.Kind {
		case "text":
			target.Type, target.Content = "text", segment.Text
		case "tool_invocations":
			target.Type = "tool_calls"
		default:
			continue
		}
		for _, call := range segment.Invocations {
			target.ToolInvocations = append(target.ToolInvocations, ports.TurnPatchToolInvocation{
				InvocationID: call.InvocationID, CallID: call.ID, Name: call.Name, Origin: call.Origin,
				ServerLabel: call.ServerLabel, Status: call.Status, Iteration: call.Iteration, DurationMs: call.DurationMs,
				InputPreview: call.InputPreview, OutputPreview: call.OutputPreview, InputBytes: call.InputBytes, OutputBytes: call.OutputBytes,
				HasDetails: call.HasDetails, ResultAvailability: call.ResultAvailability,
				HasSearchResults: call.HasSearchResults, SearchResultCount: call.SearchResultCount, SecurityOutcome: call.SecurityOutcome,
			})
		}
		patch.Message.TurnSegments = append(patch.Message.TurnSegments, target)
	}
	return patch
}

// ThreadParentIDs retorna os escopos com filhos já contados pela leitura
// transacional, inclusive fontes intermediárias de uma resposta agrupada.
func (r *TimelineItem) ThreadParentIDs() []string {
	var ids []string
	seen := map[string]bool{}
	add := func(message *Message) {
		if message != nil && message.Presentation != nil && message.Presentation.ChildCount > 0 && !seen[message.ID] {
			ids = append(ids, message.ID)
			seen[message.ID] = true
		}
	}
	for i := range r.messages {
		add(&r.messages[i])
	}
	add(r.Representative)
	return ids
}
