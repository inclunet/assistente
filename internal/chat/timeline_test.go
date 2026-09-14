package chat

import (
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/database"
)

func canonicalTurnFixture() (string, []database.ChatMessage, []TurnSegmentToolCall) {
	turnID := "turn-1"
	base := time.Date(2026, 9, 14, 8, 0, 0, 0, time.UTC)
	messages := []database.ChatMessage{
		{UUIDModel: database.UUIDModel{ID: "assistant-1", CreatedAt: base}, ConversationID: "conv-1", TurnID: &turnID, Role: "assistant", Content: "vou consultar"},
		{UUIDModel: database.UUIDModel{ID: "assistant-2", CreatedAt: base.Add(time.Second)}, ConversationID: "conv-1", TurnID: &turnID, Role: "assistant", Content: "resposta final"},
	}
	calls := []TurnSegmentToolCall{{
		InvocationID:       "inv-1",
		ID:                 "call-1",
		Name:               "read_file",
		Status:             "succeeded",
		Iteration:          1,
		HasDetails:         true,
		ResultAvailability: "available",
		AssistantMessageID: "assistant-1",
	}}
	return turnID, messages, calls
}

func TestConsolidateTimelineTurnUsaSomenteLedgerCanonico(t *testing.T) {
	_, messages, calls := canonicalTurnFixture()
	result := ConsolidateTimelineTurn(messages, calls)
	if result.Message.ID != "assistant-2" || result.Message.Content != "resposta final" {
		t.Fatalf("representante incorreto: %+v", result.Message)
	}
	if len(result.Segments) != 3 {
		t.Fatalf("esperava texto/tool/texto: %+v", result.Segments)
	}
	tool := result.Segments[1].ToolCalls[0]
	if tool.InvocationID != "inv-1" || tool.Name != "read_file" || !tool.HasDetails {
		t.Fatalf("resumo canônico incorreto: %+v", tool)
	}
}

func TestConsolidateTimelineTurnNaoSerializaMetadadoInterno(t *testing.T) {
	_, messages, calls := canonicalTurnFixture()
	payload, err := json.Marshal(ConsolidateTimelineTurn(messages, calls).Segments)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if containsAny(text, "assistant-1", "arguments", "result\":") {
		t.Fatalf("timeline expôs payload ou metadado interno: %s", text)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		for index := 0; index+len(candidate) <= len(value); index++ {
			if value[index:index+len(candidate)] == candidate {
				return true
			}
		}
	}
	return false
}

func TestConsolidateTimelineTurnAgrupaInvocacoesSemAssistantPorIteracao(t *testing.T) {
	_, messages, _ := canonicalTurnFixture()
	calls := []TurnSegmentToolCall{
		{InvocationID: "inv-2", ID: "b", Name: "segunda", Iteration: 2},
		{InvocationID: "inv-1", ID: "a", Name: "primeira", Iteration: 1},
	}
	result := ConsolidateTimelineTurn(messages, calls)
	if len(result.Segments) != 4 {
		t.Fatalf("segmentos por iteração ausentes: %+v", result.Segments)
	}
	if result.Segments[2].ToolCalls[0].ID != "a" || result.Segments[3].ToolCalls[0].ID != "b" {
		t.Fatalf("ordem de iteração incorreta: %+v", result.Segments)
	}
}

func TestConsolidateTimelineTurnNormalizaResumoIncompleto(t *testing.T) {
	_, messages, _ := canonicalTurnFixture()
	result := ConsolidateTimelineTurn(messages, []TurnSegmentToolCall{{ID: "call-1"}})
	call := result.Segments[len(result.Segments)-1].ToolCalls[0]
	if call.Name != "tool" || call.Status != "succeeded" || call.ResultAvailability != "missing" {
		t.Fatalf("normalização incorreta: %+v", call)
	}
}

func TestConsolidateTimelineTurnTrivialNaoCriaSegmentos(t *testing.T) {
	message := database.ChatMessage{UUIDModel: database.UUIDModel{ID: "assistant"}, Role: "assistant", Content: "ok"}
	result := ConsolidateTimelineTurn([]database.ChatMessage{message}, nil)
	if len(result.Segments) != 0 || result.Message.ID != "assistant" {
		t.Fatalf("turno trivial alterado: %+v", result)
	}
}

func TestMessageTimelineItemKeyMantemUsuarioSeparado(t *testing.T) {
	turnID := "turn-1"
	user := database.ChatMessage{UUIDModel: database.UUIDModel{ID: "user-1"}, TurnID: &turnID, Role: "user"}
	assistant := database.ChatMessage{UUIDModel: database.UUIDModel{ID: "assistant-1"}, TurnID: &turnID, Role: "assistant"}
	if MessageTimelineItemKey(user) != "message:user-1" {
		t.Fatalf("usuário não permaneceu separado")
	}
	if MessageTimelineItemKey(assistant) != "turn:turn-1" {
		t.Fatalf("assistant não foi agrupado")
	}
}

func TestBuildNodesWithTimelineConsolidationAnexaSegmentos(t *testing.T) {
	turnID, messages, calls := canonicalTurnFixture()
	nodes := BuildNodesWithTimelineConsolidation(messages, nil, map[string]int{}, map[string][]TurnSegmentToolCall{turnID: calls})
	if len(nodes) != 1 || len(nodes[0].Message.TurnSegments) != 3 {
		t.Fatalf("nó consolidado incorreto: %+v", nodes)
	}
}

func TestBuildTimelineMessageNodesPreservaIndiceOriginal(t *testing.T) {
	turnID, messages, calls := canonicalTurnFixture()
	items := []database.MessageWindowItem{{
		Kind:          database.MessageWindowItemKindTurn,
		TurnID:        turnID,
		MessageID:     "assistant-1",
		OriginalIndex: 7,
	}}
	nodes := BuildTimelineMessageNodes(items, messages, nil, map[string]int{}, map[string][]TurnSegmentToolCall{turnID: calls})
	if len(nodes) != 1 || nodes[0].OriginalIndex == nil || *nodes[0].OriginalIndex != 7 {
		t.Fatalf("índice original perdido: %+v", nodes)
	}
}

func TestCollectTurnIDsWithToolCallsDeduplicaTurnosConversacionais(t *testing.T) {
	turnID, messages, _ := canonicalTurnFixture()
	ids := CollectTurnIDsWithToolCalls(messages)
	if len(ids) != 1 || ids[0] != turnID {
		t.Fatalf("turn IDs inesperados: %v", ids)
	}
}
