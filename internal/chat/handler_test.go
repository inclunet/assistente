package chat

import (
	"context"
	"testing"

	"assistente/internal/database"
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

func TestEnsureAssistantPlaceholder_PrefersAssistantWithoutToolCalls(t *testing.T) {
	repo := &stubRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "assistant-placeholder"}, Role: "assistant", ToolCalls: ""},
			{UUIDModel: database.UUIDModel{ID: "assistant-tools"}, Role: "assistant", ToolCalls: `[{"id":"call-1"}]`},
		},
	}

	id, err := EnsureAssistantPlaceholder(context.Background(), repo, "conv-1", "turn-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "assistant-placeholder" {
		t.Fatalf("expected placeholder assistant ID, got %q", id)
	}
}

func TestEnsureAssistantPlaceholder_PrefersOriginalPlaceholderWhenIntermediateHasNoToolCalls(t *testing.T) {
	repo := &stubRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "assistant-placeholder"}, Role: "assistant", ToolCalls: "", Content: "resposta final antiga"},
			{UUIDModel: database.UUIDModel{ID: "assistant-intermediate"}, Role: "assistant", ToolCalls: "", Content: "vou buscar"},
		},
	}

	id, err := EnsureAssistantPlaceholder(context.Background(), repo, "conv-1", "turn-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "assistant-placeholder" {
		t.Fatalf("expected original placeholder assistant ID, got %q", id)
	}
}
