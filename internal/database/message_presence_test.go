package database

import "testing"

func TestHasConversationMessagesWithContext(t *testing.T) {
	setupTestDB(t)
	ctx := testCtx()
	conversationID := createTestConversation(t, "presença")

	hasMessages, err := HasConversationMessagesWithContext(ctx, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if hasMessages {
		t.Fatal("conversa nova não deveria ter mensagens")
	}

	if _, err := CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           "user",
		Content:        "primeira mensagem",
	}); err != nil {
		t.Fatal(err)
	}
	hasMessages, err = HasConversationMessagesWithContext(ctx, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasMessages {
		t.Fatal("mensagem persistida deveria ser detectada")
	}
}
