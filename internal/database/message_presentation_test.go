package database

import (
	"context"

	"testing"
)

func TestCanonicalChildCountsPreserveGroupingAndOwnership(t *testing.T) {
	setupOrderingTestDB(t)
	conversationID, roots := createConvWithOrderedMessages(t)
	ctx := WithUserID(context.Background(), testUserID)
	counts, err := countCanonicalChildren(ctx, db, conversationID, roots)
	if err != nil || len(counts) != 0 {
		t.Fatal("página sem filhos recebeu contagem", counts, err)
	}
	question := ChatMessage{ConversationID: conversationID, ParentID: &roots[0], Role: "user", Content: "pergunta filha"}
	if err := db.Create(&question).Error; err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"intermediária", "conclusão"} {
		if err := db.Create(&ChatMessage{ConversationID: conversationID, ParentID: &roots[0], TurnID: &question.ID, Role: "assistant", Content: text}).Error; err != nil {
			t.Fatal(err)
		}
	}
	counts, err = countCanonicalChildren(ctx, db, conversationID, roots)
	if err != nil || counts[roots[0]] != 2 {
		t.Fatal("filhos perderam agrupamento canônico", counts, err)
	}
	counts, err = countCanonicalChildren(WithUserID(context.Background(), "other"), db, conversationID, roots)
	if err != nil || len(counts) != 0 {
		t.Fatal("contagem vazou para outro usuário", counts, err)
	}
}

func TestCanonicalChildCountMatchesWindowAcrossExecutionLifecycle(t *testing.T) {
	setupOrderingTestDB(t)
	conversationID, roots := createConvWithOrderedMessages(t)
	ctx := WithUserID(context.Background(), testUserID)
	parent := roots[0]
	question := ChatMessage{ConversationID: conversationID, ParentID: &parent, Role: "user", Content: "filha"}
	if err := db.Create(&question).Error; err != nil {
		t.Fatal(err)
	}
	turns := NewChatTurnRepository(db)
	attempt, err := turns.Begin(ctx, conversationID, question.ID, "", TurnModeNew)
	if err != nil {
		t.Fatal(err)
	}
	assertCount := func(want int) {
		t.Helper()
		counts, err := countCanonicalChildren(ctx, db, conversationID, []string{parent})
		if err != nil {
			t.Fatal(err)
		}
		page, err := NewMessageRepository(db).ReadMessageWindowWithContext(ctx, MessageWindowQuery{ConversationID: conversationID, ParentID: &parent, Anchor: "end", Direction: "before", Limit: 100})
		if err != nil || counts[parent] != want || page.TotalCount != want || len(page.Items) != want {
			t.Fatalf("contagem e janela divergiram: count=%d page=%+v want=%d err=%v", counts[parent], page, want, err)
		}
	}
	assertCount(2) // Pergunta e turno admitido, ainda sem resposta persistida.
	answer := ChatMessage{ConversationID: conversationID, ParentID: &parent, TurnID: &question.ID, ChatExecutionID: &attempt.ExecutionID, Role: "assistant", Content: "anterior"}
	if err := db.Create(&answer).Error; err != nil {
		t.Fatal(err)
	}
	if err := turns.Finish(ctx, conversationID, question.ID, attempt.ExecutionID, TurnCompleted, "completed"); err != nil {
		t.Fatal(err)
	}
	if _, err := turns.Begin(ctx, conversationID, question.ID, "", TurnModeRetry); err != nil {
		t.Fatal(err)
	}
	assertCount(2) // A execução anterior continua no banco, mas não duplica o item.
	var preserved int64
	if err := db.Model(&ChatMessage{}).Where("id=?", answer.ID).Count(&preserved).Error; err != nil || preserved != 1 {
		t.Fatal("retry apagou a execução anterior", preserved, err)
	}
	missing := "missing-question"
	if err := db.Create(&ChatMessage{ConversationID: conversationID, ParentID: &parent, TurnID: &missing, Role: "assistant", Content: "vínculo legado inválido"}).Error; err != nil {
		t.Fatal(err)
	}
	assertCount(3)
}
