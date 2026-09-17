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
	if result.Segments[1].ToolCalls[0].ID != "a" || result.Segments[2].ToolCalls[0].ID != "b" {
		t.Fatalf("ordem de iteração incorreta: %+v", result.Segments)
	}
}

func TestConsolidateTimelineTurnNormalizaResumoIncompleto(t *testing.T) {
	_, messages, _ := canonicalTurnFixture()
	result := ConsolidateTimelineTurn(messages, []TurnSegmentToolCall{{ID: "call-1"}})
	var call TurnSegmentToolCall
	for _, segment := range result.Segments {
		if segment.Type == "tool_calls" && len(segment.ToolCalls) == 1 {
			call = segment.ToolCalls[0]
			break
		}
	}
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

func TestConsolidateTimelineTurnFinalPreCriadoSemUsageUsaAtualizacaoTerminal(t *testing.T) {
	turnID := "turn-updated-final"
	base := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	final := database.ChatMessage{
		UUIDModel:        database.UUIDModel{ID: "assistant-final", CreatedAt: base, UpdatedAt: base.Add(10 * time.Second)},
		ConversationID:   "conv-1",
		TurnID:           &turnID,
		Role:             "assistant",
		Content:          "resposta final",
		Reasoning:        "raciocínio final",
		PromptTokens:     12,
		CompletionTokens: 0,
		TotalTokens:      0,
		CacheReadTokens:  3,
		CacheWriteTokens: 4,
		CacheMissTokens:  5,
		Model:            "modelo-final",
	}
	intermediate := database.ChatMessage{
		UUIDModel:      database.UUIDModel{ID: "assistant-intermediate", CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(time.Second)},
		ConversationID: "conv-1",
		TurnID:         &turnID,
		Role:           "assistant",
		Content:        "vou consultar",
	}
	result := ConsolidateTimelineTurn([]database.ChatMessage{final, intermediate}, []TurnSegmentToolCall{
		{ID: "call-0", Name: "search", Iteration: 0, AssistantMessageID: intermediate.ID},
		{ID: "call-1", Name: "fetch", Iteration: 1},
	})

	if result.Message.ID != final.ID || result.Message.Content != final.Content || result.Message.TotalTokens != 0 ||
		result.Message.Reasoning != final.Reasoning || result.Message.CacheReadTokens != final.CacheReadTokens ||
		result.Message.CacheWriteTokens != final.CacheWriteTokens || result.Message.CacheMissTokens != final.CacheMissTokens ||
		result.Message.Model != final.Model {
		t.Fatalf("representante final/metadados alterados: got=%+v want=%+v", result.Message, final)
	}
	if len(result.Segments) != 4 || result.Segments[0].Content != "vou consultar" ||
		result.Segments[1].ToolCalls[0].ID != "call-0" || result.Segments[2].ToolCalls[0].ID != "call-1" ||
		result.Segments[3].Content != "resposta final" {
		t.Fatalf("cronologia do final sem usage incorreta: %+v", result.Segments)
	}
}

func TestConsolidateTimelineTurnUsageNaoVenceEscritaFinalMaisRecente(t *testing.T) {
	base := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	final := Message{
		UUIDModel: database.UUIDModel{ID: "final", CreatedAt: base, UpdatedAt: base.Add(10 * time.Second)},
		Role:      "assistant", Content: "conclusão sem usage",
	}
	older := Message{
		UUIDModel: database.UUIDModel{ID: "antigo", CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(2 * time.Second)},
		Role:      "assistant", Content: "texto anterior", TotalTokens: 42,
	}
	result := ConsolidateTimelineTurn([]Message{final, older}, nil)
	if result.Message.ID != final.ID || result.Message.TotalTokens != 0 ||
		len(result.Segments) != 2 || result.Segments[0].Content != older.Content || result.Segments[1].Content != final.Content {
		t.Fatalf("usage antiga substituiu a conclusão: %+v", result)
	}
}

func TestConsolidateTimelineTurnAgrupaIteracoesSilenciosasNumericamenteZeroBased(t *testing.T) {
	turnID := "turn-silent"
	base := time.Date(2026, 9, 17, 13, 0, 0, 0, time.UTC)
	final := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "final-silent", CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "final",
	}
	calls := []TurnSegmentToolCall{
		{ID: "opaque-10", Name: "r10", Iteration: 10},
		{ID: "opaque-2", Name: "r2", Iteration: 2},
		{ID: "opaque-1", Name: "r1", Iteration: 1},
		{ID: "opaque-0", Name: "r0", Iteration: 0},
	}
	result := ConsolidateTimelineTurn([]database.ChatMessage{final}, calls)
	if len(result.Segments) != 5 {
		t.Fatalf("esperava quatro rodadas silenciosas e texto final: %+v", result.Segments)
	}
	for index, want := range []string{"opaque-0", "opaque-1", "opaque-2", "opaque-10"} {
		if got := result.Segments[index].ToolCalls[0].ID; got != want {
			t.Fatalf("ordem numérica zero-based incorreta no segmento %d: got=%q want=%q; segments=%+v", index, got, want, result.Segments)
		}
	}
	if result.Segments[4].Type != "text" || result.Segments[4].Content != "final" {
		t.Fatalf("texto final não ficou após rodadas silenciosas: %+v", result.Segments)
	}
}

func TestConsolidateTimelineTurnIntercalaMCPDoMesmoAnchorPorIteracao(t *testing.T) {
	turnID := "turn-mcp"
	base := time.Date(2026, 9, 17, 14, 0, 0, 0, time.UTC)
	final := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "anchor-final", CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "resposta final",
	}
	first := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "assistant-0", CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(time.Second)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "primeira rodada",
	}
	second := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "assistant-1", CreatedAt: base.Add(2 * time.Second), UpdatedAt: base.Add(2 * time.Second)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "segunda rodada",
	}
	result := ConsolidateTimelineTurn([]database.ChatMessage{second, final, first}, []TurnSegmentToolCall{
		{ID: "native-0", Name: "mcp_zero", Origin: "mcp_native", Iteration: 0, AssistantMessageID: final.ID},
		{ID: "local-0", Name: "local_zero", Iteration: 0, AssistantMessageID: first.ID},
		{ID: "native-1", Name: "mcp_one", Origin: "mcp_native", Iteration: 1, AssistantMessageID: final.ID},
		{ID: "local-1", Name: "local_one", Iteration: 1, AssistantMessageID: second.ID},
	})

	var callIDs []string
	for _, segment := range result.Segments {
		if segment.Type == "tool_calls" {
			for _, call := range segment.ToolCalls {
				callIDs = append(callIDs, call.ID)
			}
		}
	}
	wantIDs := []string{"native-0", "local-0", "native-1", "local-1"}
	if len(callIDs) != len(wantIDs) {
		t.Fatalf("grupos MCP/local fundidos ou perdidos: got=%v segments=%+v", callIDs, result.Segments)
	}
	for index, want := range wantIDs {
		if callIDs[index] != want {
			t.Fatalf("ordem de blocos por iteração incorreta: got=%v want=%v", callIDs, wantIDs)
		}
	}
	if result.Segments[len(result.Segments)-1].Type != "text" || result.Segments[len(result.Segments)-1].Content != final.Content {
		t.Fatalf("texto final não ficou depois do MCP ligado à âncora: %+v", result.Segments)
	}
}

func TestConsolidateTimelineTurnPreservaOrdemDeCallsParalelasComIDsOpacos(t *testing.T) {
	turnID := "turn-parallel"
	base := time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC)
	final := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "final-parallel", CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "final",
	}
	result := ConsolidateTimelineTurn([]database.ChatMessage{final}, []TurnSegmentToolCall{
		{ID: "z-call", Name: "primeira", Iteration: 0},
		{ID: "a-call", Name: "segunda", Iteration: 0},
	})
	if len(result.Segments) != 2 || len(result.Segments[0].ToolCalls) != 2 {
		t.Fatalf("esperava um grupo paralelo antes do final: %+v", result.Segments)
	}
	if got := []string{result.Segments[0].ToolCalls[0].ID, result.Segments[0].ToolCalls[1].ID}; got[0] != "z-call" || got[1] != "a-call" {
		t.Fatalf("ordem de entrada das calls paralelas foi alterada: %v", got)
	}
}

func TestConsolidateTimelineTurnFallbackLegadoEscolheFinalCriadoPorUltimo(t *testing.T) {
	turnID := "turn-legacy-final"
	base := time.Date(2026, 9, 17, 16, 0, 0, 0, time.UTC)
	intermediate := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "legacy-intermediate", CreatedAt: base},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "texto intermediário",
	}
	final := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "legacy-final", CreatedAt: base.Add(time.Second)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "texto final",
	}
	result := ConsolidateTimelineTurn([]database.ChatMessage{final, intermediate}, []TurnSegmentToolCall{{
		ID: "legacy-call", Name: "search", Iteration: 0, AssistantMessageID: intermediate.ID,
	}})
	if result.Message.ID != final.ID || len(result.Segments) != 3 ||
		result.Segments[0].Content != intermediate.Content || result.Segments[2].Content != final.Content {
		t.Fatalf("fallback legado não preservou final criado por último: message=%+v segments=%+v", result.Message, result.Segments)
	}
}

func TestConsolidateTimelineTurnNaoPromoveIntermediarioComEdicaoTardia(t *testing.T) {
	turnID := "turn-late-edit"
	base := time.Date(2026, 9, 17, 17, 0, 0, 0, time.UTC)
	final := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "final-anchor", CreatedAt: base, UpdatedAt: base.Add(10 * time.Second)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "resposta final",
		Model:     "modelo-final",
	}
	intermediate := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "intermediate-late", CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(20 * time.Second)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "texto intermediário editado depois",
	}
	result := ConsolidateTimelineTurn([]database.ChatMessage{intermediate, final}, []TurnSegmentToolCall{{
		ID:                 "local-call",
		Name:               "search",
		Iteration:          0,
		AssistantMessageID: intermediate.ID,
	}})

	if result.Message.ID != final.ID || result.Message.Content != final.Content || result.Message.Model != final.Model {
		t.Fatalf("edição tardia promoveu intermediário ao representante: message=%+v", result.Message)
	}
	if len(result.Segments) != 3 || result.Segments[0].Content != intermediate.Content ||
		result.Segments[1].Type != "tool_calls" || result.Segments[1].ToolCalls[0].ID != "local-call" ||
		result.Segments[2].Content != final.Content {
		t.Fatalf("segmentos após edição tardia incorretos: %+v", result.Segments)
	}
}

func TestConsolidateTimelineTurnSemAssistantFinalMantemTextoAntesDasTools(t *testing.T) {
	turnID := "turn-no-final-assistant"
	message := database.ChatMessage{
		UUIDModel: database.UUIDModel{
			ID:        "assistant-intermediate-only",
			CreatedAt: time.Date(2026, 9, 17, 18, 0, 0, 0, time.UTC),
		},
		TurnID:  &turnID,
		Role:    "assistant",
		Content: "texto intermediário",
	}
	result := ConsolidateTimelineTurn([]database.ChatMessage{message}, []TurnSegmentToolCall{{
		ID:                 "tool-after-text",
		Name:               "fetch",
		Iteration:          0,
		AssistantMessageID: message.ID,
	}})

	if result.Message.ID != message.ID || len(result.Segments) != 2 {
		t.Fatalf("representante ou quantidade de segmentos incorretos: message=%+v segments=%+v", result.Message, result.Segments)
	}
	if result.Segments[0].Type != "text" || result.Segments[0].Content != message.Content ||
		result.Segments[1].Type != "tool_calls" || result.Segments[1].ToolCalls[0].ID != "tool-after-text" {
		t.Fatalf("texto foi promovido para depois das tools: %+v", result.Segments)
	}
}
