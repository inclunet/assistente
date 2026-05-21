package agent

import (
	"context"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/llm"
)

type continuationStreamer struct {
	t           *testing.T
	wantPrefill string
}

func (s *continuationStreamer) StreamChat(_ context.Context, messages []llm.Message, params llm.ChatParams, handler llm.StreamHandler, _ ...llm.ToolDefinition) {
	if !params.AllowAssistantPrefill {
		s.t.Fatalf("expected AllowAssistantPrefill=true")
	}
	if len(messages) == 0 {
		s.t.Fatalf("expected non-empty messages")
	}
	last := messages[len(messages)-1]
	if last.Role != "assistant" {
		s.t.Fatalf("expected trailing assistant, got role=%q", last.Role)
	}
	if last.GetContentAsString() != s.wantPrefill {
		s.t.Fatalf("expected trailing assistant prefill %q, got %q", s.wantPrefill, last.GetContentAsString())
	}

	handler.OnChunk("X")
	handler.OnDone("", llm.Usage{}, "")
}

func TestStreamSimpleWithRecovery_AllowsAssistantPrefill_EmitsCumulativeContent(t *testing.T) {
	repo := &inMemoryMsgRepo{}
	turnID := "u1"
	repo.messages = []chat.Message{
		{UUIDModel: database.UUIDModel{ID: turnID}, ConversationID: "c1", Role: "user", Content: "hi"},
		{UUIDModel: database.UUIDModel{ID: "a1"}, ConversationID: "c1", Role: "assistant", Content: "prefill", TurnID: &turnID},
	}
	em := &captureEmitter{}
	svc := NewService(ServiceConfig{Emitter: em, MsgRepo: repo})

	messages := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "prefill"},
	}

	svc.StreamSimpleWithRecovery(
		context.Background(),
		&continuationStreamer{t: t, wantPrefill: "prefill"},
		messages,
		llm.ChatParams{AllowAssistantPrefill: true},
		"c1",
		turnID,
		"",
		nil,
		false,
		3,
	)

	streams := em.find("chat:stream")
	if len(streams) == 0 {
		t.Fatalf("expected at least one chat:stream event")
	}
	// O primeiro emit já deve conter o prefill + chunk
	first, ok := streams[0].data.(ports.StreamEvent)
	if !ok {
		t.Fatalf("expected ports.StreamEvent")
	}
	if first.Content != "prefillX" {
		t.Fatalf("expected first stream content to be %q, got %q", "prefillX", first.Content)
	}
	if first.MessageID != "a1" {
		t.Fatalf("expected stream messageId %q, got %q", "a1", first.MessageID)
	}
}
