package chat

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"assistente/internal/core/ports"
	"assistente/internal/database"
)

// PersistedResponse é a projeção interna de conteúdo, não o envelope wire v1.
// Epoch/revision pertencem ao futuro projetor; versão do banco não os substitui.
type PersistedResponse struct {
	ItemID         string
	TurnID         string
	Representative *Message
	State          string
	Origin         string
	Segments       []ResponseSegment
	messageCount   int
	callCount      int
}

type ResponseSegment struct {
	SegmentID   string
	Kind        string
	Text        string
	Invocations []TurnSegmentToolCall
}

func responseTextSegment(kind string, message Message, start int, text string) ResponseSegment {
	return ResponseSegment{SegmentID: kind + ":" + base64.RawURLEncoding.EncodeToString([]byte(message.ID)) + ":" + strconv.Itoa(start), Kind: kind, Text: text}
}

func responseToolSegment(calls []TurnSegmentToolCall) ResponseSegment {
	// O primeiro membro identifica o grupo. Crescer o grupo preserva a identidade.
	id := calls[0].InvocationID
	if id == "" {
		// Somente compatibilidade com fixtures/leitores legados, fora da projeção
		// persistida estrita, que exige a identidade do ledger.
		id = fmt.Sprintf("legacy:%d:%s", calls[0].Iteration, calls[0].ID)
	}
	return ResponseSegment{SegmentID: "tools:" + base64.RawURLEncoding.EncodeToString([]byte(id)), Kind: "tool_invocations", Invocations: calls}
}

// ProjectPersistedResponse valida vínculos e projeta o conteúdo confirmado do turno.
func ProjectPersistedResponse(conversationID, turnID string, parentID *string, messages []Message, calls []TurnSegmentToolCall, turn *database.ChatTurn) (*PersistedResponse, error) {
	result := &PersistedResponse{ItemID: ports.ResponseItemID(turnID), TurnID: turnID, State: database.TurnHistoricalUnknown, Origin: "legacy"}
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
	result.Segments = projectResponseSegments(ordered, calls, result.Representative)
	result.messageCount, result.callCount = len(messages), len(calls)
	return result, nil
}

func sameScope(a, b *string) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// O contrato antigo exige Message mesmo quando o representante foi excluído.
// Este fallback visual não altera Representative, State nem os segmentos canônicos.
func (r *PersistedResponse) legacyMessage(messages []Message, calls []TurnSegmentToolCall) (Message, []TurnSegment) {
	var message Message
	if r.Representative != nil {
		message = *r.Representative
	} else if len(messages) > 0 {
		message = legacyTurnRepresentative(orderedTurnMessages(messages), calls)
	}
	return message, legacyResponseSegments(r.Segments, len(messages), len(calls))
}

func legacyResponseSegments(segments []ResponseSegment, messageCount, callCount int) []TurnSegment {
	if callCount == 0 && messageCount <= 1 {
		return nil
	}
	result := make([]TurnSegment, 0, len(segments))
	for _, segment := range segments {
		switch segment.Kind {
		case "text":
			result = append(result, TurnSegment{Type: "text", Content: segment.Text})
		case "tool_invocations":
			result = append(result, TurnSegment{Type: "tool_calls", ToolCalls: segment.Invocations})
		}
	}
	return result
}

// TerminalPatch adapta somente o representante explícito. O fallback de mensagem
// usado pelo visual legado não comprova qual resposta foi confirmada no storage.
func (r *PersistedResponse) TerminalPatch() *ports.TurnPatchEvent {
	if r == nil || r.Representative == nil {
		return nil
	}
	message := EnrichMessage(*r.Representative)
	patch := &ports.TurnPatchEvent{ItemID: r.ItemID, Message: ports.TurnPatchMessage{
		ID: message.ID, ConversationID: message.ConversationID, ParentID: message.ParentID,
		TurnID: r.TurnID, Content: message.Content, Reasoning: message.Reasoning,
		PromptTokens: message.PromptTokens, CompletionTokens: message.CompletionTokens,
		TotalTokens: message.TotalTokens, CacheReadTokens: message.CacheReadTokens,
		CacheWriteTokens: message.CacheWriteTokens, CacheMissTokens: message.CacheMissTokens,
		Model: message.Model, CreatedAt: message.CreatedAt.Format(time.RFC3339Nano), Timestamp: message.Timestamp,
	}}
	for _, segment := range legacyResponseSegments(r.Segments, r.messageCount, r.callCount) {
		target := ports.TurnPatchSegment{Type: segment.Type, Content: segment.Content}
		for _, call := range segment.ToolCalls {
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
