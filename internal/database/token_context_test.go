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
		id     string
		role   string
		prompt int
		compl  int
	}
	rows := []msg{
		{"01972000-0000-7000-8000-000000000001", "user", 0, 0},
		{"01972000-0000-7000-8000-000000000002", "assistant", 1000, 200}, // turno 1
		{"01972000-0000-7000-8000-000000000003", "user", 0, 0},
		{"01972000-0000-7000-8000-000000000004", "assistant", 1500, 300}, // turno 2 (mais recente)
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
	if stats.ContextTokens != 1800 {
		t.Errorf("ContextTokens (turno atual): esperado 1800, obtido %d", stats.ContextTokens)
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
