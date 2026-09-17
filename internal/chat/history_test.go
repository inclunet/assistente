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

func TestHistoryLoader_ClampsSingleMessageLimitWithoutPanic(t *testing.T) {
	repo := &stubRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "msg-1"}, Role: "user", Content: "primeira"},
			{UUIDModel: database.UUIDModel{ID: "msg-2"}, Role: "assistant", Content: "resposta"},
			{UUIDModel: database.UUIDModel{ID: "msg-3"}, Role: "user", Content: "recente"},
		},
	}

	messages, _, err := (&HistoryLoader{Repo: repo, MaxMsgs: 1}).Load(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(messages) > 2 {
		t.Fatalf("limite mínimo efetivo deveria ser 2, obteve %d mensagens", len(messages))
	}
}

func TestHistoryLoader_MantemMensagensConversacionaisDoTurno(t *testing.T) {
	turnID := "turn-1"
	repo := &stubRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: turnID, CreatedAt: time.Now().Add(-3 * time.Minute)}, Role: "user", Content: "hi"},
			{UUIDModel: database.UUIDModel{ID: "msg-assistant", CreatedAt: time.Now().Add(-2 * time.Minute)}, Role: "assistant", Content: "consultando", TurnID: &turnID},
			{UUIDModel: database.UUIDModel{ID: "msg-final", CreatedAt: time.Now().Add(-1 * time.Minute)}, Role: "assistant", Content: "resultado", TurnID: &turnID},
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
	if msgs[2].Role != "assistant" || msgs[2].Content != "resultado" {
		t.Fatalf("expected final assistant to be kept, got %+v", msgs[2])
	}
}

func TestHistoryLoader_PlaceholderAssistantVazioIsCleared(t *testing.T) {
	turnID := "turn-1"
	repo := &stubRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: turnID, CreatedAt: time.Now().Add(-2 * time.Minute)}, Role: "user", Content: "hi"},
			{UUIDModel: database.UUIDModel{ID: "msg-assistant", CreatedAt: time.Now().Add(-1 * time.Minute)}, Role: "assistant", TurnID: &turnID},
		},
	}

	loader := &HistoryLoader{Repo: repo, MaxMsgs: 100}
	msgs, _, err := loader.Load(context.Background(), "conv-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("expected only user message, got %+v", msgs)
	}
}

func TestHistoryLoader_LoadThroughMessage_FallsBackForOldMessageAndRetainsPriorSummary(t *testing.T) {
	repo := &stubRepo{
		summary: "resumo anterior",
		sumUpTo: "a-0",
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "u-0"}, Role: "user", Content: "anterior"},
			{UUIDModel: database.UUIDModel{ID: "a-0"}, Role: "assistant", Content: "resposta anterior"},
			{UUIDModel: database.UUIDModel{ID: "u-1"}, Role: "user", Content: "selecionada"},
			{UUIDModel: database.UUIDModel{ID: "a-1"}, Role: "assistant", Content: "resposta selecionada"},
			{UUIDModel: database.UUIDModel{ID: "u-2"}, Role: "user", Content: "posterior"},
			{UUIDModel: database.UUIDModel{ID: "a-2"}, Role: "assistant", Content: "resposta posterior"},
		},
	}

	messages, summary, err := (&HistoryLoader{Repo: repo, MaxMsgs: 50}).LoadThroughMessage(context.Background(), "conv-1", "u-1")
	if err != nil {
		t.Fatalf("LoadThroughMessage: %v", err)
	}
	if summary != "resumo anterior" {
		t.Fatalf("summary anterior perdido no fallback: %q", summary)
	}
	if len(messages) != 1 || messages[0].ID != "u-1" {
		t.Fatalf("janela ancorada incorreta: %+v", messages)
	}
}

func TestHistoryLoader_LoadThroughMessage_DropsPosteriorSummary(t *testing.T) {
	repo := &stubRepo{
		summary: "resumo que inclui posterior",
		sumUpTo: "a-2",
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "u-0"}, Role: "user", Content: "anterior"},
			{UUIDModel: database.UUIDModel{ID: "a-0"}, Role: "assistant", Content: "resposta anterior"},
			{UUIDModel: database.UUIDModel{ID: "u-1"}, Role: "user", Content: "selecionada"},
			{UUIDModel: database.UUIDModel{ID: "a-1"}, Role: "assistant", Content: "resposta selecionada"},
			{UUIDModel: database.UUIDModel{ID: "u-2"}, Role: "user", Content: "posterior"},
			{UUIDModel: database.UUIDModel{ID: "a-2"}, Role: "assistant", Content: "resposta posterior"},
		},
	}

	messages, summary, err := (&HistoryLoader{Repo: repo, MaxMsgs: 50}).LoadThroughMessage(context.Background(), "conv-1", "u-1")
	if err != nil {
		t.Fatalf("LoadThroughMessage: %v", err)
	}
	if summary != "" {
		t.Fatalf("summary posterior vazou para retry: %q", summary)
	}
	if len(messages) != 3 || messages[2].ID != "u-1" {
		t.Fatalf("histórico não foi cortado em U1: %+v", messages)
	}
}

func TestHistoryLoader_LoadThroughMessage_TargetNearWindowStartKeepsPreviousMessages(t *testing.T) {
	repo := &stubRepo{
		summary: "resumo até A0",
		sumUpTo: "a-0",
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "u-0"}, Role: "user", Content: "resumida"},
			{UUIDModel: database.UUIDModel{ID: "a-0"}, Role: "assistant", Content: "resumida"},
			{UUIDModel: database.UUIDModel{ID: "u-1"}, Role: "user", Content: "anterior válida"},
			{UUIDModel: database.UUIDModel{ID: "a-1"}, Role: "assistant", Content: "resposta anterior válida"},
			{UUIDModel: database.UUIDModel{ID: "u-2"}, Role: "user", Content: "alvo no início da janela recente"},
			{UUIDModel: database.UUIDModel{ID: "a-2"}, Role: "assistant", Content: "posterior ao alvo"},
		},
	}

	messages, summary, err := (&HistoryLoader{Repo: repo, MaxMsgs: 50}).LoadThroughMessage(context.Background(), "conv-1", "u-2")
	if err != nil {
		t.Fatalf("LoadThroughMessage: %v", err)
	}
	if summary != "resumo até A0" || len(messages) != 3 || messages[0].ID != "u-1" || messages[2].ID != "u-2" {
		t.Fatalf("contexto anterior ao alvo foi truncado: summary=%q messages=%+v", summary, messages)
	}
}
