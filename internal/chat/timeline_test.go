package chat

import (
	"encoding/json"
	"strings"
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
		HasSearchResults:   true,
		SearchResultCount:  2,
		SecurityOutcome:    "approved",
		AssistantMessageID: "assistant-1",
	}}
	return turnID, messages, calls
}

func TestHistoricalResponseUsaSomenteLedgerCanonico(t *testing.T) {
	_, messages, calls := canonicalTurnFixture()
	result := historicalResponse(t, messages, calls)
	if result.Representative.ID != "assistant-2" || result.Representative.Content != "resposta final" {
		t.Fatalf("representante incorreto: %+v", result.Representative)
	}
	if len(result.Segments) != 3 {
		t.Fatalf("esperava texto/tool/texto: %+v", result.Segments)
	}
	tool := result.Segments[1].Invocations[0]
	if tool.InvocationID != "inv-1" || tool.Name != "read_file" || !tool.HasDetails || !tool.HasSearchResults || tool.SearchResultCount != 2 || tool.SecurityOutcome != "approved" {
		t.Fatalf("resumo canônico incorreto: %+v", tool)
	}
}

func TestHistoricalResponseNaoSerializaMetadadoInterno(t *testing.T) {
	_, messages, calls := canonicalTurnFixture()
	payload, err := json.Marshal(canonicalItem(historicalResponse(t, messages, calls)).Segments)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if strings.Contains(text, "assistantMessageId") || strings.Contains(text, "arguments") || strings.Contains(text, "result\":") {
		t.Fatalf("timeline expôs payload ou metadado interno: %s", text)
	}
}

func TestHistoricalResponseAgrupaInvocacoesSemAssistantPorIteracao(t *testing.T) {
	_, messages, _ := canonicalTurnFixture()
	calls := []TurnSegmentToolCall{
		{InvocationID: "inv-2", ID: "b", Name: "segunda", Iteration: 2},
		{InvocationID: "inv-1", ID: "a", Name: "primeira", Iteration: 1},
	}
	result := historicalResponse(t, messages, calls)
	if len(result.Segments) != 4 {
		t.Fatalf("segmentos por iteração ausentes: %+v", result.Segments)
	}
	if result.Segments[1].Invocations[0].ID != "a" || result.Segments[2].Invocations[0].ID != "b" {
		t.Fatalf("ordem de iteração incorreta: %+v", result.Segments)
	}
}

func TestHistoricalResponseNormalizaResumoIncompleto(t *testing.T) {
	_, messages, _ := canonicalTurnFixture()
	result := historicalResponse(t, messages, []TurnSegmentToolCall{{ID: "call-1"}})
	var call TurnSegmentToolCall
	for _, segment := range result.Segments {
		if segment.Kind == "tool_invocations" && len(segment.Invocations) == 1 {
			call = segment.Invocations[0]
			break
		}
	}
	if call.Name != "tool" || call.Status != "succeeded" || call.ResultAvailability != "missing" {
		t.Fatalf("normalização incorreta: %+v", call)
	}
}

func TestHistoricalResponseFinalPreCriadoSemUsageUsaAtualizacaoTerminal(t *testing.T) {
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
	result := historicalResponse(t, []database.ChatMessage{final, intermediate}, []TurnSegmentToolCall{
		{ID: "call-0", Name: "search", Iteration: 0, AssistantMessageID: intermediate.ID},
		{ID: "call-1", Name: "fetch", Iteration: 1},
	})

	if result.Representative.ID != final.ID || result.Representative.Content != final.Content || result.Representative.TotalTokens != 0 ||
		result.Representative.Reasoning != final.Reasoning || result.Representative.CacheReadTokens != final.CacheReadTokens ||
		result.Representative.CacheWriteTokens != final.CacheWriteTokens || result.Representative.CacheMissTokens != final.CacheMissTokens ||
		result.Representative.Model != final.Model {
		t.Fatalf("representante final/metadados alterados: got=%+v want=%+v", result.Representative, final)
	}
	if len(result.Segments) != 5 || result.Segments[0].Text != "vou consultar" ||
		result.Segments[1].Invocations[0].ID != "call-0" || result.Segments[2].Invocations[0].ID != "call-1" ||
		result.Segments[3].Kind != "reasoning" || result.Segments[3].Text != final.Reasoning ||
		result.Segments[4].Text != "resposta final" {
		t.Fatalf("cronologia do final sem usage incorreta: %+v", result.Segments)
	}
}

func TestHistoricalResponseUsageNaoVenceEscritaFinalMaisRecente(t *testing.T) {
	base := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)
	final := Message{
		UUIDModel: database.UUIDModel{ID: "final", CreatedAt: base, UpdatedAt: base.Add(10 * time.Second)},
		Role:      "assistant", Content: "conclusão sem usage",
	}
	older := Message{
		UUIDModel: database.UUIDModel{ID: "antigo", CreatedAt: base.Add(time.Second), UpdatedAt: base.Add(2 * time.Second)},
		Role:      "assistant", Content: "texto anterior", TotalTokens: 42,
	}
	result := historicalResponse(t, []Message{final, older}, nil)
	if result.Representative.ID != final.ID || result.Representative.TotalTokens != 0 ||
		len(result.Segments) != 2 || result.Segments[0].Text != older.Content || result.Segments[1].Text != final.Content {
		t.Fatalf("usage antiga substituiu a conclusão: %+v", result)
	}
}

func TestHistoricalResponseAgrupaIteracoesSilenciosasNumericamenteZeroBased(t *testing.T) {
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
	result := historicalResponse(t, []database.ChatMessage{final}, calls)
	if len(result.Segments) != 5 {
		t.Fatalf("esperava quatro rodadas silenciosas e texto final: %+v", result.Segments)
	}
	for index, want := range []string{"opaque-0", "opaque-1", "opaque-2", "opaque-10"} {
		if got := result.Segments[index].Invocations[0].ID; got != want {
			t.Fatalf("ordem numérica zero-based incorreta no segmento %d: got=%q want=%q; segments=%+v", index, got, want, result.Segments)
		}
	}
	if result.Segments[4].Kind != "text" || result.Segments[4].Text != "final" {
		t.Fatalf("texto final não ficou após rodadas silenciosas: %+v", result.Segments)
	}
}

func TestHistoricalResponseIntercalaMCPDoMesmoAnchorPorIteracao(t *testing.T) {
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
	result := historicalResponse(t, []database.ChatMessage{second, final, first}, []TurnSegmentToolCall{
		{ID: "native-0", Name: "mcp_zero", Origin: "mcp_native", Iteration: 0, AssistantMessageID: final.ID},
		{ID: "local-0", Name: "local_zero", Iteration: 0, AssistantMessageID: first.ID},
		{ID: "native-1", Name: "mcp_one", Origin: "mcp_native", Iteration: 1, AssistantMessageID: final.ID},
		{ID: "local-1", Name: "local_one", Iteration: 1, AssistantMessageID: second.ID},
	})

	var callIDs []string
	for _, segment := range result.Segments {
		if segment.Kind == "tool_invocations" {
			for _, call := range segment.Invocations {
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
	if result.Segments[len(result.Segments)-1].Kind != "text" || result.Segments[len(result.Segments)-1].Text != final.Content {
		t.Fatalf("texto final não ficou depois do MCP ligado à âncora: %+v", result.Segments)
	}
}

func TestHistoricalResponsePreservaOrdemDeCallsParalelasComIDsOpacos(t *testing.T) {
	turnID := "turn-parallel"
	base := time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC)
	final := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "final-parallel", CreatedAt: base, UpdatedAt: base.Add(time.Minute)},
		TurnID:    &turnID,
		Role:      "assistant",
		Content:   "final",
	}
	result := historicalResponse(t, []database.ChatMessage{final}, []TurnSegmentToolCall{
		{ID: "z-call", Name: "primeira", Iteration: 0},
		{ID: "a-call", Name: "segunda", Iteration: 0},
	})
	if len(result.Segments) != 2 || len(result.Segments[0].Invocations) != 2 {
		t.Fatalf("esperava um grupo paralelo antes do final: %+v", result.Segments)
	}
	if got := []string{result.Segments[0].Invocations[0].ID, result.Segments[0].Invocations[1].ID}; got[0] != "z-call" || got[1] != "a-call" {
		t.Fatalf("ordem de entrada das calls paralelas foi alterada: %v", got)
	}
}

func TestHistoricalResponseFallbackLegadoEscolheFinalCriadoPorUltimo(t *testing.T) {
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
	result := historicalResponse(t, []database.ChatMessage{final, intermediate}, []TurnSegmentToolCall{{
		ID: "legacy-call", Name: "search", Iteration: 0, AssistantMessageID: intermediate.ID,
	}})
	if result.Representative.ID != final.ID || len(result.Segments) != 3 ||
		result.Segments[0].Text != intermediate.Content || result.Segments[2].Text != final.Content {
		t.Fatalf("fallback legado não preservou final criado por último: message=%+v segments=%+v", result.Representative, result.Segments)
	}
}

func TestHistoricalResponseNaoPromoveIntermediarioComEdicaoTardia(t *testing.T) {
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
	result := historicalResponse(t, []database.ChatMessage{intermediate, final}, []TurnSegmentToolCall{{
		ID:                 "local-call",
		Name:               "search",
		Iteration:          0,
		AssistantMessageID: intermediate.ID,
	}})

	if result.Representative.ID != final.ID || result.Representative.Content != final.Content || result.Representative.Model != final.Model {
		t.Fatalf("edição tardia promoveu intermediário ao representante: message=%+v", result.Representative)
	}
	if len(result.Segments) != 3 || result.Segments[0].Text != intermediate.Content ||
		result.Segments[1].Kind != "tool_invocations" || result.Segments[1].Invocations[0].ID != "local-call" ||
		result.Segments[2].Text != final.Content {
		t.Fatalf("segmentos após edição tardia incorretos: %+v", result.Segments)
	}
}

func TestHistoricalResponseSemAssistantFinalMantemTextoAntesDasTools(t *testing.T) {
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
	result := historicalResponse(t, []database.ChatMessage{message}, []TurnSegmentToolCall{{
		ID:                 "tool-after-text",
		Name:               "fetch",
		Iteration:          0,
		AssistantMessageID: message.ID,
	}})

	if result.Representative.ID != message.ID || len(result.Segments) != 2 {
		t.Fatalf("representante ou quantidade de segmentos incorretos: message=%+v segments=%+v", result.Representative, result.Segments)
	}
	if result.Segments[0].Kind != "text" || result.Segments[0].Text != message.Content ||
		result.Segments[1].Kind != "tool_invocations" || result.Segments[1].Invocations[0].ID != "tool-after-text" {
		t.Fatalf("texto foi promovido para depois das tools: %+v", result.Segments)
	}
}

// Completa apenas identidades de fixtures históricas; não adapta a saída canônica.
func historicalResponse(t *testing.T, messages []Message, calls []TurnSegmentToolCall) *TimelineItem {
	t.Helper()
	turnID := "fixture-turn"
	if len(messages) > 0 && messages[0].TurnID != nil {
		turnID = *messages[0].TurnID
	}
	rows := append([]Message(nil), messages...)
	for i := range rows {
		rows[i].ConversationID = "conv-1"
		rows[i].TurnID = &turnID
	}
	tools := append([]TurnSegmentToolCall(nil), calls...)
	for i := range tools {
		if tools[i].InvocationID == "" {
			tools[i].InvocationID = "inv-" + tools[i].ID
		}
	}
	item, err := ProjectPersistedResponse("conv-1", turnID, nil, rows, tools, nil)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func TestCanonicalIdentitySurvivesRepresentativeChange(t *testing.T) {
	_, messages, calls := canonicalTurnFixture()
	before := historicalResponse(t, messages[:1], nil)
	after := historicalResponse(t, messages, calls)
	if before.Representative.ID == after.Representative.ID {
		t.Fatal("fixture precisa trocar representante")
	}
	if before.ItemID != "turn:turn-1" || after.ItemID != before.ItemID {
		t.Fatal("identidade mudou", before, after)
	}
}

func TestCanonicalWindowPreservesOpaqueIDsAndIndependentMessages(t *testing.T) {
	plain, spaced := "question", " question "
	rows := []database.MessageWindowItem{
		{Kind: database.MessageWindowItemKindMessage, MessageID: "question"},
		{Kind: database.MessageWindowItemKindMessage, MessageID: "legacy"},
		{Kind: database.MessageWindowItemKindTurn, TurnID: plain},
		{Kind: database.MessageWindowItemKindTurn, TurnID: spaced},
	}
	messages := []Message{
		{UUIDModel: database.UUIDModel{ID: "question"}, Role: "user", TurnID: &plain},
		{UUIDModel: database.UUIDModel{ID: "legacy"}, Role: "assistant"},
	}
	responses := map[string]*TimelineItem{
		plain:  historicalResponse(t, []Message{{UUIDModel: database.UUIDModel{ID: "a"}, Role: "assistant", TurnID: &plain, Content: "primeira"}}, nil),
		spaced: historicalResponse(t, []Message{{UUIDModel: database.UUIDModel{ID: "b"}, Role: "assistant", TurnID: &spaced, Content: "segunda"}}, nil),
	}
	items, err := projectWindowItems("conv-1", nil, rows, messages, responses)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatal(items)
	}
	for i, want := range []string{"message:question", "message:legacy", "turn:question", "turn: question "} {
		if items[i].ItemID != want {
			t.Fatalf("item %d: %q != %q", i, items[i].ItemID, want)
		}
	}
	if items[2].Representative.Content != "primeira" || items[3].Representative.Content != "segunda" {
		t.Fatal("IDs opacos agrupados", items)
	}
}
