package chat

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// composeResponse substitui fontes pelo vínculo explícito do writer. Não
// compara prefixos/textos nem mistura tentativas retidas com a execução atual.
// Confirmação troca a fonte dos bytes, conservando identidade e cortes publicados.
func composeResponse(persisted *TimelineItem, live *LiveTurnSnapshot) (*TimelineItem, error) {
	return composeResponseWithSources(persisted, live, nil)
}

func composeResponseWithSources(persisted *TimelineItem, live *LiveTurnSnapshot, confirmedSources map[string][]ResponseSegment) (*TimelineItem, error) {
	if live.Detached || persisted.ItemID != live.ItemID || persisted.TurnID != live.TurnID {
		return nil, ErrProjectionRecoveryRequired
	}
	result := *persisted
	result.ExecutionID = live.ExecutionID
	result.RecoveryReference = nil
	result.State = live.State
	result.Thinking = live.Thinking
	result.TerminalReason = live.Reason
	if !live.Finished || live.Volatile {
		result.RecoveryReference = &TimelineRecoveryReference{ProjectionID: live.ProjectionID, Incarnation: live.Incarnation, TurnID: live.TurnID, ExecutionID: live.ExecutionID}
	}
	if live.Volatile {
		result.Durability = "volatile"
	}
	if len(live.Streams) == 0 {
		return &result, nil
	}
	byID := make(map[string]Message, len(persisted.messages))
	for _, message := range persisted.messages {
		byID[message.ID] = message
	}
	replaced := make(map[string]bool)
	streamMessages := make([]Message, 0, len(live.Streams))
	iterationIDs := make(map[int]string, len(live.Streams))
	liveSources := make(map[string][]ResponseSegment, len(confirmedSources))
	for id, segments := range confirmedSources {
		liveSources[id] = segments
	}
	for index, stream := range live.Streams {
		if stream.Confirmed {
			message, ok := byID[stream.PersistedMessageID]
			if !ok {
				return nil, fmt.Errorf("%w: fonte confirmada ausente", ErrProjectionRecoveryRequired)
			}
			streamMessages = append(streamMessages, message)
			if source, valid := confirmedPresentationSource(message, stream); valid {
				liveSources[message.ID], _ = source.segments(message)
			}
			iterationIDs[stream.Iteration] = message.ID
			replaced[message.ID] = true
			continue
		}
		var text, reasoning strings.Builder
		for _, segment := range stream.Segments {
			if segment.Kind == "text" {
				text.WriteString(segment.Text)
			}
			if segment.Kind == "reasoning" {
				reasoning.WriteString(segment.Text)
			}
		}
		// Até uma fonte vazia substitui o prefixo antigo após reset de transporte.
		id := fmt.Sprintf("draft:%s:%d:%d", base64.RawURLEncoding.EncodeToString([]byte(live.ExecutionID)), stream.Iteration, stream.Generation)
		liveSources[id] = append([]ResponseSegment(nil), stream.Segments...)
		for i := range liveSources[id] {
			liveSources[id][i].SourceMessageID = id
		}
		message := Message{Role: "assistant", Content: text.String(), Reasoning: reasoning.String()}
		message.ID, message.ConversationID, message.TurnID = id, persisted.ConversationID, &live.TurnID
		streamMessages = append(streamMessages, message)
		iterationIDs[stream.Iteration] = id
		if stream.PersistedMessageID != "" {
			replaced[stream.PersistedMessageID] = true
		}
		if index == len(live.Streams)-1 {
			replaced[live.ResponseID] = true
		}
		result.Durability = "volatile"
	}
	// A resposta inicial é apenas o destino final; após confirmar uma
	// intermediária, seu prefixo já pertence àquela fonte e não ao destino vazio.
	replaced[live.ResponseID] = true
	ordered := make([]Message, 0, len(persisted.messages)+len(streamMessages))
	for _, message := range persisted.messages {
		if !replaced[message.ID] {
			ordered = append(ordered, message)
		}
	}
	ordered = append(ordered, streamMessages...)
	calls := append([]TurnSegmentToolCall(nil), persisted.invocations...)
	for i := range calls {
		call := &calls[i]
		if call.ExecutionID != live.ExecutionID {
			continue
		}
		if id, ok := iterationIDs[call.Iteration]; ok {
			call.AssistantMessageID = id
		}
	}
	var representative *Message
	if len(streamMessages) != 0 {
		representative = &streamMessages[len(streamMessages)-1]
	}
	result.Segments = projectResponseSegmentsWithSources(ordered, calls, representative, liveSources)
	return &result, nil
}
