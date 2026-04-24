package portability

import (
	"testing"
	"time"

	"assistente/internal/database"
)

func TestExportConversationUsesIndexesInsteadOfIDs(t *testing.T) {
	parentID := uint(10)
	turnID := uint(10)
	assistantID := uint(20)

	conv := &database.Conversation{
		Title: "Teste",
		Messages: []database.ChatMessage{
			{ID: parentID, Role: "user", Content: "Oi", CreatedAt: time.Unix(100, 0)},
			{ID: assistantID, Role: "assistant", Content: "Ola", ParentID: &parentID, TurnID: &turnID, CreatedAt: time.Unix(101, 0)},
		},
	}

	exported := exportConversation(conv, false)
	if len(exported.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(exported.Messages))
	}
	if exported.Messages[0].ParentIndex != nil {
		t.Fatalf("root ParentIndex = %v, want nil", *exported.Messages[0].ParentIndex)
	}
	if exported.Messages[1].ParentIndex == nil || *exported.Messages[1].ParentIndex != 0 {
		t.Fatalf("assistant ParentIndex = %v, want 0", exported.Messages[1].ParentIndex)
	}
	if exported.Messages[1].TurnIndex == nil || *exported.Messages[1].TurnIndex != 0 {
		t.Fatalf("assistant TurnIndex = %v, want 0", exported.Messages[1].TurnIndex)
	}
}

func TestExportConversationOmitsAudioByDefault(t *testing.T) {
	conv := &database.Conversation{
		Title: "Audio",
		Messages: []database.ChatMessage{
			{ID: 1, Role: "assistant", Content: "fala", Audio: "base64-audio", AudioMimeType: "audio/mpeg"},
		},
	}

	exported := exportConversation(conv, false)
	if exported.Messages[0].Audio != "" {
		t.Fatalf("Audio = %q, want empty", exported.Messages[0].Audio)
	}
	if exported.Messages[0].AudioMimeType != "audio/mpeg" {
		t.Fatalf("AudioMimeType = %q, want audio/mpeg", exported.Messages[0].AudioMimeType)
	}
}
