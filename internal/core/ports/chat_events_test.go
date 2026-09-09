package ports

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDoneEventSerializaPatchMinimoDoTurno(t *testing.T) {
	parentID := "thread-root"
	payload, err := json.Marshal(DoneEvent{
		ConversationID: "conv-1",
		TurnID:         "turn-1",
		Reason:         "output_limit",
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
		`"turnPatch":{"message":`,
		`"turnSegments":[`,
		`"name":"update_plan"`,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("payload não contém %s: %s", fragment, got)
		}
	}
}
