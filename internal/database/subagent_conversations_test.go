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

func createMsgWithTokens(t *testing.T, convID, role, content string, prompt, completion, total int) {
	t.Helper()
	msg := &ChatMessage{
		ConversationID:   convID,
		Role:             role,
		Content:          content,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      total,
	}
	if err := db.Create(msg).Error; err != nil {
		t.Fatalf("criar mensagem: %v", err)
	}
}

// TestListSubAgentConversationsAggregates valida a agregação via LEFT JOIN +
// GROUP BY: contagem de mensagens e soma de tokens por sub-conversa, incluindo o
// caso de sub-conversa SEM mensagens (LEFT JOIN → count 0 e somas 0), a exclusão
// de conversas normais (kind != subagent) e a ordenação por updated_at DESC.
func TestListSubAgentConversationsAggregates(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()

	now := time.Now()
	// convA: mais recente, com 2 mensagens e tokens.
	convA := createSubConvForList(t, "A", "parent-1", now)
	createMsgWithTokens(t, convA, "user", "m1", 100, 50, 150)
	createMsgWithTokens(t, convA, "assistant", "m2", 10, 5, 15)
	// convB: mais antiga, SEM mensagens (cobre LEFT JOIN → count 0).
	convB := createSubConvForList(t, "B", "parent-2", now.Add(-time.Hour))

	// Conversa normal (kind="") com mensagem: NÃO deve aparecer na listagem.
	normal := createTestConversation(t, "normal")
	createMsgWithTokens(t, normal, "user", "x", 7, 7, 14)

	rows, err := ListSubAgentConversationsWithContext(ctx)
	if err != nil {
		t.Fatalf("ListSubAgentConversationsWithContext: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("esperava 2 sub-conversas, veio %d (%#v)", len(rows), rows)
	}

	// Ordenação por updated_at DESC: convA primeiro, convB depois.
	if rows[0].ConversationID != convA || rows[1].ConversationID != convB {
		t.Fatalf("ordenação inesperada: %s, %s", rows[0].ConversationID, rows[1].ConversationID)
	}

	a := rows[0]
	if a.MessageCount != 2 || a.PromptTokens != 110 || a.CompletionTokens != 55 || a.TotalTokens != 165 {
		t.Fatalf("agregação de convA incorreta: %#v", a)
	}
	if a.ParentConversationID != "parent-1" || a.Title != "A" {
		t.Fatalf("metadados de convA incorretos: %#v", a)
	}

	b := rows[1]
	if b.MessageCount != 0 || b.PromptTokens != 0 || b.CompletionTokens != 0 || b.TotalTokens != 0 {
		t.Fatalf("convB sem mensagens deveria ter contagem/somas 0: %#v", b)
	}
	if b.ParentConversationID != "parent-2" {
		t.Fatalf("metadados de convB incorretos: %#v", b)
	}
}
