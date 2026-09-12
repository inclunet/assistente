package ports

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStreamEventSerializaSomenteDeltaCorrelacionado(t *testing.T) {
	outputTokens := 7
	reasoningTokens := 5
	responseBytes := 8
	payload, err := json.Marshal(StreamEvent{
		MessageID:            "assistant-1",
		ConversationId:       "conv-1",
		TurnID:               "turn-1",
		Delta:                "Olá 世界",
		Reset:                true,
		BaseContent:          "prefixo",
		Sequence:             0,
		FinishReason:         "max_tokens",
		RawReason:            "length",
		Provider:             "provider-1",
		Model:                "modelo-1",
		EffectiveOutputLimit: 12,
		OutputTokens:         &outputTokens,
		ReasoningTokens:      &reasoningTokens,
		ResponseBytes:        &responseBytes,
	})
	if err != nil {
		t.Fatalf("serializar chat:stream: %v", err)
	}
	got := string(payload)
	for _, fragment := range []string{
		`"messageId":"assistant-1"`,
		`"conversationId":"conv-1"`,
		`"turnId":"turn-1"`,
		`"delta":"Olá 世界"`,
		`"reset":true`,
		`"baseContent":"prefixo"`,
		`"sequence":0`,
		`"finishReason":"max_tokens"`,
		`"rawReason":"length"`,
		`"provider":"provider-1"`,
		`"model":"modelo-1"`,
		`"effectiveOutputLimit":12`,
		`"outputTokens":7`,
		`"reasoningTokens":5`,
		`"responseBytes":8`,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("payload não contém %s: %s", fragment, got)
		}
	}
	if strings.Contains(got, `"content"`) || strings.Contains(got, `"fullResponse"`) {
		t.Fatalf("payload repetiu conteúdo acumulado: %s", got)
	}
}

func TestDoneEventSerializaPatchMinimoDoTurno(t *testing.T) {
	parentID := "thread-root"
	outputTokens := 7
	reasoningTokens := 5
	responseBytes := 8
	payload, err := json.Marshal(DoneEvent{
		ConversationID:       "conv-1",
		TurnID:               "turn-1",
		Reason:               "output_limit",
		FinishReason:         "max_tokens",
		RawReason:            "length",
		Provider:             "provider-1",
		Model:                "modelo-1",
		EffectiveOutputLimit: 12,
		OutputTokens:         &outputTokens,
		ReasoningTokens:      &reasoningTokens,
		ResponseBytes:        &responseBytes,
		TurnPatch: &TurnPatchEvent{Message: TurnPatchMessage{
			ID:             "assistant-1",
			ConversationID: "conv-1",
			ParentID:       &parentID,
			TurnID:         "turn-1",
			Content:        "parcial",
			CreatedAt:      "2026-09-08T20:00:00Z",
			TurnSegments: []TurnPatchSegment{{
				Type: "tool_calls",
				ToolCalls: []TurnPatchToolCall{{
					ID:       "call-1",
					Type:     "function",
					Function: TurnPatchToolFunction{Name: "update_plan", Arguments: "{}"},
					Result:   `{"updated":true}`,
				}},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("serializar chat:done: %v", err)
	}
	got := string(payload)
	for _, fragment := range []string{
		`"conversationId":"conv-1"`,
		`"turnId":"turn-1"`,
		`"parentId":"thread-root"`,
		`"reason":"output_limit"`,
		`"finishReason":"max_tokens"`,
		`"rawReason":"length"`,
		`"provider":"provider-1"`,
		`"effectiveOutputLimit":12`,
		`"outputTokens":7`,
		`"reasoningTokens":5`,
		`"responseBytes":8`,
		`"turnPatch":{"message":`,
		`"turnSegments":[`,
		`"name":"update_plan"`,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("payload não contém %s: %s", fragment, got)
		}
	}
}
