package chat

import (
	"assistente/internal/database"
	"fmt"
	"strings"
	"testing"
)

// Exercício de offsets ACP reservado ao CI; não executar localmente.
func TestLiveBudgetCountsToolGroupsActivatedByACPText(t *testing.T) {
	calls := make([]TurnSegmentToolCall, 20)
	for i := range calls {
		offset := i + 2
		calls[i] = TurnSegmentToolCall{InvocationID: fmt.Sprint(i) + strings.Repeat("i", 256*1024), Origin: "acp_agent", ExecutionID: "execution", AssistantMessageID: "response", ACPTextOffset: &offset}
	}
	item := compositionItem(t, []Message{{UUIDModel: database.UUIDModel{ID: "response"}, Content: "x"}}, calls)
	c := &projectionConversation{turns: map[string]*liveTurn{}}
	if !windowCompositionWithinBudget(c, []*TimelineItem{item}) {
		t.Fatal("base deveria caber")
	}
	part := &livePart{}
	part.text.WriteString(strings.Repeat("x", 100))
	c.turns["question"] = &liveTurn{turnID: "question", executionID: "execution", responseID: "response", streams: map[int]*liveStream{0: {parts: []*livePart{part}}}}
	live := &LiveTurnSnapshot{ItemID: item.ItemID, TurnID: "question", ExecutionID: "execution", ResponseID: "response", Streams: []LiveStreamSnapshot{{Segments: []ResponseSegment{{Kind: "text", SegmentID: "live", Text: strings.Repeat("x", 100)}}}}}
	composed, err := composeResponse(item, live)
	if err != nil {
		t.Fatal(err)
	}
	groups := 0
	for _, segment := range composed.Segments {
		if segment.Kind == "tool_invocations" {
			groups++
		}
	}
	if groups != 20 {
		t.Fatal("offsets live não exercitados", groups)
	}
	if windowCompositionWithinBudget(c, []*TimelineItem{item}) {
		t.Fatal("amplificação de IDs não recusada")
	}
}
