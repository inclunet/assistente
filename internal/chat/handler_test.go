package chat

import (
	"context"
	"testing"
)

func TestFinalizeAssistantMessage_WithNilRepo_PreservesAssistantMessageID(t *testing.T) {
	assistantID := "assistant-123"
	savedID, err := FinalizeAssistantMessage(context.Background(), nil, assistantID, MessageOptions{
		ConversationID: "conv-1",
		Content:        "resposta",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if savedID != assistantID {
		t.Fatalf("expected assistantMessageID %q, got %q", assistantID, savedID)
	}
}
