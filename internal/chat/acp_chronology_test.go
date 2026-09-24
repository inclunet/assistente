package chat

import (
	"assistente/internal/database"
	"testing"
)

func TestACPTextPositionsValidaLimitesEUTF8(t *testing.T) {
	messages := []Message{{UUIDModel: database.UUIDModel{ID: "assistant"}, Role: "assistant", Content: "ação"}}
	for _, offset := range []int{-1, 2, 4, 99} {
		calls := []TurnSegmentToolCall{{ID: "tool", Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &offset}}
		if _, ok := consolidateACPTextPositions(messages, calls); ok {
			t.Fatalf("offset inválido aceito: %d", offset)
		}
	}
	zero, end := 0, len(messages[0].Content)
	calls := []TurnSegmentToolCall{
		{ID: "A", Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &zero},
		{ID: "B", Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &zero},
		{ID: "C", Origin: "acp_agent", AssistantMessageID: "assistant", ACPTextOffset: &end},
	}
	segments, ok := consolidateACPTextPositions(messages, calls)
	if !ok || len(segments) != 3 || len(segments[0].ToolCalls) != 2 || segments[0].ToolCalls[0].ID != "A" || segments[0].ToolCalls[1].ID != "B" || segments[1].Content != "ação" || segments[2].ToolCalls[0].ID != "C" {
		t.Fatalf("segmentos: %+v", segments)
	}
	calls[0].ACPTextOffset = nil
	if _, ok := consolidateACPTextPositions(messages, calls); ok {
		t.Fatal("legado sem posição não pode inventar ordem")
	}
}
