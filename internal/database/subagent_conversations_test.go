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
