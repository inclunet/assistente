package chat

import (
	"assistente/internal/database"
	"testing"
)

func TestACPTextPositionsValidaLimitesEUTF8(t *testing.T) {
	messages := []Message{{UUIDModel: database.UUIDModel{ID: "assistant"}, Role: "assistant", Content: "ação"}}
	for _, offset := range []int{-1, 2, 4, 99} {
		calls := []TurnSegmentToolCall{{ID: "tool", Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &offset}}
		if positioned, remaining := positionedInvocations(messages, calls); len(positioned) != 0 || len(remaining) != 1 {
			t.Fatalf("offset inválido aceito: %d", offset)
		}
	}
	zero, end := 0, len(messages[0].Content)
	calls := []TurnSegmentToolCall{
		{ID: "A", Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &zero},
		{ID: "B", Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &zero},
		{ID: "C", Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &end},
	}
	positioned, remaining := positionedInvocations(messages, calls)
	segments := positionSourceInvocations(messageSegments(messages[0]), positioned["assistant"])
	if len(remaining) != 0 || len(segments) != 3 || len(segments[0].Invocations) != 2 || segments[0].Invocations[0].ID != "A" || segments[0].Invocations[1].ID != "B" || segments[1].Text != "ação" || segments[2].Invocations[0].ID != "C" {
		t.Fatalf("segmentos: %+v", segments)
	}
	calls[0].ACPTextOffset = nil
	if positioned, remaining := positionedInvocations(messages, calls); len(positioned["assistant"]) != 2 || len(remaining) != 1 || remaining[0].ID != "A" {
		t.Fatal("legado sem posição não pode inventar ordem")
	}
}

func TestMixedInvocationOriginsKeepKnownTextPositions(t *testing.T) {
	offset := len("antes ")
	messages := []Message{{UUIDModel: database.UUIDModel{ID: "middle"}, Role: "assistant", Content: "intermediária"}, {UUIDModel: database.UUIDModel{ID: "final"}, Role: "assistant", Content: "antes depois"}}
	calls := []TurnSegmentToolCall{
		{InvocationID: "local", ID: "local", Origin: "builtin", Iteration: 0, AssistantMessageID: "middle"},
		{InvocationID: "native", ID: "native", Origin: "mcp_native", Iteration: 1, AssistantMessageID: "final"},
		{InvocationID: "activity", ID: "activity", Origin: "acp_agent", Iteration: 1, AssistantMessageID: "final", ACPTextOffset: &offset},
	}
	segments := projectResponseSegments(messages, calls, &messages[1])
	if len(segments) != 6 || segments[0].Text != "intermediária" || segments[1].Invocations[0].InvocationID != "local" || segments[2].Invocations[0].InvocationID != "native" || segments[3].Text != "antes " || segments[4].Invocations[0].InvocationID != "activity" || segments[5].Text != "depois" {
		t.Fatalf("ordem mista perdida: %+v", segments)
	}
}
