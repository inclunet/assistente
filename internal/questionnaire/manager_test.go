package questionnaire

import (
	"context"
	"testing"
	"time"
)

func TestRequestQuestionnaire_Answered(t *testing.T) {
	var emittedEvent string
	var emittedData any

	mgr := NewManager(func(event string, data any) {
		emittedEvent = event
		emittedData = data
	})

	go func() {
		time.Sleep(20 * time.Millisecond)
		if emittedEvent != "tool:questionnaire" {
			t.Errorf("expected event 'tool:questionnaire', got %q", emittedEvent)
			return
		}
		dataMap, ok := emittedData.(map[string]any)
		if !ok {
			t.Errorf("expected map[string]any data")
			return
		}
		id, _ := dataMap["id"].(string)
		if id == "" {
			t.Errorf("expected id to be set")
			return
		}
		if err := mgr.Respond(id, map[string]any{"q1": "ok"}, false); err != nil {
			t.Errorf("Respond error: %v", err)
		}
	}()

	resp, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Title: "Teste",
		Questions: []Question{
			{ID: "q1", Type: "text", Prompt: "Pergunta"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Answers["q1"] != "ok" {
		t.Fatalf("unexpected answer: %+v", resp.Answers)
	}
	if resp.Cancelled {
		t.Fatalf("expected cancelled=false")
	}
	if mgr.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", mgr.PendingCount())
	}
}

func TestRequestQuestionnaire_Cancelled(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(event string, data any) {
		go func() {
			time.Sleep(10 * time.Millisecond)
			dataMap := data.(map[string]any)
			mgr.Respond(dataMap["id"].(string), map[string]any{}, true)
		}()
	})

	resp, err := mgr.RequestQuestionnaire(context.Background(), RequestPayload{
		Title: "Teste",
		Questions: []Question{
			{ID: "q1", Type: "text", Prompt: "Pergunta"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Cancelled {
		t.Fatalf("expected cancelled=true")
	}
}

func TestRespondQuestionnaire_NotFound(t *testing.T) {
	mgr := NewManager(func(string, any) {})
	if err := mgr.Respond("missing", map[string]any{}, false); err == nil {
		t.Fatalf("expected error for missing request")
	}
}

func TestRequestQuestionnaire_ContextCancelled(t *testing.T) {
	mgr := NewManager(func(string, any) {})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, err := mgr.RequestQuestionnaire(ctx, RequestPayload{Questions: []Question{{ID: "q1", Type: "text", Prompt: "Pergunta"}}})
	if err == nil {
		t.Fatalf("expected error on cancel")
	}
}
