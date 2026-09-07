package chat

import (
	"context"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestHistoryLoader_MissingSummaryUpToID_ClearsSummary(t *testing.T) {
	// When summaryUpToID references a deleted message (not found in allRootMessages),
	// the summary should be discarded to avoid duplication in the prompt.
	repo := &stubRepo{
		summary: "This is a summary of earlier messages.",
		sumUpTo: "msg-deleted-id",
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "msg-1", CreatedAt: time.Now().Add(-2 * time.Minute)}, Role: "user", Content: "Hello"},
			{UUIDModel: database.UUIDModel{ID: "msg-2", CreatedAt: time.Now().Add(-1 * time.Minute)}, Role: "assistant", Content: "Hi there"},
		},
	}

	loader := &HistoryLoader{Repo: repo, MaxMsgs: 100}
	msgs, summary, err := loader.Load(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if summary != "" {
		t.Errorf("expected empty summary when summaryUpToID not found, got %q", summary)
	}
	if len(msgs) != 2 {
		t.Errorf("expected all 2 messages, got %d", len(msgs))
	}
}

func TestHistoryLoader_ValidSummaryUpToID_RetainsSummary(t *testing.T) {
	// When summaryUpToID is found, summary should be retained and only messages
	// after the cut point should be returned.
	repo := &stubRepo{
		summary: "Summary of msg-1.",
		sumUpTo: "msg-1",
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "msg-1", CreatedAt: time.Now().Add(-3 * time.Minute)}, Role: "user", Content: "Hello"},
			{UUIDModel: database.UUIDModel{ID: "msg-2", CreatedAt: time.Now().Add(-2 * time.Minute)}, Role: "user", Content: "Follow up"},
			{UUIDModel: database.UUIDModel{ID: "msg-3", CreatedAt: time.Now().Add(-1 * time.Minute)}, Role: "assistant", Content: "Sure"},
		},
	}

	loader := &HistoryLoader{Repo: repo, MaxMsgs: 100}
	msgs, summary, err := loader.Load(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if summary != "Summary of msg-1." {
		t.Errorf("expected summary to be retained, got %q", summary)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages after cut, got %d", len(msgs))
	}
	if len(msgs) > 0 && msgs[0].ID != "msg-2" {
		t.Errorf("expected first message to be msg-2, got %s", msgs[0].ID)
	}
}

func TestHistoryLoader_ToolCallsSingleObject_DoesNotDropToolResult(t *testing.T) {
	turnID := "turn-1"
	callID := "call-1"
	repo := &stubRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: turnID, CreatedAt: time.Now().Add(-3 * time.Minute)}, Role: "user", Content: "hi"},
			{UUIDModel: database.UUIDModel{ID: "msg-assistant", CreatedAt: time.Now().Add(-2 * time.Minute)}, Role: "assistant", Content: "ok", TurnID: &turnID, ToolCalls: `{"id":"` + callID + `","type":"function","function":{"name":"x","arguments":"{}"}}`},
			{UUIDModel: database.UUIDModel{ID: "msg-tool", CreatedAt: time.Now().Add(-1 * time.Minute)}, Role: "tool", Content: "RESULT", TurnID: &turnID, ToolCallID: callID},
		},
	}

	loader := &HistoryLoader{Repo: repo, MaxMsgs: 100}
	msgs, _, err := loader.Load(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[2].Role != "tool" || msgs[2].ToolCallID != callID {
		t.Fatalf("expected tool result to be kept, got role=%s toolCallID=%s", msgs[2].Role, msgs[2].ToolCallID)
	}
}

func TestHistoryLoader_ToolCallsSingleObject_OrphanToolUseIsCleared(t *testing.T) {
	turnID := "turn-1"
	callID := "call-1"
	repo := &stubRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: turnID, CreatedAt: time.Now().Add(-2 * time.Minute)}, Role: "user", Content: "hi"},
			{UUIDModel: database.UUIDModel{ID: "msg-assistant", CreatedAt: time.Now().Add(-1 * time.Minute)}, Role: "assistant", Content: "ok", TurnID: &turnID, ToolCalls: `{"id":"` + callID + `","type":"function","function":{"name":"x","arguments":"{}"}}`},
		},
	}

	loader := &HistoryLoader{Repo: repo, MaxMsgs: 100}
	msgs, _, err := loader.Load(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "assistant" {
		t.Fatalf("expected assistant message, got role=%s", msgs[1].Role)
	}
	if msgs[1].ToolCalls != "" {
		t.Fatalf("expected orphan tool_calls to be cleared, got %q", msgs[1].ToolCalls)
	}
}
