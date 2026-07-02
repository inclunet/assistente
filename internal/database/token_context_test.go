package database

import (
	"math"
	"testing"
	"time"
)

// createConvWithReportedUsage cria uma conversa com vários turnos (user +
// assistant), onde cada mensagem do assistente carrega o usage reportado pelo
// provedor. prompt_tokens cresce a cada turno porque o provedor recebe todo o
// histórico reenviado — exatamente o cenário que inflava a contagem antiga.
func createConvWithReportedUsage(t *testing.T) (convID string) {
	t.Helper()

	conv := &Conversation{Title: "ctx-usage", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("failed to create conversation: %v", err)
	}
	convID = conv.ID

	base := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	type msg struct {
		id        string
		role      string
		prompt    int
		compl     int
		toolCalls string
	}
	rows := []msg{
		{id: "01972000-0000-7000-8000-000000000001", role: "user"},
		{id: "01972000-0000-7000-8000-000000000002", role: "assistant", prompt: 1000, compl: 200}, // turno 1
		{id: "01972000-0000-7000-8000-000000000003", role: "user"},
		// turno 2 (mais recente)
		{id: "01972000-0000-7000-8000-000000000004", role: "assistant", prompt: 1500, compl: 300},
		// iteração com tool_calls sem usage persistido
		{id: "01972000-0000-7000-8000-000000000005", role: "assistant", toolCalls: `[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}}]`},
		{id: "01972000-0000-7000-8000-000000000006", role: "assistant", toolCalls: "[]"},
		{id: "01972000-0000-7000-8000-000000000007", role: "assistant", toolCalls: " null "},
	}
	for i, r := range rows {
		m := ChatMessage{
			UUIDModel: UUIDModel{
				ID:        r.id,
				CreatedAt: base.Add(time.Duration(i) * time.Minute),
			},
			ConversationID:   convID,
			Role:             r.role,
			Content:          "msg",
			PromptTokens:     r.prompt,
			CompletionTokens: r.compl,
			TotalTokens:      r.prompt + r.compl,
			ToolCalls:        r.toolCalls,
		}
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("failed to create message %d: %v", i, err)
		}
	}
	return convID
}

// TestGetContextWindowUsage_UsesLatestTurnNotCumulativeSum garante que a
// ocupação do contexto reflete o usage do ÚLTIMO turno do assistente
// (1500 + 300 = 1800), e não a soma acumulada de todos os turnos
// (1200 + 1800 = 3000), que inflava o percentual e disparava alertas falsos
// (issue #197).
func TestGetContextWindowUsage_UsesLatestTurnNotCumulativeSum(t *testing.T) {
	setupOrderingTestDB(t)
	convID := createConvWithReportedUsage(t)

	const contextLimit = 10000
	percentage, contextTokens, err := GetContextWindowUsageWithContext(testCtx(), convID, contextLimit)
	if err != nil {
		t.Fatalf("GetContextWindowUsage: %v", err)
	}

	if contextTokens != 1800 {
		t.Errorf("contextTokens: esperado 1800 (último turno), obtido %d", contextTokens)
	}
	if contextTokens == 3000 {
		t.Errorf("contextTokens não pode ser a soma acumulada (3000)")
	}
	if want := 18.0; math.Abs(percentage-want) > 1e-9 {
		t.Errorf("percentual: esperado %.1f%%, obtido %.1f%%", want, percentage)
	}
}

// TestGetDetailedTokenStats_ContextVsCumulative confirma que TotalTokens
// permanece acumulado (base de custo/billing) enquanto ContextTokens reflete
// apenas o turno mais recente.
func TestGetDetailedTokenStats_ContextVsCumulative(t *testing.T) {
	setupOrderingTestDB(t)
	convID := createConvWithReportedUsage(t)

	stats, err := GetDetailedTokenStatsWithContext(testCtx(), convID, "")
	if err != nil {
		t.Fatalf("GetDetailedTokenStats: %v", err)
	}
	if stats.TotalTokens != 3000 {
		t.Errorf("TotalTokens acumulado: esperado 3000, obtido %d", stats.TotalTokens)
	}
	if stats.ModelCallCount != 3 {
		t.Errorf("ModelCallCount: esperado 3 chamadas ao modelo, obtido %d", stats.ModelCallCount)
	}
	if stats.ContextTokens != 1800 {
		t.Errorf("ContextTokens (turno atual): esperado 1800, obtido %d", stats.ContextTokens)
	}
}

func TestGetDetailedTokenStats_CountsL3FreeAssistantToolInvocation(t *testing.T) {
	setupOrderingTestDB(t)
	if err := db.AutoMigrate(&User{}, &ToolCatalog{}, &ToolInvocation{}); err != nil {
		t.Fatalf("migrate tool invocations: %v", err)
	}
	tool := ToolCatalog{
		Name:               "search",
		DisplayName:        "search",
		Origin:             "builtin",
		AvailabilityStatus: "available",
	}
	if err := db.Create(&tool).Error; err != nil {
		t.Fatalf("create tool catalog: %v", err)
	}
	conv := &Conversation{Title: "l3-free-calls", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	turnID := "01972002-0000-7000-8000-000000000001"
	user := ChatMessage{
		UUIDModel:      UUIDModel{ID: turnID},
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "pergunta",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user message: %v", err)
	}
	assistantTool := ChatMessage{
		UUIDModel:      UUIDModel{ID: "01972002-0000-7000-8000-000000000002"},
		ConversationID: conv.ID,
		TurnID:         &turnID,
		Role:           "assistant",
		Content:        "vou buscar",
	}
	if err := db.Create(&assistantTool).Error; err != nil {
		t.Fatalf("create assistant tool message: %v", err)
	}
	finalAssistant := ChatMessage{
		UUIDModel:        UUIDModel{ID: "01972002-0000-7000-8000-000000000003"},
		ConversationID:   conv.ID,
		TurnID:           &turnID,
		Role:             "assistant",
		Content:          "resposta final",
		PromptTokens:     100,
		CompletionTokens: 20,
		TotalTokens:      120,
	}
	if err := db.Create(&finalAssistant).Error; err != nil {
		t.Fatalf("create final assistant message: %v", err)
	}
	if err := db.Create(&ToolInvocation{
		UserID:        testUserID,
		ToolCatalogID: tool.ID,
		OriginType:    "chat",
		OriginID:      turnID,
		ToolCallID:    "call-search",
		Status:        "succeeded",
		Metadata:      `{"display":{"version":1,"assistant_message_id":"` + assistantTool.ID + `","name":"search","arguments":"{}"}}`,
		QueuedAt:      time.Now(),
	}).Error; err != nil {
		t.Fatalf("create tool invocation: %v", err)
	}

	turnStats, err := GetTurnTokenStatsWithContext(testCtx(), conv.ID, turnID)
	if err != nil {
		t.Fatalf("GetTurnTokenStats: %v", err)
	}
	if turnStats.ModelCallCount != 2 {
		t.Fatalf("turn ModelCallCount: esperado 2 chamadas ao modelo, obtido %d", turnStats.ModelCallCount)
	}
	stats, err := GetDetailedTokenStatsWithContext(testCtx(), conv.ID, "")
	if err != nil {
		t.Fatalf("GetDetailedTokenStats: %v", err)
	}
	if stats.ModelCallCount != 2 {
		t.Fatalf("detailed ModelCallCount: esperado 2 chamadas ao modelo, obtido %d", stats.ModelCallCount)
	}
}

func TestGetDetailedTokenStats_EmptyConversationReturnsZeroes(t *testing.T) {
	setupOrderingTestDB(t)
	conv := &Conversation{Title: "empty", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	stats, err := GetDetailedTokenStatsWithContext(testCtx(), conv.ID, "")
	if err != nil {
		t.Fatalf("GetDetailedTokenStats: %v", err)
	}

	if stats.TotalTokens != 0 || stats.PromptTokens != 0 || stats.CompletionTokens != 0 {
		t.Fatalf("expected zero token totals, got prompt=%d completion=%d total=%d", stats.PromptTokens, stats.CompletionTokens, stats.TotalTokens)
	}
	if stats.MessageCount != 0 || stats.ModelCallCount != 0 {
		t.Fatalf("expected zero counts, got messages=%d calls=%d", stats.MessageCount, stats.ModelCallCount)
	}
}

// TestGetLatestReportedContextTokens_NoUsage retorna 0 quando ainda não há
// usage reportado (conversa só com mensagem de usuário).
func TestGetLatestReportedContextTokens_NoUsage(t *testing.T) {
	setupOrderingTestDB(t)
	conv := &Conversation{Title: "sem-usage", UserID: testUserID}
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	user := ChatMessage{
		UUIDModel:      UUIDModel{ID: "01972001-0000-7000-8000-000000000001", CreatedAt: time.Now()},
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "olá",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user message: %v", err)
	}

	got, err := getLatestReportedContextTokens(testCtx(), conv.ID)
	if err != nil {
		t.Fatalf("getLatestReportedContextTokens: %v", err)
	}
	if got != 0 {
		t.Errorf("esperado 0 sem usage reportado, obtido %d", got)
	}
}
