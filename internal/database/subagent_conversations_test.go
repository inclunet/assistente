package database

import (
	"testing"
	"time"
)

func createSubConvForList(t *testing.T, title, parent string, updatedAt time.Time) string {
	t.Helper()
	conv := &Conversation{
		Title:                title,
		UserID:               testUserID,
		Kind:                 ConversationKindSubagent,
		ParentConversationID: parent,
	}
	conv.CreatedAt = updatedAt
	conv.UpdatedAt = updatedAt
	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("criar sub-conversa: %v", err)
	}
	return conv.ID
}

func createRunForChild(t *testing.T, childConvID, status string, turnIndex int, createdAt time.Time) {
	t.Helper()
	run := &SubAgentRun{
		UserID:              testUserID,
		ChildConversationID: childConvID,
		TurnIndex:           turnIndex,
		Status:              status,
	}
	run.CreatedAt = createdAt
	run.UpdatedAt = createdAt
	if err := db.Create(run).Error; err != nil {
		t.Fatalf("criar sub_agent_run: %v", err)
	}
}

// TestGetConversationsUnifiedListing valida a listagem unificada (AEP-0068):
// GetConversationsWithContext retorna conversas comuns E sub-conversas de
// sub-agentes na mesma lista, ordenadas por updated_at DESC, preenchendo
// LatestStatus (status do run mais recente) apenas para sub-conversas.
func TestGetConversationsUnifiedListing(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()

	now := time.Now()
	// Sub-conversa mais recente, com dois runs: o mais recente (turn_index maior)
	// define o LatestStatus.
	subA := createSubConvForList(t, "Sub A", "parent-1", now)
	createRunForChild(t, subA, SubAgentRunStatusFailed, 0, now.Add(-2*time.Hour))
	createRunForChild(t, subA, SubAgentRunStatusRunning, 1, now.Add(-time.Hour))

	// Conversa normal, intermediária.
	normal := createTestConversation(t, "Normal")
	if err := db.Model(&Conversation{}).Where("id = ?", normal).Update("updated_at", now.Add(-30*time.Minute)).Error; err != nil {
		t.Fatalf("ajustar updated_at: %v", err)
	}

	// Sub-conversa mais antiga, sem runs (LatestStatus vazio).
	subB := createSubConvForList(t, "Sub B", "parent-2", now.Add(-time.Hour))

	rows, err := GetConversationsWithContext(ctx)
	if err != nil {
		t.Fatalf("GetConversationsWithContext: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("esperava 3 conversas, veio %d (%#v)", len(rows), rows)
	}

	byID := make(map[string]Conversation, len(rows))
	for _, c := range rows {
		byID[c.ID] = c
	}

	if got := byID[subA]; got.Kind != ConversationKindSubagent || got.LatestStatus != SubAgentRunStatusRunning {
		t.Fatalf("subA deveria ser kind=subagent com LatestStatus=running: %#v", got)
	}
	if got := byID[subB]; got.Kind != ConversationKindSubagent || got.LatestStatus != "" {
		t.Fatalf("subB (sem runs) deveria ter LatestStatus vazio: %#v", got)
	}
	if got := byID[normal]; got.Kind != "" || got.LatestStatus != "" {
		t.Fatalf("conversa normal não deveria ter kind/LatestStatus: %#v", got)
	}

	// Ordenação por updated_at DESC: subA (now), normal (-30m), subB (-1h).
	if rows[0].ID != subA || rows[1].ID != normal || rows[2].ID != subB {
		t.Fatalf("ordenação inesperada: %s, %s, %s", rows[0].ID, rows[1].ID, rows[2].ID)
	}
}

func TestGetConversationsPageWithContext(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()

	now := time.Now()
	first := createSubConvForList(t, "Mais recente", "parent-1", now)
	second := createTestConversation(t, "Intermediária")
	if err := db.Model(&Conversation{}).Where("id = ?", second).Update("updated_at", now).Error; err != nil {
		t.Fatalf("ajustar updated_at second: %v", err)
	}
	third := createSubConvForList(t, "Mais antiga", "parent-2", now.Add(-2*time.Minute))

	page, err := GetConversationsPageWithContext(ctx, 2, 1)
	if err != nil {
		t.Fatalf("GetConversationsPageWithContext: %v", err)
	}
	if page.Total != 3 {
		t.Fatalf("total = %d, want 3", page.Total)
	}
	if len(page.Conversations) != 2 {
		t.Fatalf("len page = %d, want 2 (%#v)", len(page.Conversations), page.Conversations)
	}
	expectedFirst := first
	expectedSecond := second
	if second > first {
		expectedFirst, expectedSecond = second, first
	}
	if page.Conversations[0].ID != expectedSecond || page.Conversations[1].ID != third {
		t.Fatalf("pagina inesperada depois de %s: %#v", expectedFirst, page.Conversations)
	}

	unpaged, err := GetConversationsPageWithContext(ctx, 0, 0)
	if err != nil {
		t.Fatalf("GetConversationsPageWithContext unpaged: %v", err)
	}
	if unpaged.Total != int64(len(unpaged.Conversations)) || unpaged.Total != 3 {
		t.Fatalf("total sem paginação = %d, len = %d, want 3", unpaged.Total, len(unpaged.Conversations))
	}

	defaultLimitedPage, err := GetConversationsPageWithContext(ctx, 0, 1)
	if err != nil {
		t.Fatalf("GetConversationsPageWithContext default limit: %v", err)
	}
	if defaultLimitedPage.Total != 3 {
		t.Fatalf("total com limit padrão = %d, want 3", defaultLimitedPage.Total)
	}
	if len(defaultLimitedPage.Conversations) != 2 {
		t.Fatalf("len default limited page = %d, want 2 (%#v)", len(defaultLimitedPage.Conversations), defaultLimitedPage.Conversations)
	}
	if defaultLimitedPage.Conversations[0].ID != expectedSecond || defaultLimitedPage.Conversations[1].ID != third {
		t.Fatalf("pagina com limit padrão inesperada depois de %s: %#v", expectedFirst, defaultLimitedPage.Conversations)
	}
}

func TestGetConversationsByIDsWithContext(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()

	first := createTestConversation(t, "Primeira")
	second := createTestConversation(t, "Segunda")
	now := time.Now()
	if err := db.Model(&Conversation{}).Where("id IN ?", []string{first, second}).Update("updated_at", now).Error; err != nil {
		t.Fatalf("ajustar updated_at empatado: %v", err)
	}
	otherUser, err := CreateConversationWithContext(WithUserID(testCtx(), "other-user"), "Outra", "")
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	rows, err := GetConversationsByIDsWithContext(ctx, []string{second, otherUser.ID, first, second})
	if err != nil {
		t.Fatalf("GetConversationsByIDsWithContext: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("esperava 2 conversas escopadas, veio %d (%#v)", len(rows), rows)
	}
	got := map[string]bool{}
	for _, row := range rows {
		if row.UserID != testUserID {
			t.Fatalf("vazou conversa de outro usuário: %#v", row)
		}
		got[row.ID] = true
	}
	if !got[first] || !got[second] || got[otherUser.ID] {
		t.Fatalf("ids retornados inesperados: %#v", got)
	}
	if rows[0].ID < rows[1].ID {
		t.Fatalf("ordenação por id desc com updated_at empatado inesperada: %s, %s", rows[0].ID, rows[1].ID)
	}
}

func TestGetConversationsByIDsWithContextLimitsIDs(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()

	kept := make([]string, 0, maxConversationIDLookupLimit)
	for i := 0; i < maxConversationIDLookupLimit; i++ {
		kept = append(kept, createTestConversation(t, "Dentro do limite"))
	}
	excluded := createTestConversation(t, "Fora do limite")
	ids := append(append([]string{}, kept...), excluded)

	rows, err := GetConversationsByIDsWithContext(ctx, ids)
	if err != nil {
		t.Fatalf("GetConversationsByIDsWithContext: %v", err)
	}
	if len(rows) != maxConversationIDLookupLimit {
		t.Fatalf("len rows = %d, want %d", len(rows), maxConversationIDLookupLimit)
	}
	for _, row := range rows {
		if row.ID == excluded {
			t.Fatalf("id além do limite foi retornado: %s", excluded)
		}
	}
}
