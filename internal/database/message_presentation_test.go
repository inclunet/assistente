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
